package main

import (
	"html/template"
	"net/http"
	"sort"
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

func HandleGroups(w http.ResponseWriter, r *http.Request) {
	groups, _ := db.ListGroups(0)
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	w.Header().Add("Content-Type", "text/html")
	template.Must(template.New("").Parse(`
<meta charset="utf-8">
<table border=1>
<tr>
<th>Name</th><th>Desc</th><th>Articles</th><th>Status</th>
</tr>
{{range .}}
<tr>
<td><a href="/group?name={{.Name}}">{{.Name}}</a></td><td>{{.Description}}</td><td>{{.Count}}</td><td>{{.Posting}}</td>
</tr>
{{end}}
</table>
`)).Execute(w, groups)
}
