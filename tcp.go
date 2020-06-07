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

	players, playersMu := make(map[*net.TCPConn]*playerInfo), sync.RWMutex{}
	// rooms, roomsMu := make(map[string][4]*playerInfo), sync.Mutex{}

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
				players[conn] = p
				playersMu.Unlock()

				send("loginSuccessful", p.Group, p.Name)
				send("setRank", p.Rank)
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
		p := strings.Split(s, "`")
		p = p[2:]
		switch p[0] {
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
		case "get_online_list":
			playersMu.RLock()
			for _, p := range players {
				send("addUser", p.Name, p.Group, p.Rank, p.Hats)
			}
			playersMu.RUnlock()
		case "fill_slot": // f96`5`fill_slot`6500844_3`0`1
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
			select {
			case m := <-rootTunnel:
				routerMu.RLock()
				t, ok := router[m.addr]
				if !ok {
					tl.Fatalln("can't find route for", m.addr)
				}
				routerMu.RUnlock()
				t <- m.data
			}
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
