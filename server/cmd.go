package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"

	"github.com/coyove/nnn"
	"github.com/coyove/nnn/server/common"
)

func splitCommand(a string, n int) []string {
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
		json.NewEncoder(p).Encode(common.BaseGroupInfo{
			Name: x[0],
			Desc: x[1],
		})
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	if *GroupSilence != "" {
		x := splitCommand(*GroupSilence, 1)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		p := bytes.NewBufferString("\nG")
		json.NewEncoder(p).Encode(common.BaseGroupInfo{
			Name:    gs.Info.Name,
			Desc:    gs.Info.Description,
			Silence: !(gs.Info.Posting == nnn.PostingNotPermitted),
		})
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	return false
}
