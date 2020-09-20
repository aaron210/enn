package enn

import (
	"fmt"
	"io"
	"math"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/enn/server/common"
)

// PostingStatus type for groups.
type PostingStatus byte

// PostingStatus values.
const (
	Unknown             = PostingStatus(0)
	PostingPermitted    = PostingStatus('y')
	PostingNotPermitted = PostingStatus('n')
	PostingModerated    = PostingStatus('m')
)

func (ps PostingStatus) String() string {
	switch ps {
	case PostingModerated:
		return "Mod"
	case PostingNotPermitted:
		return "Closed"
	case PostingPermitted, Unknown:
		return "Open"
	}
	return fmt.Sprintf("%c", ps)
}

// Group represents a usenet newsgroup.
type Group struct {
	Name        string
	Description string
	Count       int64
	High        int64
	Low         int64
	Posting     PostingStatus
}

// An Article that may appear in one or more groups.
type Article struct {
	// The article's headers
	Header textproto.MIMEHeader
	// The article's body
	Body io.Reader
	// Number of bytes in the article body (used by OVER/XOVER)
	Bytes int
	// Number of lines in the article body (used by OVER/XOVER)
	Lines      int
	RemoteAddr net.Addr
}

// MessageID provides convenient access to the article's Message ID.
func (a *Article) MessageID() string {
	return a.Header.Get("Message-Id")
}

// An NNTPError is a coded NNTP error message.
type NNTPError struct {
	Code int
	Msg  string
}

// ErrNoSuchGroup is returned for a request for a group that can't be found.
var ErrNoSuchGroup = &NNTPError{411, "No such newsgroup"}

// ErrNoSuchGroup is returned for a request that requires a current
// group when none has been selected.
var ErrNoGroupSelected = &NNTPError{412, "No newsgroup selected"}

// ErrInvalidMessageID is returned when a message is requested that can't be found.
var ErrInvalidMessageID = &NNTPError{430, "No article with that message-id"}

// ErrInvalidArticleNumber is returned when an article is requested that can't be found.
var ErrInvalidArticleNumber = &NNTPError{423, "No article with that number"}

// ErrNoCurrentArticle is returned when a command is executed that
// requires a current article when one has not been selected.
var ErrNoCurrentArticle = &NNTPError{420, "Current article number is invalid"}

// ErrUnknownCommand is returned for unknown comands.
var ErrUnknownCommand = &NNTPError{500, "Unknown command"}

// ErrSyntax is returned when a command can't be parsed.
var ErrSyntax = &NNTPError{501, "not supported, or syntax error"}

// ErrPostingNotPermitted is returned as the response to an attempt to
// post an article where posting is not permitted.
var ErrPostingNotPermitted = &NNTPError{440, "Posting not permitted"}

// ErrPostingFailed is returned when an attempt to post an article fails.
var ErrPostingFailed = &NNTPError{441, "posting failed"}

var ErrPostingTooLarge = &NNTPError{441, "posting large article"}

// ErrNotWanted is returned when an attempt to post an article is
// rejected due the server not wanting the article.
var ErrNotWanted = &NNTPError{435, "Article not wanted"}

// ErrAuthRequired is returned to indicate authentication is required
// to proceed.
var ErrAuthRequired = &NNTPError{450, "authorization required"}

// ErrAuthRejected is returned for invalid authentication.
var ErrAuthRejected = &NNTPError{452, "authorization rejected"}

// ErrNotAuthenticated is returned when a command is issued that requires
// authentication, but authentication was not provided.
var ErrNotAuthenticated = &NNTPError{480, "authentication required"}

var ErrServerBad = &NNTPError{500, "Server bad"}

var ErrNotMod = &NNTPError{Code: 441, Msg: "Not moderator"}

// Handler is a low-level protocol handler
type Handler func(args []string, s *session, c *textproto.Conn) error

// A NumberedArticle provides local sequence nubers to articles When
// listing articles in a group.
type NumberedArticle struct {
	Num     int64
	Article *Article
}

// The Backend that provides the things and does the stuff.
type Backend interface {
	ListGroups(max int) ([]*Group, error)
	GetGroup(name string) (*Group, error)
	GetArticle(group *Group, id string, headerOnly bool) (*Article, error)
	GetArticles(group *Group, from, to int64, headerOnly bool) ([]NumberedArticle, error)
	Authorized() bool
	// Authenticate and optionally swap out the backend for this session.
	// You may return nil to continue using the same backend.
	Authenticate(user, pass string) (Backend, error)
	AllowPost() bool
	Post(article *Article) error
}

type session struct {
	server  *Server
	backend Backend
	group   *Group
	conn    net.Conn

	throtTimer time.Time
}

// The Server handle.
type Server struct {
	// Handlers are dispatched by command name.
	Handlers map[string]Handler
	// The backend (your code) that provides data
	Backend Backend
	// The currently selected group.
	group *Group

	ThrotCmdInterval time.Duration
	ThrotCmdWindow   time.Duration
}

// NewServer builds a new server handle request to a backend.
func NewServer(backend Backend) *Server {
	rv := Server{
		Handlers:         make(map[string]Handler),
		Backend:          backend,
		ThrotCmdInterval: time.Second,
		ThrotCmdWindow:   time.Second * 5,
	}
	rv.Handlers["quit"] = handleQuit
	rv.Handlers["group"] = handleGroup
	rv.Handlers["list"] = handleList
	rv.Handlers["head"] = handleHead
	rv.Handlers["body"] = handleBody
	rv.Handlers["article"] = handleArticle
	rv.Handlers["post"] = handlePost
	rv.Handlers["ihave"] = handleIHave
	rv.Handlers["capabilities"] = handleCap
	rv.Handlers["mode"] = handleMode
	rv.Handlers["authinfo"] = handleAuthInfo
	rv.Handlers["newgroups"] = handleNewGroups
	rv.Handlers["over"] = handleOver
	rv.Handlers["xover"] = handleOver
	rv.Handlers["stat"] = handleStat
	return &rv
}

func (e *NNTPError) Error() string {
	return fmt.Sprintf("%d %s", e.Code, e.Msg)
}

func (s *session) dispatchCommand(cmd string, args []string, c *textproto.Conn) (err error) {
	cmd = strings.ToLower(cmd)

	handler, found := s.server.Handlers[cmd]
	if !found {
		common.E("unknown command: %v %v", cmd, args)
		handler = handleDefault
	}
	return handler(args, s, c)
}

// Process an NNTP session.
func (s *Server) Process(nc net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			common.E("panic: %v: %v", nc.RemoteAddr(), r)
		}
		nc.Close()
	}()

	c := textproto.NewConn(nc)

	sess := &session{
		server:     s,
		backend:    s.Backend,
		group:      nil,
		conn:       nc,
		throtTimer: time.Now(),
	}

	c.PrintfLine("200 Hello!")
	for {
		l, err := c.ReadLine()
		if err != nil {
			if err != io.EOF {
				common.E("client.ReadLine: %v: %v", nc.RemoteAddr(), err)
			}
			return
		}
		cmd := strings.Split(l, " ")
		// common.L("%v", cmd)
		args := []string{}
		if len(cmd) > 1 {
			args = cmd[1:]
		}

		if now := time.Now(); sess.throtTimer.Sub(now) < s.ThrotCmdWindow {
			if sess.throtTimer.Before(now) {
				sess.throtTimer = now
			}
			sess.throtTimer = sess.throtTimer.Add(s.ThrotCmdInterval)
		} else {
			wait := sess.throtTimer.Add(-s.ThrotCmdWindow).Sub(now)
			if wait > time.Millisecond*250 {
				common.L("%v: throt wait %v", nc.RemoteAddr(), wait)
				time.Sleep(wait)
			}
		}

		if err := sess.dispatchCommand(cmd[0], args, c); err != nil {
			switch _, isNNTPError := err.(*NNTPError); {
			case err == io.EOF:
				return
			case isNNTPError:
				c.PrintfLine(err.Error())
			default:
				common.E("%q at %v: %v", cmd, nc.RemoteAddr(), err)
				return
			}
		}
	}
}

func parseRange(spec string) (low, high int64) {
	if spec == "" {
		return 0, math.MaxInt64
	}
	parts := strings.Split(spec, "-")
	if len(parts) == 1 {
		h, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			h = math.MaxInt64
		}
		return 0, h
	}
	l, _ := strconv.ParseInt(parts[0], 10, 64)
	h, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		h = math.MaxInt64
	}
	return l, h
}
