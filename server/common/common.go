package common

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

func IntIf(a, b int64) int64 {
	if a == 0 {
		return b
	}
	return a
}

func PanicIf(err interface{}, f string, a ...interface{}) {
	if v, ok := err.(bool); ok {
		if v {
			F(f, a...)
		}
		return
	}
	if err != nil {
		f = strings.Replace(f, "%%err", strings.Replace(fmt.Sprint(err), "%", "%%", -1), -1)
		F(f, a...)
	}
}

func ExtractMsgID(msgID string) string {
	if strings.HasPrefix(msgID, "<") && strings.HasSuffix(msgID, ">") {
		msgID = msgID[1 : len(msgID)-1]
		msgID = strings.Split(msgID, "@")[0]
	}
	return msgID
}

func ExtractEmail(from string) string {
	start, end := strings.Index(from, "<"), strings.Index(from, ">")
	if start > -1 && end > -1 && end > start {
		return from[start+1 : end]
	}
	return ""
}

func MsgIDToRawMsgID(msgid string, msgidbuf []byte) [16]byte {
	var x [16]byte
	if msgidbuf != nil {
		copy(x[:], msgidbuf)
	} else {
		copy(x[:], msgid)
	}
	return x
}

func FormatSize(v int64) string {
	if v > 1000*1000 {
		return fmt.Sprintf("%.2fM", float64(v)/1e6)
	}
	if v > 1000 {
		return fmt.Sprintf("%.2fK", float64(v)/1e3)
	}
	return fmt.Sprintf("%dB", v)
}

var TranslateEncoding = func() func(string) string {
	// https://tools.ietf.org/html/rfc1342
	var r = regexp.MustCompile(`(?i)=\?UTF-8\?B\?(\S+)\?=`)
	var rq = regexp.MustCompile(`(?i)=\?UTF-8\?Q\?(\S+)\?=`)
	var rq2 = regexp.MustCompile(`(?i)(=[0-9a-f]{2})`)

	return func(in string) string {
		outs := r.FindAllStringSubmatch(in, -1)
		if len(outs) == 1 && len(outs[0]) == 2 {
			buf, err := base64.StdEncoding.DecodeString(outs[0][1])
			if err == nil {
				return string(buf)
			}
		}
		outs = rq.FindAllStringSubmatch(in, -1)
		if len(outs) == 1 && len(outs[0]) == 2 {
			return string(rq2.ReplaceAllFunc([]byte(outs[0][1]), func(in []byte) []byte {
				hex.Decode(in, in[1:])
				return in[:1]
			}))
		}
		return in
	}
}()
