package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log"
	"os"
	"strconv"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/backend"
	"github.com/coyove/nnn/server/common"
)

func LoadIndex(path string, db *backend.Backend) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return err
	}
	df, err := os.OpenFile(path+".data", os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return err
	}

	db.Groups = map[string]*backend.Group{}
	db.Articles = map[string]*common.ArticleRef{}
	db.Mods = map[string]*common.ModInfo{}
	db.Index = f
	db.Data = df
	db.ServerName = *ServerName
	db.Init()

	const maxArticles = 100000

	rd := bufio.NewReader(f)
	for ln := 1; ; ln++ {
		line, _ := rd.ReadBytes('\n')
		if len(line) == 0 {
			break
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		switch line[0] {
		case 'G':
			if len(line) < 3 {
				log.Printf("#%d %q invalid G header\n", ln, line)
				continue
			}
			info := common.BaseGroupInfo{}
			if err := json.Unmarshal(line[1:], &info); err != nil {
				log.Printf("#%d %q invalid G header, json: %v\n", ln, line, err)
				continue
			}
			if info.Name == "" {
				log.Printf("#%d %q invalid G header, empty group name\n", ln, line)
				continue
			}
			gs := &backend.Group{
				Info: &nnn.Group{
					Name:        info.Name,
					Description: info.Desc,
				},
				Articles: &common.HighLowSlice{MaxSize: maxArticles},
			}
			if info.Silence {
				gs.Info.Posting = nnn.PostingNotPermitted
			}
			if old := db.Groups[info.Name]; old != nil {
				log.Printf("#%d UPDATE group: %s => %#v", ln, info.Name, info)
				old.Info = gs.Info
			} else {
				log.Printf("#%d load group: %s", ln, info.Name)
				db.Groups[info.Name] = gs
			}
		case 'A':
			if len(line) < 8 { // Agroup msgid offset length, 8 chars minimal
				log.Printf("#%d %q invalid A header\n", ln, line)
				continue
			}

			parts := bytes.Split(line[1:], []byte(" "))
			if len(parts) != 4 {
				log.Printf("#%d %q invalid A header, space not found\n", ln, line)
				continue
			}
			group, msgid, offsetbuf, lengthbuf := parts[0], parts[1], parts[2], parts[3]

			g := db.Groups[string(group)]
			if g == nil {
				log.Printf("#%d %q invalid A header, invalid group: %q\n", ln, line, group)
				continue
			}

			offset, err := strconv.ParseInt(string(offsetbuf), 36, 64)
			if err != nil {
				log.Printf("#%d %q invalid A header, invalid offset: %v\n", ln, line, err)
				continue
			}

			length, err := strconv.ParseInt(string(lengthbuf), 36, 64)
			if err != nil {
				log.Printf("#%d %q invalid A header, invalid length: %v\n", ln, line, err)
				continue
			}

			ar := &common.ArticleRef{}
			ar.MsgID = string(msgid)
			ar.Offset = offset
			ar.Length = length

			g.Append(db, ar)
			db.Articles[ar.MsgID] = ar
		case 'M':
			db.MaxLiveArticels, _ = strconv.Atoi(string(line[1:]))
			log.Printf("#%d max live articles: %d", ln, db.MaxLiveArticels)
			if db.MaxLiveArticels > 0 {
				for _, g := range db.Groups {
					g.Articles.MaxSize = db.MaxLiveArticels
				}
			}
		case 'D':
			msgid := string(line[1:])
			delete(db.Articles, msgid)
		case 'm':
			mi := &common.ModInfo{}
			if err := json.Unmarshal(line[1:], mi); err != nil {
				log.Printf("#%d %q invalid mod info: %v\n", ln, line, err)
				continue
			}
			log.Printf("#%d load mod info: %#v\n", ln, mi)
			db.Mods[mi.Email] = mi
		case 'c':
			log.Printf("#%d remove mod info: %q\n", ln, line[1:])
			delete(db.Mods, string(line[1:]))
		}
	}

	// Finishing up
	for _, g := range db.Groups {
		g.Info.Count = int64(g.Articles.Len())
		g.Info.High = int64(g.Articles.High())
		g.Info.Low = int64(g.Articles.Low() + 1)
	}
	log.Printf("loader: %d groups, %d articles, %d mods", len(db.Groups), len(db.Articles), len(db.Mods))
	return nil
}
