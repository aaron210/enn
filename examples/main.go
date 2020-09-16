package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coyove/nnn"
)

var ServerName = flag.String("name", "10.94.86.99", "")
var MaxPostSize = flag.Int64("max-post-size", 1024*1024*3, "")

type HighLowSlice struct {
	mu        sync.RWMutex
	d         []interface{}
	MaxSize   int
	high, low int
}

func (s *HighLowSlice) Len() int { return s.high }

func (s *HighLowSlice) High() int { return s.high }

func (s *HighLowSlice) Low() int { return s.low }

func (s *HighLowSlice) String() string {
	return fmt.Sprintf("[%v~%v max:%v data:%v", s.low, s.high, s.MaxSize, s.d)
}

func (s *HighLowSlice) Get(i int) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i < s.low || i >= s.high {
		return nil, false
	}
	i -= s.low
	return s.d[i], true
}

func (s *HighLowSlice) Set(i int, v interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < s.low {
		return
	}
	i -= s.low
	s.d[i] = v
}

func (s *HighLowSlice) Slice(i, j int, copy bool) []interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if j <= s.low || j > s.high {
		return nil
	}
	j -= s.low
	if i < s.low {
		i = s.low
	}
	if copy {
		return append([]interface{}{}, s.d[i:j]...)
	}
	return s.d[i:j]
}

func (s *HighLowSlice) Append(v interface{}) ([]interface{}, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.d = append(s.d, v)
	s.high++

	var purged []interface{}
	if len(s.d) > s.MaxSize {
		p := 1 / float64(len(s.d)-s.MaxSize+1)
		if rand.Float64() > p {
			x := len(s.d) - s.MaxSize
			purged = append([]interface{}{}, s.d[:x]...)

			s.low += x
			copy(s.d, s.d[x:])
			s.d = s.d[:s.MaxSize]
		}
	}
	return purged, s.high
}

const maxArticles = 100

type articleRef struct {
	msgid string
	num   int
}

type groupStorage struct {
	group *nnn.Group
	// article refs
	articles *HighLowSlice
}

type articleStorage struct {
	headers  textproto.MIMEHeader
	body     string
	refcount int
}

type testBackendType struct {
	// group name -> group storage
	groups map[string]*groupStorage
	// message ID -> article
	articles map[string]*articleStorage
}

var testBackend = testBackendType{
	groups:   map[string]*groupStorage{},
	articles: map[string]*articleStorage{},
}

func init() {

	testBackend.groups["alt.test"] = &groupStorage{
		group: &nnn.Group{
			Name:        "alt.test",
			Description: "A test.",
			Posting:     nnn.PostingNotPermitted},
		articles: &HighLowSlice{MaxSize: maxArticles},
	}

	testBackend.groups["misc.test"] = &groupStorage{
		group: &nnn.Group{
			Name:        "misc.test",
			Description: "More testing.",
			Posting:     nnn.PostingPermitted},
		articles: &HighLowSlice{MaxSize: maxArticles},
	}

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

func mkArticle(a *articleStorage) *nnn.Article {
	hdr := make(textproto.MIMEHeader, len(a.headers))
	for k, v := range a.headers {
		if k == "X-Message-Id" {
			hdr["Message-Id"] = []string{"<" + v[0] + "@" + *ServerName + ">", v[0]}
		} else {
			hdr[k] = v
		}
	}
	return &nnn.Article{
		Header: hdr,
		Body:   strings.NewReader(a.body),
		Bytes:  len(a.body),
		Lines:  strings.Count(a.body, "\n"),
	}
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
	return mkArticle(a), nil
}

func (tb *testBackendType) GetArticles(group *nnn.Group, from, to int64) ([]nnn.NumberedArticle, error) {
	gs, ok := tb.groups[group.Name]
	if !ok {
		return nil, nnn.ErrNoSuchGroup
	}

	var rv []nnn.NumberedArticle
	for _, v := range gs.articles.Slice(int(from-1), int(to-1)+1, false) {
		aref, ok := v.(*articleRef)
		if !ok {
			continue
		}
		a, ok := tb.articles[aref.msgid]
		if !ok {
			continue
		}
		rv = append(rv, nnn.NumberedArticle{
			Num:     int64(aref.num),
			Article: mkArticle(a),
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
		headers:  article.Header,
		body:     buf.String(),
		refcount: 0,
	}

	if _, ok := tb.articles[msgID]; ok {
		return nnn.ErrPostingFailed
	}

	for _, g := range article.Header["Newsgroups"] {
		if g, ok := tb.groups[g]; ok {
			a.refcount++

			ar := &articleRef{msgid: msgID}
			_, ar.num = g.articles.Append(ar)

			g.group.Low = int64(g.articles.Low() + 1)
			g.group.High = int64(g.articles.High()+1) - 1
			g.group.Count = int64(g.articles.Len())

			log.Printf("%q new post: %v", g.group.Name, msgID)
		}
	}

	if a.refcount > 0 {
		tb.articles[msgID] = &a
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

	s := nnn.NewServer(&testBackend)

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
