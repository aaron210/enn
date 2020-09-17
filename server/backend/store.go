package backend

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/common"
)

type Group struct {
	Info     *nnn.Group
	Articles *common.HighLowSlice
}

func (g *Group) Append(b *Backend, ar *common.ArticleRef) {
	purged, _ := g.Articles.Append(ar)

	if len(purged) > 0 {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, a := range purged {
			delete(b.Articles, a.MsgID)
		}
	}
}

type Backend struct {
	Groups      map[string]*Group
	Articles    map[string]*common.ArticleRef
	Mods        map[string]*common.ModInfo
	Index, Data *os.File
	AuthObject  interface{}

	ServerName      string
	MaxLiveArticels int

	filemu *sync.Mutex
	mu     *sync.RWMutex
}

func (tb *Backend) Init() {
	tb.mu = new(sync.RWMutex)
	tb.filemu = new(sync.Mutex)
}

func (tb *Backend) writeData(buf []byte) (*common.ArticleRef, error) {
	const sep = "\x01\x23\x45\x67\x89\xab\xcd\xef"

	tb.filemu.Lock()
	defer tb.filemu.Unlock()

	f := tb.Data

	if _, err := f.Seek(0, 2); err != nil {
		return nil, err
	}

	if _, err := f.WriteString(sep); err != nil {
		return nil, err
	}

	if err := binary.Write(f, binary.BigEndian, int64(len(buf))); err != nil {
		return nil, err
	}

	offset, err := f.Seek(0, 1)
	if err != nil {
		return nil, err
	}

	n, err := f.Write(buf)
	if err != nil {
		return nil, err
	}

	if n != len(buf) {
		return nil, io.ErrShortWrite
	}

	return &common.ArticleRef{
		Offset: offset,
		Length: int64(n),
	}, nil
}

func (tb *Backend) WriteCommand(buf []byte) error {
	return tb.writeIndex(buf)
}

func (tb *Backend) writeIndex(buf []byte) error {
	tb.filemu.Lock()
	defer tb.filemu.Unlock()
	f := tb.Index

	if _, err := f.Seek(0, 2); err != nil {
		return err
	}
	n, err := f.Write(buf)
	if err != nil {
		return err
	}
	if n != len(buf) {
		return io.ErrShortWrite
	}
	return nil
}

func (tb *Backend) ListGroups(max int) ([]*nnn.Group, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var rv []*nnn.Group
	for _, g := range tb.Groups {
		rv = append(rv, g.Info)
	}
	return rv, nil
}

func (tb *Backend) GetGroup(name string) (*nnn.Group, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	group, ok := tb.Groups[name]
	if !ok {
		return nil, nnn.ErrNoSuchGroup
	}
	return group.Info, nil
}

func (tb *Backend) mkArticle(a *common.ArticleRef) (*nnn.Article, error) {
	f, err := os.OpenFile(tb.Data.Name(), os.O_RDONLY, 0777)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(a.Offset, 0); err != nil {
		return nil, err
	}

	buf := make([]byte, a.Length)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}

	as := common.Article{}
	if err := as.Unmarshal(buf); err != nil {
		log.Printf("corrupted article ref %v", err)
		return nil, err
	}

	hdr := make(textproto.MIMEHeader, len(as.Headers))
	for k, v := range as.Headers {
		switch k {
		case "X-Message-Id":
			hdr["Message-Id"] = []string{"<" + v[0] + "@" + tb.ServerName + ">", v[0]}
		case "X-Remote-Ip":
			// Not viewable from client side
		default:
			hdr[k] = v
		}
	}
	return &nnn.Article{
		Header: hdr,
		Body:   strings.NewReader(as.Body),
		Bytes:  len(as.Body),
		Lines:  strings.Count(as.Body, "\n"),
	}, nil
}

func (tb *Backend) DeleteArticle(msgid string) error {
	if err := tb.writeIndex([]byte("\nD" + msgid)); err != nil {
		return err
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	delete(tb.Articles, msgid)
	return nil
}

func (tb *Backend) internalGetGroup(name string) (*Group, bool) {
	tb.mu.RLock()
	gs, ok := tb.Groups[name]
	tb.mu.RUnlock()
	return gs, ok
}

func (tb *Backend) internalGetArticle(id string) (*common.ArticleRef, bool) {
	tb.mu.RLock()
	gs, ok := tb.Articles[id]
	tb.mu.RUnlock()
	return gs, ok
}

func (tb *Backend) GetArticle(group *nnn.Group, id string) (*nnn.Article, error) {
	msgID := id

	if intId, err := strconv.ParseInt(id, 10, 64); err == nil {
		groupStorage, ok := tb.internalGetGroup(group.Name)
		if !ok {
			return nil, nnn.ErrNoSuchGroup
		}

		ar, _ := groupStorage.Articles.Get(int(intId - 1))
		if ar == nil {
			log.Println("get article:", group, id, "not found")
			return nil, nnn.ErrInvalidArticleNumber
		}
		msgID = ar.MsgID
	}
	msgID = common.ExtractMsgID(msgID)
	a, _ := tb.internalGetArticle(msgID)
	if a == nil {
		return nil, nnn.ErrInvalidMessageID
	}
	return tb.mkArticle(a)
}

func (tb *Backend) GetArticles(group *nnn.Group, from, to int64) ([]nnn.NumberedArticle, error) {
	gs, ok := tb.internalGetGroup(group.Name)
	if !ok {
		return nil, nnn.ErrNoSuchGroup
	}

	var rv []nnn.NumberedArticle
	refs, start, _ := gs.Articles.Slice(int(from-1), int(to-1)+1, false)
	for i, v := range refs {
		if v == nil {
			continue
		}
		a, ok := tb.internalGetArticle(v.MsgID)
		if !ok {
			continue
		}
		aa, err := tb.mkArticle(a)
		if err != nil {
			log.Println("failed to get article:", v.MsgID, err)
			continue
		}
		rv = append(rv, nnn.NumberedArticle{
			Num:     int64(i+start) + 1,
			Article: aa,
		})
	}

	return rv, nil
}

func (tb *Backend) AllowPost() bool {
	return true
}

func (tb *Backend) Post(article *nnn.Article) error {
	return nnn.ErrNotAuthenticated

	log.Printf("post: %#v", article.Header)

	subject := article.Header.Get("Subject")
	switch strings.TrimSpace(subject) {
	case "d":
		if tb.AuthObject == nil {
			return nnn.ErrNotAuthenticated
		}
		if !ImplIsMod(tb) {
			return &nnn.NNTPError{Code: 441, Msg: "Not moderator"}
		}
		refer := article.Header.Get("References")
		if refer == "" {
			return &nnn.NNTPError{Code: 441, Msg: "Please refer an article"}
		}
		log.Println("delete article", tb.AuthObject, refer)
		return tb.DeleteArticle(common.ExtractMsgID(refer))
	}

	mps := ImplMaxPostSize(tb)

	buf := &bytes.Buffer{}
	n, err := io.Copy(buf, io.LimitReader(article.Body, mps))
	if err != nil {
		return err
	}

	if n >= mps {
		return &nnn.NNTPError{Code: 441, Msg: fmt.Sprintf("Post too large (max %d)", mps)}
	}

	var msgID string
	if msgid := article.Header["Message-Id"]; len(msgid) > 0 {
		msgID = common.ExtractMsgID(msgid[0])
		log.Println("post: predefined msgid:", msgID)
		delete(article.Header, "Message-Id")
	} else {
		msgID = strconv.FormatInt(time.Now().Unix(), 36) + "-" + strconv.FormatUint(uint64(rand.Uint32()), 36)
	}

	article.Header["X-Message-Id"] = []string{msgID}
	article.Header["X-Remote-Ip"] = []string{fmt.Sprint(article.RemoteAddr)}

	a := common.Article{
		Headers: article.Header,
		Body:    buf.String(),
		Refer:   article.Header["Newsgroups"],
	}

	for i := len(a.Refer) - 1; i >= 0; i-- {
		ar := a.Refer[i]
		groups := strings.Split(ar, ",")
		if len(groups) > 1 {
			for i := range groups {
				groups[i] = strings.TrimSpace(groups[i])
			}
			a.Refer = append(a.Refer[:i], append(groups, a.Refer[i+1:]...)...)
		}
	}

	if _, ok := tb.Articles[msgID]; ok {
		return nnn.ErrPostingFailed
	}

	ar, err := tb.writeData(a.Marshal())
	if err != nil {
		return err
	}
	ar.MsgID = msgID

	tmp := bytes.Buffer{}
	for _, g := range a.Refer {
		if g, ok := tb.Groups[g]; ok {
			tmp.WriteString(fmt.Sprintf("\nA%s %s %s %s", g.Info.Name, msgID,
				strconv.FormatInt(ar.Offset, 36),
				strconv.FormatInt(ar.Length, 36)))
		}
	}
	if err := tb.writeIndex(tmp.Bytes()); err != nil {
		return err
	}

	postSuccess := 0
	for _, g := range a.Refer {
		g, ok := tb.Groups[g]
		if !ok {
			continue
		}

		if g.Info.Posting == nnn.PostingNotPermitted {
			if !ImplIsMod(tb) {
				continue
			}
		}

		g.Append(tb, ar)
		g.Info.Low = int64(g.Articles.Low() + 1)
		g.Info.High = int64(g.Articles.High()+1) - 1
		g.Info.Count = int64(g.Articles.Len())

		log.Printf("post: %q new article %v", g.Info.Name, msgID)
		postSuccess++
	}

	if postSuccess > 0 {
		tb.Articles[msgID] = ar
	} else {
		return nnn.ErrPostingFailed
	}
	return nil
}

func (tb *Backend) Authorized() bool {
	return ImplAuth(tb, "", "") == nil
}

func (tb *Backend) Authenticate(user, pass string) (nnn.Backend, error) {
	tb2 := *tb
	err := ImplAuth(&tb2, user, pass)
	return &tb2, err
}
