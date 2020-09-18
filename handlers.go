package enn

import (
	"fmt"
	"io"
	"net/textproto"
	"strings"
)

func handleStat(args []string, s *session, c *textproto.Conn) error {
	if s.group == nil {
		return ErrNoGroupSelected
	}
	if len(args) != 1 {
		return ErrSyntax
	}
	a, err := s.backend.GetArticle(s.group, args[0], true)
	if err != nil {
		return err
	}
	c.PrintfLine("223 %d %s", 0, a.MessageID())
	return nil
}

/*
   "0" or article number (see below)
   Subject header content
   From header content
   Date header content
   Message-ID header content
   References header content
   :bytes metadata item
   :lines metadata item
*/

func handleOver(args []string, s *session, c *textproto.Conn) error {
	if s.group == nil {
		return ErrNoGroupSelected
	}
	from, to := parseRange(args[0])
	articles, err := s.backend.GetArticles(s.group, from, to, true)
	if err != nil {
		return err
	}
	c.PrintfLine("224 here it comes")
	dw := c.DotWriter()
	defer dw.Close()
	for _, a := range articles {
		fmt.Fprintf(dw, "%d\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n", a.Num,
			a.Article.Header.Get("Subject"),
			a.Article.Header.Get("From"),
			a.Article.Header.Get("Date"),
			a.Article.Header.Get("Message-Id"),
			a.Article.Header.Get("References"),
			a.Article.Bytes, a.Article.Lines)
	}
	return nil
}

func handleListOverviewFmt(c *textproto.Conn) error {
	err := c.PrintfLine("215 Order of fields in overview database.")
	if err != nil {
		return err
	}
	dw := c.DotWriter()
	defer dw.Close()
	_, err = fmt.Fprintln(dw, `Subject:
From:
Date:
Message-ID:
References:
:bytes
:lines`)
	return err
}

func handleList(args []string, s *session, c *textproto.Conn) error {
	ltype := "active"
	if len(args) > 0 {
		ltype = strings.ToLower(args[0])
	}

	if ltype == "overview.fmt" {
		return handleListOverviewFmt(c)
	}

	groups, err := s.backend.ListGroups(-1)
	if err != nil {
		return err
	}
	c.PrintfLine("215 list of newsgroups follows")
	dw := c.DotWriter()
	defer dw.Close()
	for _, g := range groups {
		switch ltype {
		case "active":
			fmt.Fprintf(dw, "%s %d %d %v\r\n", g.Name, g.High, g.Low, g.Posting)
		case "newsgroups":
			fmt.Fprintf(dw, "%s %s\r\n", g.Name, g.Description)
		}
	}

	return nil
}

func handleNewGroups(args []string, s *session, c *textproto.Conn) error {
	c.PrintfLine("231 list of newsgroups follows")
	c.PrintfLine(".")
	return nil
}

func handleDefault(args []string, s *session, c *textproto.Conn) error {
	return ErrUnknownCommand
}

func handleQuit(args []string, s *session, c *textproto.Conn) error {
	c.PrintfLine("205 bye")
	return io.EOF
}

func handleGroup(args []string, s *session, c *textproto.Conn) error {
	if len(args) < 1 {
		return ErrNoSuchGroup
	}

	group, err := s.backend.GetGroup(args[0])
	if err != nil {
		return err
	}

	s.group = group

	c.PrintfLine("211 %d %d %d %s", group.Count, group.Low, group.High, group.Name)
	return nil
}

func (s *session) getArticle(args []string, ho bool) (*Article, error) {
	if s.group == nil {
		return nil, ErrNoGroupSelected
	}
	return s.backend.GetArticle(s.group, args[0], ho)
}

/*
   Syntax
     HEAD message-id
     HEAD number
     HEAD


   First form (message-id specified)
     221 0|n message-id    Headers follow (multi-line)
     430                   No article with that message-id

   Second form (article number specified)
     221 n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     423                   No article with that number

   Third form (current article number used)
     221 n message-id      Headers follow (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid
*/

func handleHead(args []string, s *session, c *textproto.Conn) error {
	article, err := s.getArticle(args, true)
	if err != nil {
		return err
	}
	c.PrintfLine("221 1 %s", article.MessageID())
	dw := c.DotWriter()
	defer dw.Close()
	for k, v := range article.Header {
		fmt.Fprintf(dw, "%s: %s\r\n", k, v[0])
	}
	return nil
}

/*
   Syntax
     BODY message-id
     BODY number
     BODY

   Responses

   First form (message-id specified)
     222 0|n message-id    Body follows (multi-line)
     430                   No article with that message-id

   Second form (article number specified)
     222 n message-id      Body follows (multi-line)
     412                   No newsgroup selected
     423                   No article with that number

   Third form (current article number used)
     222 n message-id      Body follows (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid

   Parameters
     number        Requested article number
     n             Returned article number
     message-id    Article message-id
*/

func handleBody(args []string, s *session, c *textproto.Conn) error {
	article, err := s.getArticle(args, false)
	if err != nil {
		return err
	}
	c.PrintfLine("222 1 %s", article.MessageID())
	dw := c.DotWriter()
	defer dw.Close()
	_, err = io.Copy(dw, article.Body)
	return err
}

/*
   Syntax
     ARTICLE message-id
     ARTICLE number
     ARTICLE

   Responses

   First form (message-id specified)
     220 0|n message-id    Article follows (multi-line)
     430                   No article with that message-id

   Second form (article number specified)
     220 n message-id      Article follows (multi-line)
     412                   No newsgroup selected
     423                   No article with that number

   Third form (current article number used)
     220 n message-id      Article follows (multi-line)
     412                   No newsgroup selected
     420                   Current article number is invalid

   Parameters
     number        Requested article number
     n             Returned article number
     message-id    Article message-id
*/

func handleArticle(args []string, s *session, c *textproto.Conn) error {
	article, err := s.getArticle(args, false)
	if err != nil {
		return err
	}
	c.PrintfLine("220 1 %s", article.MessageID())
	dw := c.DotWriter()
	defer dw.Close()

	for k, v := range article.Header {
		fmt.Fprintf(dw, "%s: %s\r\n", k, v[0])
	}

	fmt.Fprintln(dw, "")

	_, err = io.Copy(dw, article.Body)
	return err
}

/*
   Syntax
     POST

   Responses

   Initial responses
     340    Send article to be posted
     440    Posting not permitted

   Subsequent responses
     240    Article received OK
     441    Posting failed
*/

func handlePost(args []string, s *session, c *textproto.Conn) error {
	if !s.backend.AllowPost() {
		return ErrPostingNotPermitted
	}

	c.PrintfLine("340 Input article; end with <CR-LF>.<CR-LF>")
	var err error
	var article Article
	article.Header, err = c.ReadMIMEHeader()
	if err != nil {
		return ErrPostingFailed
	}
	article.Body = c.DotReader()
	article.RemoteAddr = s.conn.RemoteAddr()
	err = s.backend.Post(&article)
	if err != nil {
		return err
	}
	c.PrintfLine("240 article received OK")
	return nil
}

func handleIHave(args []string, s *session, c *textproto.Conn) error {
	if !s.backend.AllowPost() {
		return ErrNotWanted
	}

	// XXX:  See if we have it.
	article, err := s.backend.GetArticle(nil, args[0], true)
	if article != nil {
		return ErrNotWanted
	}

	c.PrintfLine("335 send it")
	article = &Article{}
	article.Header, err = c.ReadMIMEHeader()
	if err != nil {
		return ErrPostingFailed
	}
	article.Body = c.DotReader()
	err = s.backend.Post(article)
	if err != nil {
		return err
	}
	c.PrintfLine("235 article received OK")
	return nil
}

func handleCap(args []string, s *session, c *textproto.Conn) error {
	c.PrintfLine("101 Capability list:")
	dw := c.DotWriter()
	defer dw.Close()

	fmt.Fprintf(dw, "VERSION 2\n")
	fmt.Fprintf(dw, "READER\n")
	if s.backend.AllowPost() {
		fmt.Fprintf(dw, "POST\n")
		fmt.Fprintf(dw, "IHAVE\n")
	}
	fmt.Fprintf(dw, "OVER\n")
	fmt.Fprintf(dw, "XOVER\n")
	fmt.Fprintf(dw, "LIST ACTIVE NEWSGROUPS OVERVIEW.FMT\n")
	return nil
}

func handleMode(args []string, s *session, c *textproto.Conn) error {
	if s.backend.AllowPost() {
		c.PrintfLine("200 Posting allowed")
	} else {
		c.PrintfLine("201 Posting prohibited")
	}
	return nil
}

func handleAuthInfo(args []string, s *session, c *textproto.Conn) error {
	if len(args) < 2 {
		return ErrSyntax
	}
	if strings.ToLower(args[0]) != "user" {
		return ErrSyntax
	}

	// if s.backend.Authorized() {
	// 	return c.PrintfLine("281 Authentication accepted")
	// }

	c.PrintfLine("381 Password required")

	a, err := c.ReadLine()
	parts := strings.SplitN(a, " ", 3)
	if strings.ToLower(parts[0]) != "authinfo" || strings.ToLower(parts[1]) != "pass" {
		return ErrSyntax
	}
	b, err := s.backend.Authenticate(args[1], parts[2])
	if err == nil {
		c.PrintfLine("281 Authentication accepted")
		if b != nil {
			s.backend = b
		}
	}
	return err
}
