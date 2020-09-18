package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/backend"
	"github.com/coyove/nnn/server/common"
)

var (
	DBPath         = flag.String("db", "testdb", "")
	ServerName     = flag.String("name", "10.94.86.99", "")
	Listen         = flag.String("l", ":1119 :1563 :8080", "")
	GroupAdd       = flag.String("group-create", "", "name,desc")
	GroupSilence   = flag.String("group-post", "", "name")
	GroupMaxPostSz = flag.String("group-mps", "", "")
	ModAdd         = flag.String("mod-add", "", "email,pass")
	ModDel         = flag.String("mod-del", "", "email")
)

var db = &backend.Backend{}

type authObject struct {
	user, pass string
}

func (obj *authObject) isMod(db *backend.Backend) bool {
	if obj == nil {
		return false
	}
	mi := db.Mods[obj.user]
	if mi == nil {
		return false
	}
	return mi.Password == obj.pass
}

func main() {
	rand.Seed(time.Now().Unix())
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ltime | log.Lmicroseconds | log.Ldate)

	backend.ImplMaxPostSize = func(b *backend.Backend) int64 {
		return 1024 * 1024 * 3
	}
	backend.ImplAuth = func(b *backend.Backend, user, pass string) error {
		obj, _ := b.AuthObject.(*authObject)
		if obj == nil && user != "" && pass != "" {
			b.AuthObject = &authObject{user, pass}
		}
		return nil
	}
	backend.ImplIsMod = func(b *backend.Backend) bool {
		obj, _ := b.AuthObject.(*authObject)
		return obj.isMod(db)
	}

	common.PanicIf(LoadIndex(*DBPath, db), "load data source %v: %%err", *DBPath)

	if HandleCommand() {
		return
	}

	s := nnn.NewServer(db)
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
		common.PanicIf(err, "error resolving listener: %v", err)
		l, err := net.ListenTCP("tcp", a)
		common.PanicIf(err, "error listening: %v", err)

		go handle(l)
	}

	if tlsBind != "" {
		cert, err := tls.LoadX509KeyPair("cert.pem", "key.pem")
		common.PanicIf(err, "error loading TLS cert: %v", err)

		l, err := tls.Listen("tcp", tlsBind, &tls.Config{Certificates: []tls.Certificate{cert}})
		common.PanicIf(err, "error setting up TLS listener: %v", err)

		go handle(l)
	}

	if httpBind != "" {
		http.HandleFunc("/", HandleGroups)
		http.HandleFunc("/group", HandleGroup)
		go http.ListenAndServe(httpBind, nil)
	}

	select {}
}
