package common

import (
	"math/rand"
	"testing"
	"time"
)

func TestAA(t *testing.T) {
	s := &HighLowSlice{MaxSize: 2}
	rand.Seed(time.Now().Unix())
	for i := 0; i < 10; i++ {
		s.Append(i)
		t.Log(s)
	}

	for i := 0; i < 10; i++ {
		t.Log(s.Get(i))
	}
}
