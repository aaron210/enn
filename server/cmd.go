package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/common"
)

func splitCommand(a string, n int) []string {
	if a == "-" {
		total := bytes.Buffer{}
		for {
			var line string
			_, err := fmt.Scanf("%s", &line)
			if err != nil {
				break
			}
			total.WriteString(line)
			total.WriteString("\n")
		}
		a = total.String()
	}

	res, _ := csv.NewReader(strings.NewReader(a)).ReadAll()
	common.PanicIf(len(res) == 0 || len(res[0]) != n, "invalid command: %q (%d)", a, n)
	return res[0]
}

func HandleCommand() bool {
	if *ModAdd != "" {
		x := splitCommand(*ModAdd, 2)
		common.PanicIf(db.Mods[x[0]] != nil, "mod %q already existed", x[0])

		p := bytes.NewBufferString("\nm")
		json.NewEncoder(p).Encode(common.ModInfo{
			Email:    x[0],
			Password: x[1],
		})
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	if *ModDel != "" {
		x := splitCommand(*ModDel, 1)
		common.PanicIf(db.Mods[x[0]] == nil, "mod %q not existed", x[0])
		common.PanicIf(db.WriteCommand([]byte("\nc"+x[0])), "%%err")
		return true
	}

	if *GroupAdd != "" {
		x := splitCommand(*GroupAdd, 2)
		p := bytes.NewBufferString("\nG")
		gs := db.Groups[x[0]]
		if gs == nil {
			json.NewEncoder(p).Encode(common.BaseGroupInfo{
				Name: x[0],
				Desc: x[1],
			})
			common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		} else {
			gs.Info.Name = x[0]
			gs.Info.Description = x[1]
			common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.Info)), "%%err")
		}
		return true
	}

	if *GroupMaxPostSz != "" {
		x := splitCommand(*GroupMaxPostSz, 2)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		x[1] = strings.Replace(strings.ToLower(x[1]), "k", "000", -1)
		x[1] = strings.Replace(strings.ToLower(x[1]), "m", "000000", -1)
		gs.Info.MaxPostSize, _ = strconv.ParseInt(x[1], 10, 64)
		common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.Info)), "%%err")
		return true
	}

	if *GroupSilence != "" {
		x := splitCommand(*GroupSilence, 1)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		if gs.Info.Posting == nnn.PostingNotPermitted {
			gs.Info.Posting = nnn.PostingPermitted
		} else {
			gs.Info.Posting = nnn.PostingNotPermitted
		}
		common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.Info)), "%%err")
		return true
	}

	return false
}

func groupInfoAdapter(info *nnn.Group) []byte {
	p := bytes.NewBufferString("\nG")
	json.NewEncoder(p).Encode(common.BaseGroupInfo{
		Name:        info.Name,
		Desc:        info.Description,
		Silence:     info.Posting == nnn.PostingNotPermitted,
		MaxPostSize: info.MaxPostSize,
	})
	return p.Bytes()
}
