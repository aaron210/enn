package common

import (
	"fmt"
	"math/rand"
	"sync"
)

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
