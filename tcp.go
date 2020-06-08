package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	tcpAddr = ":9160"
	delim   = 0x04
)

var tl = log.New(os.Stdout, "{TCP} ", 0)

var (
	loginNum          = 0
	defaultPlayerInfo = playerInfo{
		Name:  "Guest",
		Group: "1",
		Rank:  150,
	}
)

const noslot = -1

type player struct {
	playerInfo
	room   string
	course string
	slot   int
}

func handleConn(conn *net.TCPConn, tunnel chan interface{}) {
	tl.Println("connect:", conn.RemoteAddr())
	defer tl.Println("disconnect:", conn.RemoteAddr())
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	var sendNum int32
	send := func(s ...interface{}) {
		num := int(atomic.LoadInt32(&sendNum))

		var buf strings.Builder
		buf.WriteString(fmt.Sprint(s[0]))
		for i := 1; i < len(s); i++ {
			buf.WriteRune('`')
			buf.WriteString(fmt.Sprint(s[i]))
		}

		subHash := GetMD5Hash(PacketSubHashSalt + strconv.Itoa(num) + "`" + buf.String())[:3]
		rw.WriteString(subHash)
		rw.WriteRune('`')
		rw.WriteString(strconv.Itoa(num))
		rw.WriteRune('`')
		rw.WriteString(buf.String())
		rw.WriteByte(delim)
		rw.Flush()

		atomic.AddInt32(&sendNum, 1)
	}

	players, playersMu := make(map[*net.TCPConn]*player), sync.RWMutex{}
	rooms, roomsMu := make(map[string][4]*net.TCPConn), sync.RWMutex{}
	conf, confMu := make(map[string][4]bool), sync.RWMutex{}

	go func() {
		for {
			switch e := (<-tunnel).(type) {
			case login:
				tl.Println("received", e.UserName)
				p, err := getPlayerInfo(e.UserName)
				if err != nil {
					log.Println("but err occurred:", err)
					*p = defaultPlayerInfo
				}

				playersMu.Lock()
				players[conn] = &player{playerInfo: *p, slot: noslot}
				playersMu.Unlock()

				send("loginSuccessful", p.Group, p.Name)
				send("setRank", p.Rank)
			case fillslot:
			}
		}
	}()

	for {
		s, err := rw.ReadString(delim)
		if err != nil {
			if err == io.EOF {
				break
			}
			tl.Fatalln(err)
		}
		tl.Println("<-", s)
		// for i, c := range s {

		// }
		seg := strings.Split(s, "`")
		seg = seg[2:]

		// var command, subhash string
		// var sendn int
		// fmt.Sscanf(s, "%s`%d`%s", &subhash, &sendn, &command)

		switch seg[0] {
		// menu
		case "request_login_id":
			loginNum++
			send("setLoginID", loginNum)

		// lobby
		case "ping":
			send("ping", time.Now().Unix())
		case "get_customize_info":
			playersMu.RLock()
			if p, ok := players[conn]; ok {
				var buf strings.Builder
				buf.WriteString("1")
				for i := 2; i < 46; i++ {
					buf.WriteRune(',')
					buf.WriteString(strconv.Itoa(i))
				}
				all := buf.String()
				allhats := "1,2,3,4,5,6,7,8,9,10,11,12,13,14"
				send("setCustomizeInfo",
					p.HatColor, p.HeadColor, p.BodyColor, p.FeetColor,
					p.Hat, p.Head, p.Body, p.Feet,
					allhats, all, all, all,
					50, 50, 50,
					p.Rank, 0, 0,
					p.HatColor2, p.HeadColor2, p.BodyColor2, p.FeetColor2,
					all, all, all, all,
				)
			}
			playersMu.RUnlock()
		case "set_customize_info":

		case "get_online_list":
			playersMu.RLock()
			for _, p := range players {
				send("addUser", p.Name, p.Group, p.Rank, p.Hats)
			}
			playersMu.RUnlock()
		case "set_right_room":
			playersMu.Lock()
			players[conn].room = seg[1]
			playersMu.Unlock()
		case "fill_slot":
			if slot, err := strconv.Atoi(seg[2]); err == nil {
				playersMu.Lock()
				p := players[conn]
				roomid := p.room + seg[1]

				roomsMu.Lock()
				p.slot = slot
				p.course = seg[1]
				box := rooms[roomid]
				box[slot] = conn
				rooms[roomid] = box
				send("fillSlot"+seg[1], slot, p.Name, p.Rank, "me")
				// for conn, _ := range players {

				// }
				roomsMu.Unlock()
				playersMu.Unlock()
			}
		case "clear_slot":
			playersMu.RLock()
			p := players[conn]
			roomid := p.room + p.course
			course := p.course
			slot := p.slot

			roomsMu.Lock()
			if box, ok := rooms[roomid]; ok {
				box[slot] = nil
				rooms[roomid] = box
				send("clearSlot"+course, slot)

				any := false
				for i := 0; i < len(box); i++ {
					if box[i] != nil {
						any = true
						break
					}
				}
				if !any {
					delete(rooms, roomid)
					confMu.Lock()
					delete(conf, roomid)
					confMu.Unlock()
				}
			}
			roomsMu.Unlock()
			playersMu.RUnlock()
		case "confirm_slot":
			playersMu.RLock()
			p := players[conn]
			roomid := p.room + p.course
			course := p.course
			slot := p.slot

			roomsMu.Lock()
			validroom := rooms[roomid][slot] == conn

			if validroom {
				confMu.Lock()
				box := conf[roomid]
				box[slot] = true
				conf[roomid] = box
				send("confirmSlot"+course, slot)

				allconfirmed := true
				for i := 0; i < len(box); i++ {
					if !box[i] {
						allconfirmed = false
						break
					}
				}
				if allconfirmed {
					delete(rooms, roomid)
					delete(conf, roomid)

					// start race
				}
				confMu.Unlock()
			}
			roomsMu.Unlock()
			playersMu.RUnlock()
		}
	}
}

func runTCP() {
	addr, err := net.ResolveTCPAddr("tcp4", tcpAddr)
	if err != nil {
		tl.Fatalln(err)
	}
	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		tl.Fatalln(err)
	}
	defer l.Close()

	var routerMu sync.RWMutex
	router := make(map[string]chan interface{})

	// tunnels router
	go func() {
		for {
			m := <-rootTunnel
			routerMu.RLock()
		tryagain:
			t, ok := router[m.addr]
			if !ok {
				// ipv4/6 localhost hack
				if m.addr == "[::1]" {
					m.addr = "127.0.0.1"
					goto tryagain
				}
				tl.Println("can't find route for", m.addr)
				continue
			}
			routerMu.RUnlock()
			t <- m.data
		}
	}()

	tl.Println("started")
	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			tl.Fatalln(err)
		}

		addr := trimport(conn.RemoteAddr().String())
		routerMu.Lock()
		t := make(chan interface{})
		router[addr] = t
		routerMu.Unlock()

		go handleConn(conn, t)
	}
}
