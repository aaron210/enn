package common

import (
	"bytes"
	"encoding/gob"
	"fmt"
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
	Body    string
	Refer   []string
}

func (a *Article) Unmarshal(buf []byte) error {
	return gob.NewDecoder(bytes.NewReader(buf)).Decode(a)
}

func (a *Article) Marshal() []byte {
	buf := &bytes.Buffer{}
	gob.NewEncoder(buf).Encode(a)
	return buf.Bytes()
}

type BaseGroupInfo struct {
	Name        string `json:",omitempty"`
	Desc        string `json:",omitempty"`
	Announce    string `json:",omitempty"`
	Silence     bool   `json:",omitempty"`
	MaxPostSize int64  `json:",omitempty"`
}

type ModInfo struct {
	Email    string
	Password string
}
