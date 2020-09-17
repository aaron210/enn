package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/backend"
	"github.com/coyove/nnn/server/common"
)

var ServerName = flag.String("name", "10.94.86.99", "")
var MaxPostSize = flag.Int64("max-post-size", 1024*1024*3, "")
var Listen = flag.String("l", ":1119 :1563 :8080", "")

const maxArticles = 100

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ltime | log.Lmicroseconds | log.Ldate)

	backend.GetMaxPostSize = func(b *backend.Backend) int64 {
		return *MaxPostSize
	}

	b := &backend.Backend{}
	common.PanicErr(LoadIndex("testdb", b), "load data source: %%err")

	s := nnn.NewServer(b)
	handle := func(l net.Listener) {
		for {
			c, err := l.Accept()
			if err != nil {
				log.Println("handle:", err)
				return
			}
			go s.Process(c)
		}
	}

	var plainBind, tlsBind, httpBind string
	fmt.Sscanf(*Listen, "%s %s %s", &plainBind, &tlsBind, &httpBind)
	log.Printf("bind: plain=%q, tls=%q, http=%q", plainBind, tlsBind, httpBind)

	if plainBind != "" {
		a, err := net.ResolveTCPAddr("tcp", plainBind)
		common.PanicErr(err, "error resolving listener: %v", err)
		l, err := net.ListenTCP("tcp", a)
		common.PanicErr(err, "error listening: %v", err)

		go handle(l)
	}

	if tlsBind != "" {
		cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
		common.PanicErr(err, "error loading TLS cert: %v", err)

		l, err := tls.Listen("tcp", tlsBind, &tls.Config{Certificates: []tls.Certificate{cert}})
		common.PanicErr(err, "error setting up TLS listener: %v", err)

		go handle(l)
	}

	select {}
}
