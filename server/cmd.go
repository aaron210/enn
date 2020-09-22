package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/coyove/enn/server/common"
)

func askInput(prompt, value interface{}) (string, int64) {
	var in string
	fmt.Printf("%s (default: %#v) >> ", prompt, value)
	fmt.Scanln(&in)
	if in == "" {
		in = fmt.Sprint(value)
	}
	tmp := strings.Replace(strings.ToLower(in), "k", "000", -1)
	tmp = strings.Replace(tmp, "m", "000000", -1)
	v, _ := strconv.ParseInt(tmp, 10, 64)
	return in, v
}

func HandleCommand() bool {
	if *NopDB != "" {
		var lines []int
		var tmp bytes.Buffer
		var raw = *NopDB + " "

		for raw != "" {
			if unicode.IsDigit(rune(raw[0])) {
				tmp.WriteByte(raw[0])
			} else {
				line, _ := strconv.Atoi(tmp.String())
				if line > 0 {
					lines = append(lines, line)
				}
				tmp.Reset()
			}
			raw = raw[1:]
		}
		common.PanicIf(NopLines(*DBPath, lines...), "%%err")
		return true
	}

	if *ConfigCmd {
		_, db.Config.MaxPostSize = askInput("Global post size", db.Config.MaxPostSize)
		_, db.Config.ThrotCmdWin = askInput("Throt # of NNTP commands", db.Config.ThrotCmdWin)
		_, db.Config.PostIntervalSec = askInput("Cooldown seconds between two posts", db.Config.PostIntervalSec)
		p := bytes.NewBufferString("\nC")
		json.NewEncoder(p).Encode(db.Config)
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	if *ModCmd != "" {
		m := db.Mods[*ModCmd]
		if m == nil {
			fmt.Println("Create new mod")
			m = &common.ModInfo{Email: *ModCmd}
			m.Password, _ = askInput("Password", m.Password)
		} else {
			fmt.Println("Delete mod", *ModCmd)
			m.Deleted = true
		}
		p := bytes.NewBufferString("\nm")
		json.NewEncoder(p).Encode(m)
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	if *BlacklistCmd {
		fmt.Printf("Existing blocks (%d)\n", len(db.Blacklist))
		for k, v := range db.Blacklist {
			fmt.Printf("%q => %v\n", k, v)
		}
		name, _ := askInput("Block name", "")
		if name == "" {
			return true
		}

		b := db.Blacklist[name]
		if b == nil {
			fmt.Println("Add to blacklist", name)
			cc, _ := askInput("Enter range (CIDR format)", "")
			_, ipnet, err := net.ParseCIDR(cc)
			common.PanicIf(err, "%%err")
			common.PanicIf(db.WriteCommand([]byte("\nB"+name+" "+ipnet.String())), "%%err")
		} else {
			fmt.Println("Remove from blacklist", name)
			common.PanicIf(db.WriteCommand([]byte("\nB"+name+" 0.0.0.0/32")), "%%err")
		}
		return true
	}

	if *GroupCmd != "" {
		gs := db.Groups[*GroupCmd]
		if gs == nil {
			fmt.Println("Create new group")
			gs = &Group{
				BaseInfo: &common.BaseGroupInfo{
					Name:        *GroupCmd,
					Desc:        "",
					Posting:     0,
					MaxLives:    1000,
					MaxPostSize: 0,
					CreateTime:  time.Now().Unix(),
				},
			}
		} else {
			fmt.Println("Update group", *GroupCmd)
		}
		gs.BaseInfo.Desc, _ = askInput("Description", gs.BaseInfo.Desc)
		_, gs.BaseInfo.MaxLives = askInput("Max live articles", gs.BaseInfo.MaxLives)
		_, gs.BaseInfo.MaxPostSize = askInput("Max post size (0: using global setting)", gs.BaseInfo.MaxPostSize)
		_, gs.BaseInfo.Posting = askInput("Posting (0: unlimited, 1: disbaled)", gs.BaseInfo.Posting)
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
