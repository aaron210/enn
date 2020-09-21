package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"sort"
	"strconv"

	"github.com/coyove/enn"
	"github.com/coyove/enn/server/common"
)

func LoadIndex(path string, db *Backend) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return err
	}

	db.Groups = map[string]*Group{}
	db.Articles = map[[16]byte]*common.ArticleRef{}
	db.Mods = map[string]*common.ModInfo{}
	db.Blacklist = map[string]*net.IPNet{}
	db.Index = f
	db.ServerName = *ServerName

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
		case ' ': // nop
		case 'G':
			if len(line) < 3 {
				common.E("#%d %q invalid G header", ln, line)
				continue
			}
			baseInfo := &common.BaseGroupInfo{}
			if err := json.Unmarshal(line[1:], baseInfo); err != nil {
				common.E("#%d %q invalid G header, json: %v", ln, line, err)
				continue
			}
			if baseInfo.Name == "" {
				common.E("#%d %q invalid G header, empty group name", ln, line)
				continue
			}
			gs := &Group{
				Group: &enn.Group{
					Name:        baseInfo.Name,
					Description: baseInfo.Desc,
				},
				Articles: &common.HighLowSlice{
					MaxSize: int(baseInfo.MaxLives),

					// We can't append 'D' commands while loading,
					// so NoPurgeNotify must be toggled, when all finished
					// it will be false again
					NoPurgeNotify: true,
				},
				BaseInfo: baseInfo,
			}
			switch baseInfo.Posting {
			case 0:
				gs.Group.Posting = enn.PostingPermitted
			case 1:
				gs.Group.Posting = enn.PostingNotPermitted
			case 2:
				gs.Group.Posting = enn.PostingModerated
			default:
				common.E("#%d %q invalid G header, invalid posting status", ln, line)
				continue
			}
			if old := db.Groups[baseInfo.Name]; old != nil {
				common.L("#%d update group: %s => %s", ln, baseInfo.Name, baseInfo.Diff(old.BaseInfo))
				old.Group = gs.Group
				old.BaseInfo = gs.BaseInfo
				old.Articles.MaxSize = int(baseInfo.MaxLives)
			} else {
				common.L("#%d create group: %s", ln, baseInfo.Name)
				db.Groups[baseInfo.Name] = gs
			}
		case 'A':
			if len(line) < 10 { // format: "Agroup msgid index offset length", 10 chars minimal
				common.E("#%d %q invalid A header", ln, line)
				continue
			}

			parts := bytes.Split(line[1:], []byte(" "))
			if len(parts) != 5 {
				common.E("#%d %q invalid A header, need 5 arguments", ln, line)
				continue
			}
			group, msgid, indexbuf, offsetbuf, lengthbuf := parts[0], parts[1], parts[2], parts[3], parts[4]

			g := db.Groups[string(group)]
			if g == nil {
				common.E("#%d %q invalid A header, invalid group: %q", ln, line, group)
				continue
			}

			ar := &common.ArticleRef{}

			ar.Index, err = strconv.Atoi(string(indexbuf))
			if err != nil {
				common.E("#%d %q invalid A header, invalid index: %v", ln, line, err)
				continue
			}

			ar.Offset, err = strconv.ParseInt(string(offsetbuf), 36, 64)
			if err != nil {
				common.E("#%d %q invalid A header, invalid offset: %v", ln, line, err)
				continue
			}

			ar.Length, err = strconv.ParseInt(string(lengthbuf), 36, 64)
			if err != nil {
				common.E("#%d %q invalid A header, invalid length: %v", ln, line, err)
				continue
			}

			ar.RawMsgID = common.MsgIDToRawMsgID("", msgid)
			g.Append(db, ar)
			db.Articles[ar.RawMsgID] = ar
		case 'D':
			msgid := line[1:]
			delete(db.Articles, common.MsgIDToRawMsgID("", msgid))
		case 'm':
			mi := &common.ModInfo{}
			if err := json.Unmarshal(line[1:], mi); err != nil {
				common.E("#%d %q invalid mod info: %v", ln, line, err)
				continue
			}
			common.L("#%d load mod info: %v", ln, mi)
			db.Mods[mi.Email] = mi
		case 'c':
			common.L("#%d remove mod info: %q", ln, line[1:])
			delete(db.Mods, string(line[1:]))
		case 'B':
			parts := bytes.Split(line[1:], []byte(" "))
			if len(parts) != 2 {
				common.E("#%d %q invalid B header, need 2 arguments", ln, line)
				continue
			}
			name, ipnetbuf := string(parts[0]), string(parts[1])
			_, ipnet, err := net.ParseCIDR(ipnetbuf)
			if err != nil {
				common.E("#%d %q invalid B header, invalid CIDR: %v", ln, line, err)
				continue
			}
			common.L("#%d add to blacklist: %q => %v", ln, name, ipnet)
			db.Blacklist[name] = ipnet
		case 'b':
			name := string(line[1:])
			common.L("#%d delete from blacklist: %q", ln, name)
			delete(db.Blacklist, name)
		}
	}

	// Finishing up
	for _, g := range db.Groups {
		g.Group.Count = int64(g.Articles.Len())
		g.Group.High = int64(g.Articles.High())
		g.Group.Low = int64(g.Articles.Low() + 1)
		g.Articles.NoPurgeNotify = false
	}

	common.L("loader: %d data files, %d groups, %d articles, %d mods, %d blocks",
		len(db.Data), len(db.Groups), len(db.Articles), len(db.Mods), len(db.Blacklist))
	return nil
}

// NopLines search the database and 'nop' the given lines so they will be omitted in any future loadings
func NopLines(path string, lines ...int) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0777)
	if err != nil {
		return err
	}
	defer f.Close()

	sort.Ints(lines)

	tmp := [1]byte{}
	rd := bufio.NewReader(f)
	read := 0
	ln := 1
	lineStartOffset := map[int]int{}

	for len(lines) > 0 {
		if ln == lines[0] {
			lineStartOffset[ln] = read
			lines = lines[1:]
		}

		n, _ := rd.Read(tmp[:])
		if n == 0 {
			break
		}

		read++
		if tmp[0] == '\n' {
			ln++
		}
	}

	for _, pos := range lineStartOffset {
		if _, err := f.Seek(int64(pos), 0); err != nil {
			return err
		}
		if _, err := f.WriteString(" "); err != nil {
			return err
		}
	}
	return nil
}
