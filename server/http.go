package main

import (
	"fmt"
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

func HandleGroups(w http.ResponseWriter, r *http.Request) {
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
		}
	}

	sort.Slice(payload, func(i, j int) bool {
		return payload[i].LastArticleTime > payload[j].LastArticleTime
	})

	tb := font.Textbox{
		Width:     500,
		Margin:    10,
		LineSpace: 4,
		CharSpace: 1,
	}

	tb.Begin()
	tb.Write(fmt.Sprintf("%s/%s\n运行:%v 更新:%v TLS:%v\nMod:",
		*ServerName, *Listen,
		time.Since(startAt)/1e9*1e9, time.Now().Format("060102150405"),
		x509cert.NotAfter.Format("060102"),
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
			tb.Strikeline = true
			tb.Write(strings.Repeat(" ", len(timeFormat)))
			tb.Strikeline = false
		}
		tb.Gray = false

		tb.Write("  ")
		tb.Strikeline = g.Group.Posting == enn.PostingNotPermitted
		tb.Bold = true
		tb.Write(g.BaseGroupInfo.Name)
		tb.Strikeline = false
		tb.Bold = false
		tb.Write("  ")

		tb.Write(strconv.FormatInt(g.Group.Count, 10))
		tb.Write("篇 ")
		tb.Gray = true
		tb.Write(fmt.Sprintf("-%d(%d) ", g.Low, g.MaxLives))
		tb.Gray = false

		if g.MaxPostSize != 0 {
			tb.Underline = true
			tb.Write("限发文大小:" + common.FormatSize(g.MaxPostSize))
			tb.Underline = false
			tb.Write(" ")
		}

		tb.Write("\n")

		if g.LastArticleSub != "" {
			tb.Green = true
			tb.Indent = len(timeFormat) + 2
			tb.Write("最新: " + g.LastArticleSub)
			tb.Green = false
			tb.Indent = 0
			tb.Write("\n")
		}

		if g.Description != "" {
			tb.Blue = true
			tb.Indent = len(timeFormat) + 2
			tb.Write(g.Description)
			tb.Blue = false
			tb.Indent = 0
			tb.Write("\n")
		}

	}

	w.Header().Add("Content-Type", "image/png")
	tb.End(w)
}
