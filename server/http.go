package main

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/common/dateparse"
)

func HandleGroup(w http.ResponseWriter, r *http.Request) {
	// 	name := r.FormValue("name")
	// 	page, _ := strconv.Atoi(f.FormValue("p"))
	// 	n, _ := strconv.Atoi(f.FormValue("n"))
	// 	if n == 0 {
	// 		n = 10
	// 	}
	//
	// 	info, err := db.GetGroup(name)
	// 	if err != nil {
	// 		log.Println("http group:", name, err)
	// 		w.WriteHeader(500)
	// 		return
	// 	}
	//
	// 	totalpage := int(math.Ceil(float64(info.Count) / float64(n)))
	// 	if page < 1 {
	// 		page = 1
	// 	}
	// 	if page > totalpage {
	// 		page = totalpage
	// 	}
	// 	start, end := (page-1)*n, page*n
	// 	if end > int(info.Count) {
	// 		end = int(info.Count)
	// 	}
	// 	start, end = int(info.Count)-1-start, int(info.Count)-end
	//
	// 	w.Header().Add("Content-Type", "text/html")
	// 	template.Must(template.New("").Parse(`
	// <meta charset="utf-8">
	// {{.High}}
	// `)).Execute(w, info)
}

var indexPage = template.Must(template.New("").Funcs(template.FuncMap{
	"size": func(v int64) string {
		if v > 1000*1000 {
			return fmt.Sprintf("%.2fM", float64(v)/1e6)
		}
		if v > 1000 {
			return fmt.Sprintf("%.2fK", float64(v)/1e3)
		}
		if v == 0 {
			return "Default"
		}
		return fmt.Sprintf("%d", v)
	},
}).Parse(`
<meta charset="utf-8">
<style> 
    .ptable {
	font-family: lucida sans unicode,lucida grande,Sans-Serif;
	background: #fff;
	width: 100%;
	border-collapse: collapse;
	text-align: left;
    }
    .ptable th {
	font-size: 14px;
	font-weight: 400;
	color: #039;
	border-bottom: 2px solid #6678b1;
	padding: 10px 8px
    }
    .ptable td {
	border-bottom: 1px solid #ccc;
	color: #669;
	padding: 6px 8px
    }
    .ptable tbody tr:hover td {
	color: #009
    }
.ptable [nowrap] {
width: 1%;
white-space: nowrap;
}
.ptable span.low {
color: #ccc;
}
.ptable td input {
font: inherit;
border: none;
width: 100%;
}
</style>
<table class="ptable">
<tr>
	<th>Name</th>
	<th>Description</th>
	<th nowrap>Max Post Size</th>
	<th nowrap>Articles / <span class=low>Low</span></th>
	<th nowrap>Latest</th>
</tr>
{{range .}}
<tr>
	{{$disabled := eq .Posting 'n'}}
	<td nowrap>{{if $disabled}}<s>{{end}}{{.Name}}</td>
	<td><input readonly value="{{.Description}}"></td>
	<td nowrap>{{size .MaxPostSize}}</td>
	<td nowrap>{{.Count}} / <span class=low>{{.Low}}</span></td>
	<td nowrap>{{.LastArticleTime}}</td>
</tr>
{{end}}
</table>
`))

func HandleGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := db.ListGroups(0)

	payload := make([]struct {
		LastArticleTime string
		*nnn.Group
	}, len(groups))
	for i := range payload {
		g := groups[i]
		payload[i].Group = g
		if g.High == 0 {
			continue
		}

		a, _ := db.GetArticle(g, strconv.FormatInt(g.High, 10))
		if a != nil {
			payload[i].LastArticleTime = a.Header.Get("Date")
			t, err := dateparse.ParseAny(payload[i].LastArticleTime)
			if err == nil {
				payload[i].LastArticleTime = t.Format("2006-01-02 15:04")
			}
		}
	}

	sort.Slice(payload, func(i, j int) bool {
		return payload[i].LastArticleTime > payload[j].LastArticleTime
	})

	w.Header().Add("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf("<title>%s</title>", *ServerName)))
	indexPage.Execute(w, payload)
}
