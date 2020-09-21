package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/coyove/enn"
	"github.com/coyove/enn/server/common"
)

var (
	Certbot      = flag.String("certbot", "", "")
	DBPath       = flag.String("db", "testdb", "")
	NopDB        = flag.String("nop", "", "")
	ServerName   = flag.String("name", "10.94.86.99", "Server name")
	Listen       = flag.String("l", ":1119 :1563 :8080", "listen addresses")
	MaxPostSize  = flag.Int64("max-post-size", 1024*1024*3, "Global max post size (bytes)")
	GroupCmd     = flag.String("group", "", "")
	ModCmd       = flag.String("mod", "", "")
	BlacklistCmd = flag.String("blacklist", "", "")
)

var (
	db                           = &Backend{}
	startAt                      = time.Now()
	x509cert                     x509.Certificate
	plainBind, tlsBind, httpBind string
)

func main() {
	rand.Seed(time.Now().Unix())
	flag.Parse()

	common.PanicIf(LoadIndex(*DBPath, db), "load data source %v: %%err", *DBPath)

	if HandleCommand() {
		return
	}

	s := enn.NewServer(db)
	handle := func(l net.Listener) {
		for {
			c, err := l.Accept()
			if err != nil {
				common.E("handle: %v", err)
				continue
			}

			tcpaddr, ok := c.RemoteAddr().(*net.TCPAddr)
			if !ok {
				c.Close()
				common.E("handle addr: %v", c.RemoteAddr())
				continue
			}
			if db.IsBanned(tcpaddr.IP) {
				c.Close()
				common.E("handle banned IP: %v", c.RemoteAddr())
				continue
			}
			go s.Process(c)
		}
	}

	fmt.Sscanf(*Listen, "%s %s %s", &plainBind, &tlsBind, &httpBind)
	common.L("bind: plain=%q, tls=%q, http=%q", plainBind, tlsBind, httpBind)

	if plainBind != "" {
		a, err := net.ResolveTCPAddr("tcp", plainBind)
		common.PanicIf(err, "error resolving listener: %v", err)
		l, err := net.ListenTCP("tcp", a)
		common.PanicIf(err, "error listening: %v", err)

		go handle(l)
	}

	if tlsBind != "" {
		if ip := net.ParseIP(*ServerName); *ServerName == "" || *ServerName == "localhost" || len(ip) > 0 {
			common.L("invalid server name, TLS disabled")
			goto SKIP_TLS
		}

		dir := filepath.Join(*Certbot, *ServerName)
		common.L("load cert in %s", dir)

		cert, err := tls.LoadX509KeyPair(dir+"/fullchain.pem", dir+"/privkey.pem")
		common.PanicIf(err, "%%err")

		xc, err := x509.ParseCertificate(cert.Certificate[0])
		common.PanicIf(err, "%%err")
		x509cert = *xc

		l, err := tls.Listen("tcp", tlsBind, &tls.Config{Certificates: []tls.Certificate{cert}})
		common.PanicIf(err, "error setting up TLS listener: %v", err)

		go handle(l)
	}

SKIP_TLS:
	if httpBind != "" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })
		http.HandleFunc("/status.png", HandleGroups)
		go http.ListenAndServe(httpBind, nil)
	}

	select {}
}
