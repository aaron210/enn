package backend

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/coyove/common/lru"
	"github.com/coyove/enn"
	"github.com/coyove/enn/server/common"
)

type Group struct {
	Group    *enn.Group
	BaseInfo *common.BaseGroupInfo
	Articles *common.HighLowSlice
}

func (g *Group) Append(b *Backend, ar *common.ArticleRef) {
	purged, _ := g.Articles.Append(ar)

	if len(purged) > 0 {
		tmp := bytes.Buffer{}
		for _, p := range purged {
			tmp.WriteString(fmt.Sprintf("\nD%s", p.MsgID()))
		}
		common.L("purge in append: %v, len: %d", b.writeIndex(tmp.Bytes()), len(purged))

		b.mu.Lock()
		defer b.mu.Unlock()
		for _, a := range purged {
			delete(b.Articles, a.RawMsgID)
		}
	}
}

type Backend struct {
	Groups       map[string]*Group
	Articles     map[[16]byte]*common.ArticleRef
	Mods         map[string]*common.ModInfo
	Index        *os.File
	Data         []*os.File
	AuthObject   interface{}
	PostInterval time.Duration

	ServerName string

	ipCache *lru.Cache
	filemu  *sync.Mutex
	mu      *sync.RWMutex
}

func (tb *Backend) Init() {
	tb.mu = new(sync.RWMutex)
	tb.filemu = new(sync.Mutex)
	tb.ipCache = lru.NewCache(1e3)

	if tb.PostInterval == 0 {
		tb.PostInterval = time.Minute
	}
}

func (tb *Backend) writeData(buf []byte) (*common.ArticleRef, error) {
	const sep = "\x01\x23\x45\x67\x89\xab\xcd\xef"

	tb.filemu.Lock()
	defer tb.filemu.Unlock()

	f := tb.Data[len(tb.Data)-1]

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
		Index:  len(tb.Data) - 1,
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

func (tb *Backend) ListGroups(max int) ([]*enn.Group, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	var rv []*enn.Group
	for _, g := range tb.Groups {
		rv = append(rv, g.Group)
	}
	return rv, nil
}

func (tb *Backend) GetGroup(name string) (*enn.Group, error) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	group, ok := tb.Groups[name]
	if !ok {
		return nil, enn.ErrNoSuchGroup
	}
	return group.Group, nil
}

func (tb *Backend) mkArticle(a *common.ArticleRef, headerOnly bool) (A *enn.Article, E error) {
	if a.Index >= len(tb.Data) {
		return nil, enn.ErrInvalidArticleNumber
	}

	name := tb.Data[a.Index].Name()
	f, err := os.OpenFile(name, os.O_RDONLY, 0777)
	if err != nil {
		common.E("open %q: %v", name, err)
		return nil, enn.ErrServerBad
	}

	defer func() {
		f.Close()
		if E != nil {
			common.E("open %q: %v", name, E)
			E = enn.ErrInvalidArticleNumber
		}
	}()

	// Early check
	var matchLength int64
	f.Seek(a.Offset-8, 0)
	binary.Read(f, binary.BigEndian, &matchLength)
	if matchLength != a.Length {
		return nil, fmt.Errorf("invalid length marker %d, expect %d", matchLength, a.Length)
	}

	if _, err := f.Seek(a.Offset, 0); err != nil {
		return nil, err
	}

	rd := io.LimitReader(f, a.Length)
	as := common.Article{}
	if err := as.Unmarshal(rd, headerOnly); err != nil {
		common.E("corrupted article ref %v", err)
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

	na := &enn.Article{
		Header: hdr,
		Body:   bytes.NewReader(as.Body),
	}
	na.Bytes, _ = strconv.Atoi(hdr.Get("X-Length"))
	na.Lines, _ = strconv.Atoi(hdr.Get("X-Lines"))
	return na, nil
}

func (tb *Backend) DeleteArticle(msgid string) error {
	if err := tb.writeIndex([]byte("\nD" + msgid)); err != nil {
		return err
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	delete(tb.Articles, common.MsgIDToRawMsgID(msgid, nil))
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
	gs, ok := tb.Articles[common.MsgIDToRawMsgID(id, nil)]
	tb.mu.RUnlock()
	return gs, ok
}

func (tb *Backend) GetArticle(group *enn.Group, id string, ho bool) (*enn.Article, error) {
	msgID := id

	if intId, err := strconv.ParseInt(id, 10, 64); err == nil {
		groupStorage, ok := tb.internalGetGroup(group.Name)
		if !ok {
			return nil, enn.ErrNoSuchGroup
		}

		ar, _ := groupStorage.Articles.Get(int(intId - 1))
		if ar == nil {
			common.E("get article %q in %q not found ", id, group)
			return nil, enn.ErrInvalidArticleNumber
		}
		msgID = ar.MsgID()
	}
	msgID = common.ExtractMsgID(msgID)
	a, _ := tb.internalGetArticle(msgID)
	if a == nil {
		return nil, enn.ErrInvalidMessageID
	}
	return tb.mkArticle(a, ho)
}

func (tb *Backend) GetArticles(group *enn.Group, from, to int64, ho bool) ([]enn.NumberedArticle, error) {
	gs, ok := tb.internalGetGroup(group.Name)
	if !ok {
		return nil, enn.ErrNoSuchGroup
	}

	var rv []enn.NumberedArticle
	refs, start, _ := gs.Articles.Slice(int(from-1), int(to-1)+1, false)
	for i, v := range refs {
		if v == nil {
			continue
		}
		a, ok := tb.internalGetArticle(v.MsgID())
		if !ok {
			continue
		}
		aa, err := tb.mkArticle(a, ho)
		if err != nil {
			common.E("failed to get article %q: %v", v.MsgID(), err)
			continue
		}
		rv = append(rv, enn.NumberedArticle{
			Num:     int64(i+start) + 1,
			Article: aa,
		})
	}

	return rv, nil
}

func (tb *Backend) AllowPost() bool {
	return true
}

func (tb *Backend) Authorized() bool {
	return ImplAuth(tb, "", "") == nil
}

func (tb *Backend) Authenticate(user, pass string) (enn.Backend, error) {
	tb2 := *tb
	err := ImplAuth(&tb2, user, pass)
	return &tb2, err
}
