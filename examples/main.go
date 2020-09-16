package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/examples/common"
)

var ServerName = flag.String("name", "10.94.86.99", "")
var MaxPostSize = flag.Int64("max-post-size", 1024*1024*3, "")

const maxArticles = 100

type articleRef struct {
	msgid  string
	offset int64
	length int64
}

type groupStorage struct {
	group    *nnn.Group
	articles *common.HighLowSlice
}

type articleStorage struct {
	Headers textproto.MIMEHeader
	Body    string
	Refer   []string
}

type testBackendType struct {
	groups      map[string]*groupStorage
	articles    map[string]*articleRef
	index, data *os.File
	mu          sync.Mutex
}

func (tb *testBackendType) writeData(buf []byte) (*articleRef, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	f := tb.data

	if _, err := f.Seek(0, 2); err != nil {
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

	return &articleRef{
		offset: offset,
		length: int64(n),
	}, nil
}

func (tb *testBackendType) writeIndex(buf []byte) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	f := tb.index

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

func (tb *testBackendType) ListGroups(max int) ([]*nnn.Group, error) {
	rv := []*nnn.Group{}
	for _, g := range tb.groups {
		rv = append(rv, g.group)
	}
	return rv, nil
}

func (tb *testBackendType) GetGroup(name string) (*nnn.Group, error) {
	group, ok := tb.groups[name]
	if !ok {
		return nil, nnn.ErrNoSuchGroup
	}
	return group.group, nil
}

func (tb *testBackendType) mkArticle(a *articleRef) (*nnn.Article, error) {
	f, err := os.OpenFile(tb.data.Name(), os.O_RDONLY, 0777)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(a.offset, 0); err != nil {
		return nil, err
	}

	buf := make([]byte, a.length)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}

	as := articleStorage{}
	if err := json.Unmarshal(buf, &as); err != nil {
		return nil, err
	}

	hdr := make(textproto.MIMEHeader, len(as.Headers))
	for k, v := range as.Headers {
		if k == "X-Message-Id" {
			hdr["Message-Id"] = []string{"<" + v[0] + "@" + *ServerName + ">", v[0]}
		} else {
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

func (tb *testBackendType) GetArticle(group *nnn.Group, id string) (*nnn.Article, error) {
	msgID := id

	if intid, err := strconv.ParseInt(id, 10, 64); err == nil {
		groupStorage, ok := tb.groups[group.Name]
		if !ok {
			return nil, nnn.ErrNoSuchGroup
		}

		ar, _ := groupStorage.articles.Get(int(intid - 1))
		aref, ok := ar.(*articleRef)
		if !ok {
			log.Println("GetArticle", group, id, "not found")
			return nil, nnn.ErrInvalidArticleNumber
		}

		msgID = aref.msgid
	}
	msgID = ExtractMsgID(msgID)
	a := tb.articles[msgID]
	if a == nil {
		return nil, nnn.ErrInvalidMessageID
	}
	return tb.mkArticle(a)
}

func (tb *testBackendType) GetArticles(group *nnn.Group, from, to int64) ([]nnn.NumberedArticle, error) {
	gs, ok := tb.groups[group.Name]
	if !ok {
		return nil, nnn.ErrNoSuchGroup
	}

	var rv []nnn.NumberedArticle
	for i, v := range gs.articles.Slice(int(from-1), int(to-1)+1, false) {
		aref, ok := v.(*articleRef)
		if !ok {
			continue
		}
		a, ok := tb.articles[aref.msgid]
		if !ok {
			continue
		}
		aa, err := tb.mkArticle(a)
		if err != nil {
			log.Println("GetArticles, msgid:", aref.msgid, err)
			continue
		}
		rv = append(rv, nnn.NumberedArticle{
			Num:     int64(i) + (from - 1) + 1,
			Article: aa,
		})
	}

	return rv, nil
}

func (tb *testBackendType) AllowPost() bool {
	return true
}

func (tb *testBackendType) Post(article *nnn.Article) error {
	log.Printf("Post headers: %#v", article.Header)

	buf := &bytes.Buffer{}
	n, err := io.Copy(buf, io.LimitReader(article.Body, *MaxPostSize))
	if err != nil {
		return err
	}

	if n == *MaxPostSize {
		return &nnn.NNTPError{441, fmt.Sprintf("Post too large (max %d)", *MaxPostSize)}
	}

	var msgID string
	if msgid := article.Header["Message-Id"]; len(msgid) > 0 {
		msgID = ExtractMsgID(msgid[0])
		log.Println("Predefined msgid:", msgID)
		delete(article.Header, "Message-Id")
	} else {
		msgID = strconv.FormatInt(time.Now().Unix(), 36) + "-" + strconv.FormatUint(uint64(rand.Uint32()), 36)
	}

	article.Header["X-Message-Id"] = []string{msgID}

	a := articleStorage{
		Headers: article.Header,
		Body:    buf.String(),
		Refer:   article.Header["Newsgroups"],
	}

	if _, ok := tb.articles[msgID]; ok {
		return nnn.ErrPostingFailed
	}

	jsonbuf, _ := json.Marshal(a)
	ar, err := tb.writeData(jsonbuf)
	if err != nil {
		return err
	}
	ar.msgid = msgID

	tmp := bytes.Buffer{}
	for _, g := range a.Refer {
		if g, ok := tb.groups[g]; ok {
			tmp.WriteString(fmt.Sprintf("\nA%s %s %s %s", g.group.Name, msgID,
				strconv.FormatInt(ar.offset, 36),
				strconv.FormatInt(ar.length, 36)))
		}
	}

	if err := tb.writeIndex(tmp.Bytes()); err != nil {
		return err
	}

	for _, g := range a.Refer {
		g, ok := tb.groups[g]
		if !ok {
			continue
		}

		g.articles.Append(ar)
		g.group.Low = int64(g.articles.Low() + 1)
		g.group.High = int64(g.articles.High()+1) - 1
		g.group.Count = int64(g.articles.Len())
		log.Printf("%q new post: %v", g.group.Name, msgID)
	}

	if len(a.Refer) > 0 {
		tb.articles[msgID] = ar
	} else {
		return nnn.ErrPostingFailed
	}
	return nil
}

func (tb *testBackendType) Authorized() bool {
	return true
}

func (tb *testBackendType) Authenticate(user, pass string) (nnn.Backend, error) {
	return nil, nnn.ErrAuthRejected
}

func maybefatal(err error, f string, a ...interface{}) {
	if err != nil {
		log.Fatalf(f, a...)
	}
}

func main() {
	flag.Parse()

	b := &testBackendType{}
	if err := LoadIndex("testdb", b); err != nil {
		panic(err)
	}

	s := nnn.NewServer(b)

	handle := func(l net.Listener) {
		for {
			c, err := l.Accept()
			if err != nil {
				log.Println("Handle:", err)
				return
			}
			go s.Process(c)
		}
	}

	{
		a, err1 := net.ResolveTCPAddr("tcp", ":1119")
		maybefatal(err1, "Error resolving listener: %v", err1)
		l, err := net.ListenTCP("tcp", a)
		maybefatal(err, "Error setting up listener: %v", err)

		go handle(l)
	}

	{
		cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
		maybefatal(err, "Error tls cert: %v", err)

		l, err := tls.Listen("tcp", ":1563", &tls.Config{Certificates: []tls.Certificate{cert}})
		maybefatal(err, "Error setting up TLS listener: %v", err)

		go handle(l)
	}

	select {}
}

func ExtractMsgID(msgID string) string {
	if strings.HasPrefix(msgID, "<") && strings.HasSuffix(msgID, ">") {
		msgID = msgID[1 : len(msgID)-1]
		msgID = strings.Split(msgID, "@")[0]
	}
	return msgID
}
