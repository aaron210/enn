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

	db.Groups = map[string]*backend.Group{}
	db.Articles = map[[16]byte]*common.ArticleRef{}
	db.Mods = map[string]*common.ModInfo{}
	db.Index = f
	db.ServerName = *ServerName
	db.MaxLiveArticels = 1e5

	df0, err := os.OpenFile(path+".data.0", os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return err
	}
	db.Data = []*os.File{df0}

	for i := 1; ; i++ {
		df, err := os.OpenFile(path+".data."+strconv.Itoa(i), os.O_RDWR, 0777)
		if err != nil {
			break
		}
		db.Data = append(db.Data, df)
	}

	db.Init()

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
					MaxPostSize: info.MaxPostSize,
				},
				Articles: &common.HighLowSlice{MaxSize: db.MaxLiveArticels},
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
			if len(line) < 10 { // format: "Agroup msgid index offset length", 10 chars minimal
				log.Printf("#%d %q invalid A header\n", ln, line)
				continue
			}

			parts := bytes.Split(line[1:], []byte(" "))
			if len(parts) != 5 {
				log.Printf("#%d %q invalid A header, need 5 arguments\n", ln, line)
				continue
			}
			group, msgid, indexbuf, offsetbuf, lengthbuf := parts[0], parts[1], parts[2], parts[3], parts[4]

			g := db.Groups[string(group)]
			if g == nil {
				log.Printf("#%d %q invalid A header, invalid group: %q\n", ln, line, group)
				continue
			}

			ar := &common.ArticleRef{}

			ar.Index, err = strconv.Atoi(string(indexbuf))
			if err != nil {
				log.Printf("#%d %q invalid A header, invalid index: %v\n", ln, line, err)
				continue
			}

			ar.Offset, err = strconv.ParseInt(string(offsetbuf), 36, 64)
			if err != nil {
				log.Printf("#%d %q invalid A header, invalid offset: %v\n", ln, line, err)
				continue
			}

			ar.Length, err = strconv.ParseInt(string(lengthbuf), 36, 64)
			if err != nil {
				log.Printf("#%d %q invalid A header, invalid length: %v\n", ln, line, err)
				continue
			}

			ar.RawMsgID = common.MsgIDToRawMsgID("", msgid)
			g.Append(db, ar)
			db.Articles[ar.RawMsgID] = ar
		case 'M':
			db.MaxLiveArticels, _ = strconv.Atoi(string(line[1:]))
			log.Printf("#%d max live articles: %d", ln, db.MaxLiveArticels)
			if db.MaxLiveArticels > 0 {
				for _, g := range db.Groups {
					g.Articles.MaxSize = db.MaxLiveArticels
				}
			}
		case 'D':
			msgid := line[1:]
			delete(db.Articles, common.MsgIDToRawMsgID("", msgid))
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

	log.Println()
	log.Printf("loader: %d data files, %d groups, %d articles, %d mods",
		len(db.Data), len(db.Groups), len(db.Articles), len(db.Mods))
	log.Println()
	return nil
}
