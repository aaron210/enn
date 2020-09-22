package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"sync"

	"github.com/coyove/common/lru"
	"github.com/coyove/enn"
	"github.com/coyove/enn/server/common"
)

type Group struct {
	Group         *enn.Group
	BaseInfo      *common.BaseGroupInfo
	Articles      *common.HighLowSlice
	NoPurgeNotify bool
}

func (g *Group) Append(b *Backend, ar *common.ArticleRef) {
	purged, _ := g.Articles.Append(ar)

	if len(purged) > 0 {
		if !g.NoPurgeNotify {
			tmp := bytes.Buffer{}
			for _, p := range purged {
				tmp.WriteString(fmt.Sprintf("\nD%s", p.MsgID()))
			}
			common.D("purge in append: %v, len: %d", b.writeIndex(tmp.Bytes()), len(purged))
		}

		b.mu.Lock()
		defer b.mu.Unlock()
		for _, a := range purged {
			delete(b.Articles, a.RawMsgID)
		}
	}
}

type Backend struct {
	Config     common.Config
	ServerName string

	Groups    map[string]*Group
	Articles  map[[16]byte]*common.ArticleRef
	Mods      map[string]*common.ModInfo
	Blacklist map[string]*net.IPNet

	Index *os.File
	Data  []*os.File

	AuthObject *common.AuthObject

	ipCache *lru.Cache
	muFile  *sync.Mutex
	mu      *sync.RWMutex
}

func (db *Backend) IsMod() bool {
	if db.AuthObject == nil {
		return false
	}
	mi := db.Mods[db.AuthObject.User]
	if mi == nil {
		return false
	}
	return mi.Password == db.AuthObject.Pass
}

func (db *Backend) writeData(buf []byte) (*common.ArticleRef, error) {
	const sep = "\x01\x23\x45\x67\x89\xab\xcd\xef"

	db.muFile.Lock()
	defer db.muFile.Unlock()

	f := db.Data[len(db.Data)-1]

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
		Index:  len(db.Data) - 1,
		Offset: offset,
		Length: int64(n),
	}, nil
}

func (db *Backend) WriteCommand(buf []byte) error {
	return db.writeIndex(buf)
}

func (db *Backend) writeIndex(buf []byte) error {
	db.muFile.Lock()
	defer db.muFile.Unlock()
	f := db.Index

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

func (db *Backend) ListGroups(max int) ([]*enn.Group, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	var rv []*enn.Group
	for _, g := range db.Groups {
		rv = append(rv, g.Group)
	}
	return rv, nil
}

func (db *Backend) GetGroup(name string) (*enn.Group, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	group, ok := db.Groups[name]
	if !ok {
		return nil, enn.ErrNoSuchGroup
	}
	return group.Group, nil
}

func (db *Backend) mkArticle(a *common.ArticleRef, headerOnly bool, errors *[]error) (A *enn.Article, E error) {
	if a.Index >= len(db.Data) {
		return nil, enn.ErrInvalidArticleNumber
	}

	name := db.Data[a.Index].Name()
	f, err := os.OpenFile(name, os.O_RDONLY, 0777)
	if err != nil {
		common.E("open %q: %v", name, err)
		return nil, enn.ErrServerBad
	}

	defer func() {
		f.Close()
		if E != nil {
			if errors != nil {
				*errors = append(*errors, fmt.Errorf("load data %v: %v", a.MsgID(), E))
			} else {
				common.E("load data %v: %v", a, E)
			}
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
			hdr["Message-Id"] = []string{"<" + v[0] + "@" + db.ServerName + ">", v[0]}
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

func (db *Backend) DeleteArticle(msgID string) error {
	if err := db.writeIndex([]byte("\nD" + msgID)); err != nil {
		return err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.Articles, common.MsgIDToRawMsgID(msgID, nil))
	return nil
}

func (db *Backend) internalGetGroup(name string) (*Group, bool) {
	db.mu.RLock()
	gs, ok := db.Groups[name]
	db.mu.RUnlock()
	return gs, ok
}

func (db *Backend) internalGetArticle(msgID string) (*common.ArticleRef, bool) {
	db.mu.RLock()
	gs, ok := db.Articles[common.MsgIDToRawMsgID(msgID, nil)]
	db.mu.RUnlock()
	return gs, ok
}

func (db *Backend) GetArticle(group *enn.Group, id string, ho bool) (*enn.Article, error) {
	msgID := id

	if intId, err := strconv.ParseInt(id, 10, 64); err == nil {
		groupStorage, ok := db.internalGetGroup(group.Name)
		if !ok {
			return nil, enn.ErrNoSuchGroup
		}

		ar, _ := groupStorage.Articles.Get(int(intId - 1))
		if ar == nil {
			common.E("get article %q in %v not found ", id, group)
			return nil, enn.ErrInvalidArticleNumber
		}
		msgID = ar.MsgID()
	}
	msgID = common.ExtractMsgID(msgID)
	a, _ := db.internalGetArticle(msgID)
	if a == nil {
		return nil, enn.ErrInvalidMessageID
	}
	return db.mkArticle(a, ho, nil)
}

func (db *Backend) GetArticles(group *enn.Group, from, to int64, ho bool) ([]enn.NumberedArticle, error) {
	gs, ok := db.internalGetGroup(group.Name)
	if !ok {
		return nil, enn.ErrNoSuchGroup
	}

	var rv []enn.NumberedArticle
	var errors []error
	refs, start, _ := gs.Articles.Slice(int(from-1), int(to-1)+1, false)
	for i, v := range refs {
		if v == nil {
			continue
		}
		a, ok := db.internalGetArticle(v.MsgID())
		if !ok {
			continue
		}
		aa, err := db.mkArticle(a, ho, &errors)
		if err != nil {
			continue
		}
		rv = append(rv, enn.NumberedArticle{
			Num:     int64(i+start) + 1,
			Article: aa,
		})
	}

	if len(errors) > 0 {
		if len(errors) > 10 {
			errors = append(errors[:5], errors[len(errors)-5:]...)
		}
		common.E("get articles, multiple errors: %v", errors)
	}

	return rv, nil
}

func (db *Backend) AllowPost() bool {
	return true
}

// func (tb *Backend) Authorized() bool {
// 	return ImplAuth(tb, "", "") == nil
// }

func (db *Backend) Authenticate(user, pass string) (enn.Backend, error) {
	tb2 := *db
	tb2.AuthObject = &common.AuthObject{user, pass}
	return &tb2, nil
}
