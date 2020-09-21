package common

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"sync"
)

type HighLowSlice struct {
	mu            sync.RWMutex
	d             []*ArticleRef
	MaxSize       int
	NoPurgeNotify bool
	high, low     int
}

func (s *HighLowSlice) Len() int { return s.high }

func (s *HighLowSlice) High() int { return s.high }

func (s *HighLowSlice) Low() int { return s.low }

func (s *HighLowSlice) String() string {
	return fmt.Sprintf("[%v~%v max:%v data:%v", s.low, s.high, s.MaxSize, s.d)
}

func (s *HighLowSlice) Get(i int) (*ArticleRef, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i < s.low || i >= s.high {
		return nil, false
	}
	i -= s.low
	return s.d[i], true
}

func (s *HighLowSlice) Set(i int, v *ArticleRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i < s.low {
		return
	}
	i -= s.low
	s.d[i] = v
}

func (s *HighLowSlice) Slice(i, j int, copy bool) (results []*ArticleRef, actualStart, actualEnd int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if j <= s.low {
		return
	}
	if j > s.high {
		j = s.high
	}
	j -= s.low

	if i < s.low {
		i = 0
	} else {
		i -= s.low
	}

	if i > j {
		return
	}

	if copy {
		results = append([]*ArticleRef{}, s.d[i:j]...)
	} else {
		results = s.d[i:j]
	}

	actualStart = i + s.low
	actualEnd = j + s.low
	return
}

func (s *HighLowSlice) Append(v *ArticleRef) ([]*ArticleRef, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.d = append(s.d, v)
	s.high++

	var purged []*ArticleRef
	if s.MaxSize > 0 && len(s.d) > s.MaxSize {
		p := 1 / float64(len(s.d)-s.MaxSize+1)
		if rand.Float64() > p {
			x := len(s.d) - s.MaxSize
			purged = append([]*ArticleRef{}, s.d[:x]...)

			s.low += x
			copy(s.d, s.d[x:])
			s.d = s.d[:s.MaxSize]
		}
	}
	if s.NoPurgeNotify {
		purged = nil
	}
	return purged, s.high
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
