package common

import (
	"math/rand"
	"testing"
	"time"
)

func TestAA(t *testing.T) {
	s := &HighLowSlice{MaxSize: 4}
	rand.Seed(time.Now().Unix())
	for i := 0; i < 10; i++ {
		s.Append(&ArticleRef{Offset: int64(i)})
		t.Log(s)
	}

	for i := -10; i < 15; i++ {
		start, end := i+rand.Intn(s.Len()), i+rand.Intn(s.Len())
		if start > end {
			end, start = start, end
		}

		x, s, e := s.Slice(start, end, false)
		t.Log(x, start, "->", s, "\t", end, "->", e)
	}
}
