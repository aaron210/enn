package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"
	"unicode"

	"github.com/coyove/enn/server/common"
)

func askInput(prompt, value interface{}) (string, int64, int) {
	var in string
	fmt.Printf("%s (default: %#v): ", prompt, value)
	fmt.Scanln(&in)
	if in == "" {
		in = fmt.Sprint(value)
	}
	v, _ := strconv.ParseInt(in, 10, 64)
	v2, _ := strconv.Atoi(in)
	return in, v, v2
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

	if *ModCmd != "" {
		m := db.Mods[*ModCmd]
		if m == nil {
			fmt.Println("Create new mod")
			m = &common.ModInfo{Email: *ModCmd}
		} else {
			fmt.Println("Update mod", *ModCmd)
		}
		m.Password, _, _ = askInput("Password", m.Password)

		p := bytes.NewBufferString("\nm")
		json.NewEncoder(p).Encode(m)
		common.PanicIf(db.WriteCommand(p.Bytes()), "%%err")
		return true
	}

	if *BlacklistCmd != "" {
		fmt.Println("Existing blacklist")
		for k, v := range db.Blacklist {
			fmt.Printf("%q => %v\n", k, v)
		}
		name := *BlacklistCmd
		b := db.Blacklist[name]
		if b == nil {
			fmt.Println("Add blacklist", name)
			cc, _, _ := askInput("Enter range (CIDR format)", "")
			_, ipnet, err := net.ParseCIDR(cc)
			common.PanicIf(err, "%%err")
			common.PanicIf(db.WriteCommand([]byte("\nB"+name+" "+ipnet.String())), "%%err")
		} else {
			fmt.Println("Remove blacklist", name)
			common.PanicIf(db.WriteCommand([]byte("\nb"+name)), "%%err")
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
					Posting:     'y',
					MaxLives:    1000,
					MaxPostSize: 0,
					CreateTime:  time.Now().Unix(),
				},
			}
		} else {
			fmt.Println("Update group", *GroupCmd)
		}
		gs.BaseInfo.Desc, _, _ = askInput("Description", gs.BaseInfo.Desc)
		_, gs.BaseInfo.MaxLives, _ = askInput("Max live articles", gs.BaseInfo.MaxLives)
		_, gs.BaseInfo.MaxPostSize, _ = askInput("Max post size (0 means using global setting)", gs.BaseInfo.MaxPostSize)
		_, _, gs.BaseInfo.Posting = askInput("Posting (0: unlimited, 1: disbaled)", gs.BaseInfo.Posting)
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
