package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/enn"
	"github.com/coyove/enn/server/common"
	"github.com/coyove/enn/server/common/dateparse"
	"github.com/coyove/enn/server/common/font"
)

var HandleGroups = func() func(w http.ResponseWriter, r *http.Request) {
	var lastBuffer []byte
	var interval = time.Second * 2

	go func() {
		for {
			time.Sleep(interval + time.Duration(rand.Intn(500))*time.Millisecond)
			lastBuffer = generateStatus()
		}
	}()

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "image/png")
		w.Header().Add("Cache-Control", "max-age="+strconv.Itoa(int(interval/time.Second)))
		w.Write(lastBuffer)
	}
}()

func getListenPort(addr string) int {
	a, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return 0
	}
	return a.Port
}

func generateStatus() []byte {
	const timeFormat = "2006-01-02 15:04"

	groups, _ := db.ListGroups(0)

	payload := make([]struct {
		LastArticleTime string
		LastArticleSub  string
		*enn.Group
		*common.BaseGroupInfo
	}, len(groups))

	for i := range payload {
		g := groups[i]
		payload[i].Group = g
		payload[i].BaseGroupInfo = db.Groups[g.Name].BaseInfo

		if g.High == 0 {
			continue
		}

		a, _ := db.GetArticle(g, strconv.FormatInt(g.High, 10), true)
		if a != nil {
			payload[i].LastArticleTime = a.Header.Get("Date")

			sub := a.Header.Get("Subject")
			sub = common.TranslateEncoding(sub)
			payload[i].LastArticleSub = sub

			t, err := dateparse.ParseAny(payload[i].LastArticleTime)
			if err == nil {
				payload[i].LastArticleTime = t.Format(timeFormat)
			}
			if tmp := payload[i].LastArticleTime; len(tmp) > len(timeFormat) {
				payload[i].LastArticleTime = tmp[:len(timeFormat)]
			}
		}
	}

	sort.Slice(payload, func(i, j int) bool {
		return payload[i].LastArticleTime > payload[j].LastArticleTime
	})

	tb := font.Textbox{
		Width:     600,
		Margin:    10,
		LineSpace: 4,
		CharSpace: 1,
	}

	tb.Begin()
	tb.Underline = true
	tb.Write("桌面客户端：").Wgreen("Thunderbird").
		Write("，iOS：").Wgreen("Newsy").
		Write("，Android：").Wgreen("net.piaohong.newsgroup").
		Write("\n\n")
	tb.Underline = false

	tb.Write("服务器:").Wu(*ServerName).
		Write(" 端口:").Wu(strconv.Itoa(getListenPort(plainBind))).
		Write(" SSL/TLS端口:").Wu(strconv.Itoa(getListenPort(tlsBind))).
		Write(" 编码:").Wu("UTF-8").
		Write("\n\n").
		Write(fmt.Sprintf("运行:%v 更新:%v 证书:%v\nMod:",
			time.Since(startAt)/1e9*1e9,
			time.Now().Format(timeFormat),
			x509cert.NotAfter.Format(timeFormat),
		))

	for k := range db.Mods {
		tb.Write(" ")
		tb.Write(k)
	}
	tb.Write("\n\n")

	for _, g := range payload {
		tb.Gray = true
		if g.LastArticleTime != "" {
			tb.Write(g.LastArticleTime)
		} else {
			tb.Ws(strings.Repeat(" ", len(timeFormat)))
		}
		tb.Gray = false

		tb.Write("  ")
		tb.Strikeline = g.Group.Posting == enn.PostingNotPermitted
		tb.Wb(g.BaseGroupInfo.Name)
		tb.Strikeline = false
		tb.Write("  ")

		tb.Write(strconv.FormatInt(g.Group.Count, 10)).Write("篇 ")
		tb.Wgray(fmt.Sprintf("-%d(%d) ", g.Low, g.MaxLives))

		if g.MaxPostSize != 0 {
			tb.Underline = true
			tb.Write("限发文大小:" + common.FormatSize(g.MaxPostSize))
			tb.Underline = false
			tb.Write(" ")
		}

		tb.Write("\n")

		if g.LastArticleSub != "" {
			tb.Indent = len(timeFormat) + 2
			tb.Wgreen("最新: " + g.LastArticleSub)
			tb.Indent = 0
			tb.Write("\n")
		}

		if g.Description != "" {
			tb.Indent = len(timeFormat) + 2
			tb.Wblue(g.Description)
			tb.Indent = 0
			tb.Write("\n")
		}

	}

	w := &bytes.Buffer{}
	tb.End(w)
	return w.Bytes()
}
