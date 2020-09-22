package common

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var Verbose = 1

func D(f string, a ...interface{}) {
	if Verbose <= 0 {
		logger(0, f, a...)
	}
}

func L(f string, a ...interface{}) {
	if Verbose <= 1 {
		logger(1, f, a...)
	}
}

func E(f string, a ...interface{}) {
	if Verbose <= 2 {
		logger(2, f, a...)
	}
}

func F(f string, a ...interface{}) {
	logger(3, f, a...)
	os.Exit(3)
}

func logger(lv int, f string, a ...interface{}) {
	_, file, ln, _ := runtime.Caller(2)
	file = filepath.Base(file)

	lead := "I"
	switch lv {
	case 1:
		lead = "M"
	case 2:
		lead = "e"
	}

	fmt.Printf("%s %s % 12s:%03d] ", lead, time.Now().Format("0102 15:04:05.000"), file, ln)
	fmt.Println(fmt.Sprintf(f, a...))
}
