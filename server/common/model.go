package common

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net/textproto"
)

type ArticleRef struct {
	MsgID  string
	Offset int64
	Length int64
}

func (ar *ArticleRef) String() string {
	return fmt.Sprintf("<%s:%d-%d>", ar.MsgID, ar.Offset, ar.Offset+ar.Length)
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
	Name     string
	Desc     string
	Announce string
	Silence  bool
}
