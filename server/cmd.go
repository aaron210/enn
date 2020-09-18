package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coyove/enn/server/common"
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
		gs := db.Groups[x[0]]
		if gs == nil {
			common.PanicIf(db.WriteCommand(groupInfoAdapter(&common.BaseGroupInfo{
				Name:       x[0],
				Desc:       x[1],
				MaxLives:   1000,
				CreateTime: time.Now().Unix(),
			})), "%%err")
		} else {
			gs.BaseInfo.Name = x[0]
			gs.BaseInfo.Desc = x[1]
			common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.BaseInfo)), "%%err")
		}
		return true
	}

	if *GroupMaxLives != "" {
		x := splitCommand(*GroupMaxLives, 2)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		gs.BaseInfo.MaxLives, _ = strconv.ParseInt(x[1], 10, 64)
		common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.BaseInfo)), "%%err")
		return true
	}

	if *GroupMaxPostSz != "" {
		x := splitCommand(*GroupMaxPostSz, 2)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		x[1] = strings.Replace(strings.ToLower(x[1]), "k", "000", -1)
		x[1] = strings.Replace(strings.ToLower(x[1]), "m", "000000", -1)
		gs.BaseInfo.MaxPostSize, _ = strconv.ParseInt(x[1], 10, 64)
		common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.BaseInfo)), "%%err")
		return true
	}

	if *GroupSilence != "" {
		x := splitCommand(*GroupSilence, 2)
		gs := db.Groups[x[0]]
		common.PanicIf(gs == nil, "group %q not existed", x[0])
		gs.BaseInfo.Posting = x[1][0]
		common.PanicIf(db.WriteCommand(groupInfoAdapter(gs.BaseInfo)), "%%err")
		return true
	}

	return false
}

func groupInfoAdapter(info *common.BaseGroupInfo) []byte {
	p := bytes.NewBufferString("\nG")
	json.NewEncoder(p).Encode(info)
	return p.Bytes()
}
