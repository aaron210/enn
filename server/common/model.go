package common

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
)

type ArticleRef struct {
	Index    int
	RawMsgID [16]byte
	Offset   int64
	Length   int64
}

func (ar *ArticleRef) MsgID() string {
	return string(bytes.Trim(ar.RawMsgID[:], "\x00"))
}

func (ar *ArticleRef) String() string {
	return fmt.Sprintf("<%s:%d-%d>", ar.MsgID(), ar.Offset, ar.Offset+ar.Length)
}

type Article struct {
	Headers textproto.MIMEHeader
	Body    []byte
	Refer   []string
}

func (a *Article) Unmarshal(rd io.Reader, headerOnly bool) error {
	dec := gob.NewDecoder(rd)
	if err := dec.Decode(&a.Headers); err != nil {
		return err
	}
	if headerOnly {
		return nil
	}
	if err := dec.Decode(&a.Body); err != nil {
		return err
	}
	if err := dec.Decode(&a.Refer); err != nil {
		return err
	}
	return nil
}

func (a *Article) Marshal() []byte {
	buf := &bytes.Buffer{}
	enc := gob.NewEncoder(buf)
	enc.Encode(a.Headers)
	enc.Encode(a.Body)
	enc.Encode(a.Refer)
	return buf.Bytes()
}

type BaseGroupInfo struct {
	Name        string `json:",omitempty"`
	Desc        string `json:",omitempty"`
	Posting     int64  `json:",omitempty"`
	MaxPostSize int64  `json:",omitempty"`
	MaxLives    int64  `json:",omitempty"`
	CreateTime  int64  `json:",omitempty"`
}

func (g BaseGroupInfo) Diff(g2 *BaseGroupInfo) string {
	if g.Name == g2.Name {
		g.Name = ""
	}
	if g.Desc == g2.Desc {
		g.Desc = ""
	}
	if g.Posting == g2.Posting {
		g.Posting = 0
	}
	if g.MaxPostSize == g2.MaxPostSize {
		g.MaxPostSize = 0
	}
	if g.MaxLives == g2.MaxLives {
		g.MaxLives = 0
	}
	if g.CreateTime == g2.CreateTime {
		g.CreateTime = 0
	}
	buf, _ := json.Marshal(g)
	return string(buf)
}

type ModInfo struct {
	Email    string
	Password string
	Deleted  bool
}

func (m *ModInfo) String() string {
	buf, _ := json.Marshal(m)
	return string(buf)
}

type AuthObject struct {
	User, Pass string
}

type Config struct {
	MaxPostSize     int64
	ThrotCmdWin     int64
	PostIntervalSec int64
}
