package common

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

var Verbose = 0

func L(f string, a ...interface{}) {
	if Verbose >= 0 {
		logger(0, f, a...)
	}
}

func E(f string, a ...interface{}) {
	if Verbose >= 1 {
		logger(1, f, a...)
	}
}

func F(f string, a ...interface{}) {
	if Verbose >= 1 {
		logger(2, f, a...)
	}
	os.Exit(2)
}

func logger(lv int, f string, a ...interface{}) {
	_, file, ln, _ := runtime.Caller(2)
	file = filepath.Base(file)
	fmt.Printf("%d %s % 12s:%03d] ", lv, time.Now().Format("0102 15:04:05.000"), file, ln)
	fmt.Println(fmt.Sprintf(f, a...))
}
