package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	tcpAddr = ":9160"
	raceCap = 4
)

var tl = log.New(os.Stdout, "{TCP} ", 0)

var (
	loginNum      = 0
	defaultPlayer = player{
		playerInfo: playerInfo{
			Name:  "Guest",
			Group: "1",
			Rank:  150,
		},
		speed:    "100",
		accel:    "100",
		jump:     "100",
		wornHats: make([]hat, 0),
		replays:  make(map[string]string),
	}
)

type player struct {
	playerInfo
	tempID             int
	speed, accel, jump string
	room, chat         string
	course             string
	slot               int

	wornHats []hat

	// replays []*strings.Builder
	replaying bool
	replayer  <-chan []string
	replays   map[string]string
}

func (p *player) CustomizeInfo() string {
	return fmt.Sprintf("%s`%s`%s`%s`%s`%s`%s`%s`%s`%s`%s`%s`%s`%s`%s",
		p.HatColor, p.HeadColor, p.BodyColor, p.FeetColor,
		p.HatColor2, p.HeadColor2, p.BodyColor2, p.FeetColor2,
		p.Hat, p.Head, p.Body, p.Feet,
		p.speed, p.accel, p.jump,
	)
}

func (p *player) SetCustomizeInfo(s []string) {
	p.HatColor = s[0]
	p.HeadColor = s[1]
	p.BodyColor = s[2]
	p.FeetColor = s[3]
	p.HatColor2 = s[4]
	p.HeadColor2 = s[5]
	p.BodyColor2 = s[6]
	p.FeetColor2 = s[7]
	p.Hat = s[8]
	p.Head = s[9]
	p.Body = s[10]
	p.Feet = s[11]
	p.speed = s[12]
	p.accel = s[13]
	p.jump = s[14]
}

func (p *player) RemoteInfo() string {
	return strings.Join([]string{
		strconv.Itoa(p.tempID), p.Name,
		p.HatColor, p.HeadColor, p.BodyColor, p.FeetColor,
		p.Hat, p.Head, p.Body, p.Feet,
		fmt.Sprint(p.HatColor2), fmt.Sprint(p.HeadColor2), fmt.Sprint(p.BodyColor2), fmt.Sprint(p.FeetColor2),
	}, sep)
}

func (p *player) LocalInfo() string {
	return strings.Join([]string{
		strconv.Itoa(p.tempID),
		p.speed, p.accel, p.jump,
		p.HatColor, p.HeadColor, p.BodyColor, p.FeetColor,
		p.Hat, p.Head, p.Body, p.Feet,
		fmt.Sprint(p.HatColor2), fmt.Sprint(p.HeadColor2), fmt.Sprint(p.BodyColor2), fmt.Sprint(p.FeetColor2),
	}, sep)
}

type server struct {
	players map[conner]*player
	boxes   map[string][raceCap]slotinfo
	races   map[conner]*race
	chats   map[string]chat
}

type slotinfo struct {
	conn      *pr2conn
	confirmed bool
	joined    int64
}

type race struct {
	racers          []conner
	looseHats       map[int]hat
	looseHatCounter int
	// race info that must persist after a player has left the race
	// for the finish times.
	// zero value is ready to use
	stats [raceCap]struct {
		name              string
		drawn             bool
		startTime         time.Time
		finishTime        time.Duration
		finished, quit    bool
		gone              bool
		objectivesReached int
	}
}

type hat struct {
	num           string
	color, color2 string
}

// zero value is ready to use
type chat struct {
	chatters []*pr2conn
	history  []string
}

func (r *race) id(conn *pr2conn) int {
	for i, rconn := range r.racers {
		if conn == rconn {
			return i
		}
	}
	return -1
}

func subhash(p []byte) string {
	hasher := md5.New()
	hasher.Write([]byte(PacketSubHashSalt))
	hasher.Write(p)
	return hex.EncodeToString(hasher.Sum(nil))[:3]
}

func setHatsStr(s *server, conn *pr2conn) string {
	var b strings.Builder
	meID := s.players[conn].tempID //s.races[conn].id(conn)
	b.WriteString("setHats" + strconv.Itoa(meID))
	b.WriteString(sep)
	for i, h := range s.players[conn].wornHats {
		if i != 0 {
			b.WriteString(sep) // TODO make betr xd
		}
		b.WriteString(h.num)
		b.WriteString(sep)
		b.WriteString(h.color)
		b.WriteString(sep)
		b.WriteString(h.color2)
	}
	return b.String()
}

func finishTimesStr(s *server, conn conner) string {
	var b strings.Builder
	b.WriteString("finishTimes")

	r, ok := s.races[conn]
	if !ok {
		b.WriteString(sep)
		b.WriteString("[error: can't find race]")
		b.WriteString(sep)
		b.WriteString("0")
		b.WriteString(sep)
		// drawing
		b.WriteString(sep)
		// gone
		return b.String()
	}
	statssorted := make([]struct {
		name              string
		drawn             bool
		startTime         time.Time
		finishTime        time.Duration
		finished          bool
		quit              bool
		gone              bool
		objectivesReached int
	}, len(r.stats))
	copy(statssorted, r.stats[:])
	sort.Slice(statssorted, func(i, j int) bool {
		if statssorted[i].finished && !statssorted[j].finished {
			return true
		}
		if !statssorted[i].finished && statssorted[j].finished {
			return false
		}
		return statssorted[i].finishTime < statssorted[j].finishTime
	})

	for _, st := range statssorted {
		if len(st.name) == 0 {
			continue
		}
		if st.finished || st.quit {
			b.WriteString(sep)
			b.WriteString(st.name)
			b.WriteString(sep)
			if st.finished {
				b.WriteString(fmt.Sprint(st.finishTime.Seconds()))
			} else if st.quit {
				b.WriteString("forfeit")
			}
			b.WriteString(sep)
			if !st.drawn {
				b.WriteString("1")
			}
			b.WriteString(sep)
			if !st.gone {
				b.WriteString("1")
			}
		}
	}

	return b.String()
}

func startRace(s *server, course string, boxid string) {
	box := s.boxes[boxid]
	// sort em by join time
	boxsorted := box[:]
	sort.Slice(boxsorted, func(i, j int) bool {
		return boxsorted[i].joined > boxsorted[j].joined
	})
	// assign their tempIDs
	for i, slot := range boxsorted {
		if slot.conn == nil {
			continue
		}
		s.players[slot.conn].tempID = i
	}

	r := &race{
		racers:    make([]conner, 0),
		looseHats: make(map[int]hat),
	}
	// vcr watches packets to record a replay
	vcr := newvcr(&strings.Builder{})

	for i, slot := range boxsorted {
		if slot.conn == nil {
			continue
		}

		// init player
		p := s.players[slot.conn]
		p.wornHats = []hat{}
		if p.Hat != "0" {
			p.wornHats = append(p.wornHats, hat{
				num:    p.Hat,
				color:  p.HatColor,
				color2: fmt.Sprint(p.HatColor2),
			})
		}

		// init race stuff
		r.stats[i].name = p.Name
		r.racers = append(r.racers, slot.conn)
		s.races[slot.conn] = r

		slot.conn.send("forceTime", 0)
		slot.conn.send("tournamentMode", 0)
		slot.conn.send("startGame", strings.Split(course, "_")[0])
		slot.conn.send("createLocalCharacter", p.LocalInfo())
		for _, slot2 := range boxsorted {
			if slot2.conn == nil {
				continue
			}
			if slot.conn != slot2.conn {
				slot.conn.send("createRemoteCharacter", s.players[slot2.conn].RemoteInfo())
			}
		}
	}
	// vcr is the last "racer"
	for _, racer := range r.racers {
		vcr.send("createRemoteCharacter", s.players[racer].RemoteInfo())
	}
	r.racers = append(r.racers, vcr)
}

func handle(conn *pr2conn, svr chan func(*server)) {
	tl.Println("connect:", conn.conn.RemoteAddr())
	defer tl.Println("disconnect:", conn.conn.RemoteAddr())

	defer func() {
		if err := recover(); err != nil {
			ioutil.WriteFile(fmt.Sprint(time.Now().Unix())+".txt",
				[]byte(fmt.Sprintf("%v %s", err, string(debug.Stack()))), 0777)
			panic(err)
		}
	}()

	defer func() {
		svr <- func(s *server) {
			// clean tf up
			p := s.players[conn]
			// clear the slot we occupy
			boxid := p.room + p.course
			if box, ok := s.boxes[boxid]; ok {
				box[p.slot] = slotinfo{}
			}
			// kick us from the race
			if r, ok := s.races[conn]; ok {
				for i, racer := range r.racers {
					if racer == conn {
						r.racers = append(r.racers[:i], r.racers[i+1:]...)
						id := s.players[conn].tempID
						r.stats[id].quit = true
						r.stats[id].gone = true
						// send finishTimes if race is running?
						break
					}
				}
				delete(s.races, conn)
			}
			// clear the chat we occupy
			if p.chat != "none" {
				if chat, ok := s.chats[p.chat]; ok {
					for i, chatter := range chat.chatters {
						if chatter == conn {
							chat.chatters = append(chat.chatters[:i], chat.chatters[i+1:]...)
							s.chats[p.chat] = chat
							break
						}
					}
				}
			}
			// delete yourself
			delete(s.players, conn)
		}
	}()

	svr <- func(s *server) {
		p := s.players[conn]
		conn.send("loginSuccessful", p.Group, p.Name)
		conn.send("setRank", p.Rank)
	}

	for {
		seg, err := conn.read()
		if err != nil {
			if err == io.EOF {
				break
			}
			tl.Println("read err:", err)
			break
		}
		tl.Println("<-", seg)

		switch seg[0] {
		// lobby
		case "ping":
			conn.send("ping", time.Now().Unix())
		case "get_customize_info":
			svr <- func(s *server) {
				if p, ok := s.players[conn]; ok {
					var buf strings.Builder
					buf.WriteString("1")
					for i := 2; i < 46; i++ {
						buf.WriteRune(',')
						buf.WriteString(strconv.Itoa(i))
					}
					all := buf.String()
					allhats := "1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16"
					conn.send("setCustomizeInfo",
						p.HatColor, p.HeadColor, p.BodyColor, p.FeetColor,
						p.Hat, p.Head, p.Body, p.Feet,
						allhats, all, all, all,
						p.speed, p.accel, p.jump,
						p.Rank, 0, 0,
						p.HatColor2, p.HeadColor2, p.BodyColor2, p.FeetColor2,
						all, all, all, all,
					)
				}
			}
		case "set_customize_info":
			svr <- func(s *server) {
				s.players[conn].SetCustomizeInfo(seg[1:])
			}
		case "get_online_list":
			svr <- func(s *server) {
				for _, p := range s.players {
					conn.send("addUser", p.Name, p.Group, p.Rank, p.Hats)
				}
			}
		case "set_game_room":
			svr <- func(s *server) {
				if seg[1] != "none" {
					tl.Println("unexpected game room:", seg)
				}
				s.players[conn].replaying = false
				if r, ok := s.races[conn]; ok {
					meID := r.id(conn)
					tl.Println(meID, "IS GONEEEEEEEEEE")
					if meID != -1 {
						r.stats[s.players[conn].tempID].gone = true
						r.racers = append(r.racers[:meID], r.racers[meID+1:]...)
						// setFinishTime
						finishtime := finishTimesStr(s, conn)
						for _, racer := range r.racers {
							racer.send(finishtime)
						}
					} else {
						tl.Println("WTFFFFF?? WHY -1?")
					}

					if len(r.racers) == 1 {
						// save our replay!
						p := s.players[conn]
						replayID := fmt.Sprintf("%s_%d", p.course, time.Now().Unix())
						replay := r.racers[0].(*vcrconn).out.(*strings.Builder)
						p.replays[replayID] = replay.String()
						// err := ioutil.WriteFile(replayID, []byte(replay.String()), 0666)
						// if err != nil {
						// 	log.Println(err)
						// }
						conn.send("message", fmt.Sprintf("replayID: %s", replayID))
					}

					delete(s.races, conn)
				}
			}
		case "set_right_room":
			svr <- func(s *server) {
				s.players[conn].room = seg[1]
				room := seg[1]
				for boxid, box := range s.boxes {
					if strings.HasPrefix(boxid, room) {
						course := boxid[len(room):]
						for i, slot := range box {
							if slot.conn == nil {
								continue
							}
							p := s.players[slot.conn]

							if p == nil {
								// FUCK
								box[i] = slotinfo{}
								s.boxes[boxid] = box
								continue
							}

							conn.send("fillSlot"+course, i, p.Name, p.Rank)
							if slot.confirmed {
								conn.send("confirmSlot"+course, i)
							}
						}
					}
				}
			}
		case "fill_slot":
			svr <- func(s *server) {
				if slot, err := strconv.Atoi(seg[2]); err == nil {

					me := s.players[conn]

					// clear the slot we occupy, if any
					boxid := me.room + me.course
					if box, ok := s.boxes[boxid]; ok {
						box[me.slot] = slotinfo{}
					}

					me.slot = slot
					me.course = seg[1]
					boxid = me.room + me.course

					box := s.boxes[boxid]
					// dont steal some1's spot
					if box[slot].conn == nil {
						box[slot] = slotinfo{conn: conn, joined: time.Now().Unix()}
						s.boxes[boxid] = box

						conn.send("fillSlot"+seg[1], slot, me.Name, me.Rank, "me")
						for pconn, p := range s.players {
							if me.room == p.room {
								pconn.send("fillSlot"+seg[1], slot, me.Name, me.Rank)
							}
						}
					}
				}
			}
		case "clear_slot":
			svr <- func(s *server) {
				me := s.players[conn]
				boxid := me.room + me.course

				if box, ok := s.boxes[boxid]; ok {
					box[me.slot] = slotinfo{}
					s.boxes[boxid] = box

					for pconn, p := range s.players {
						if me.room == p.room {
							pconn.send("clearSlot"+me.course, me.slot)
						}
					}

					any := false
					allconfirmed := true
					for i := 0; i < len(box); i++ {
						if box[i].conn != nil {
							any = true
							if !box[i].confirmed {
								allconfirmed = false
							}
						}
					}
					if any {
						if allconfirmed && s.races[conn] == nil {
							startRace(s, s.players[conn].course, boxid)
						}
					} else {
						delete(s.boxes, boxid)
					}
				}
			}
		case "confirm_slot":
			svr <- func(s *server) {
				me := s.players[conn]
				roomid := me.room + me.course
				validroom := s.boxes[roomid][me.slot].conn == conn

				if validroom {
					box := s.boxes[roomid]
					box[me.slot].confirmed = true
					s.boxes[roomid] = box

					for pconn, p := range s.players {
						if me.room == p.room {
							pconn.send("confirmSlot"+me.course, me.slot)
						}
					}

					allconfirmed := true
					for i := 0; i < len(box); i++ {
						slot := s.boxes[roomid][i]
						if slot.conn != nil && !slot.confirmed {
							allconfirmed = false
							break
						}
					}
					if allconfirmed {
						startRace(s, s.players[conn].course, roomid)
					}
				}
			}
		case "finish_drawing":
			svr <- func(s *server) {
				p := s.players[conn]
				if p.replaying {
					conn.send("createSpectator", "")
					go playReplay(svr, conn, p.replayer)
					return
				}
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					r.stats[meID].drawn = true

					tl.Println("finished drawing", conn)
					for _, racer := range r.racers {
						racer.send("finishDrawing", p.tempID)
					}

					alldrawn := true
					for _, st := range r.stats {
						if len(st.name) != 0 && !st.drawn {
							alldrawn = false
						}
					}
					if alldrawn {
						// begin race
						start := time.Now()
						for i, racer := range r.racers {
							r.stats[i].startTime = start

							switch racer.(type) {
							case *pr2conn:
								sethats := setHatsStr(s, racer.(*pr2conn))
								for _, racer2 := range r.racers {
									racer2.send(sethats)
								}
							}

							racer.send("ping", time.Now().Unix())
							racer.send("beginRace", "")
						}

						conn.send("finishTimes")
						// conn.send("p", 0, 0)
					}
				}
			}
		case "finish_race":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					r.stats[meID].finished = true
					r.stats[meID].finishTime = time.Since(r.stats[meID].startTime)

					// setFinishTime
					finishtime := finishTimesStr(s, conn)
					for _, racer := range r.racers {
						racer.send(finishtime)
					}
				}
			}
		case "quit_race":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					r.stats[meID].quit = true
					r.stats[meID].finishTime = time.Since(r.stats[meID].startTime)

					// setFinishTime
					finishtime := finishTimesStr(s, conn)
					for _, racer := range r.racers {
						racer.send(finishtime)
					}
				}
			}
		case "grab_egg":
		case "objective_reached":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					r.stats[s.players[conn].tempID].objectivesReached++
				}
			}
		case "loose_hat":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					worn := s.players[conn].wornHats
					var popped hat
					popped, s.players[conn].wornHats = worn[len(worn)-1], worn[:len(worn)-1]

					hatID := r.looseHatCounter
					r.looseHats[hatID] = popped
					r.looseHatCounter++

					sethats := setHatsStr(s, conn)
					for _, racer := range r.racers {
						racer.send("addEffect", "Hat", strings.Join(seg[1:], sep),
							popped.num, popped.color, popped.color2, hatID)

						racer.send(sethats)
					}
				}
			}
		case "get_hat":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					hatID, err := strconv.Atoi(seg[1])
					if err != nil {
						tl.Panicln(hatID, "is not an int")
					}
					hat := r.looseHats[hatID]
					delete(r.looseHats, hatID)
					for _, racer := range r.racers {
						racer.send("removeHat"+seg[1], "")
					}
					if hat.num == "12" {
						// TODO: thief stuff
					} else if hat.num == "13" {
						// TODO: arti stuff
					} else {
						s.players[conn].wornHats = append(s.players[conn].wornHats, hat)
						for _, racer := range r.racers {
							racer.send(setHatsStr(s, conn))
						}
					}
				}
			}
		case "p":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("p"+strconv.Itoa(meID), strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "exact_pos":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("exactPos"+strconv.Itoa(meID), strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "squash":
			// TODO
		case "sting":
			// TODO
		case "set_var":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("var"+strconv.Itoa(meID), strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "add_effect":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("addEffect", strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "zap":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					meID := s.players[conn].tempID //r.id(conn)
					for _, racer := range r.racers {
						racer.send("zap", meID)
					}
				}
			}
		case "hit":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("hit" + strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "activate":
			svr <- func(s *server) {
				if r, ok := s.races[conn]; ok {
					for _, racer := range r.racers {
						if racer != conn {
							racer.send("activate", strings.Join(seg[1:], sep))
						}
					}
				}
			}
		case "chat":
			svr <- func(s *server) {
				me := s.players[conn]

				if strings.HasPrefix(seg[1], ",say ") {
					msg := strings.TrimPrefix(seg[1], ",say ")
					for pconn := range s.players {
						if pconn == conn {
							continue
						}
						if _, ok := s.races[pconn]; ok {
							pconn.send("systemChat", "notice: "+msg)
						} else {
							pconn.send("message", msg)
						}
					}
				} else if strings.HasPrefix(seg[1], "/replay ") {
					replayID := strings.TrimPrefix(seg[1], "/replay ")
					spl := strings.Split(replayID, "_")
					p := s.players[conn]
					if replay, ok := p.replays[replayID]; ok {
						conn.send("startReplay", spl[0], spl[1])
						p.replaying = true
						replayClone := make([]byte, len(replay))
						copy(replayClone, replay)
						p.replayer = Replay(bytes.NewBuffer(replayClone))
					} else {
						conn.send("systemChat", "invalid replay id")
					}
				} else {

					if r, ok := s.races[conn]; ok {
						// race chat
						for _, racer := range r.racers {
							racer.send("chat", me.Name, me.Group, seg[1])
						}
					} else if me.chat != "none" {
						// TODO: lobby chat
						chat := s.chats[me.chat]
						if len(chat.history) == 20 {
							chat.history = chat.history[1:]
						}
						chat.history = append(chat.history, strings.Join([]string{
							"chat", me.Name, me.Group, seg[1],
						}, sep))
						s.chats[me.chat] = chat
						for _, chatter := range chat.chatters {
							chatter.send("chat", me.Name, me.Group, seg[1])
						}
					}
				}
			}
		case "set_chat_room":
			svr <- func(s *server) {
				p := s.players[conn]

				chat := s.chats[p.chat]
				id := -1
				for i, chatter := range chat.chatters {
					if chatter == conn {
						id = i
					}
				}
				inachat := id != -1

				if inachat && p.chat != "none" {
					// leave it
					chat.chatters = append(chat.chatters[:id], chat.chatters[id+1:]...)
					s.chats[p.chat] = chat

					if len(chat.chatters) == 0 && p.chat != "main" {
						delete(s.chats, p.chat)
					}
				}

				if seg[1] != "none" {
					chat := s.chats[seg[1]]
					chat.chatters = append(chat.chatters, conn)
					s.chats[seg[1]] = chat
					// send the history
					for _, msg := range chat.history {
						conn.send(msg)
					}
				}

				p.chat = seg[1]
			}
		// if seg[1] == "main" {
		// 	conn.send("systemChat", TODO)
		// }
		case "get_chat_rooms":
			svr <- func(s *server) {
				var b strings.Builder
				b.WriteString("setChatRoomList")
				if len(s.chats) > 0 {
					for chatroom, chat := range s.chats {
						// if chatroom == "none" {
						// 	continue
						// }
						b.WriteString(sep)
						b.WriteString(chatroom)
						b.WriteString(" - ")
						b.WriteString(strconv.Itoa(len(chat.chatters)))
						b.WriteString(" players")
					}
				} else {
					b.WriteString(sep)
					b.WriteString("No one is chatting. :(")
				}
				conn.send(b.String())
			}
		}
	}
}

func playReplay(svr chan func(*server), conn *pr2conn, replay <-chan []string) {
	for {
		args, ok := <-replay
		if !ok {
			// we're done i guess
			break
		}
		svr <- func(s *server) {
			golangbad := make([]interface{}, len(args))
			for i, a := range args {
				golangbad[i] = a
			}
			conn.send(golangbad...)
		}
	}
}

func runTCP() {
	addr, err := net.ResolveTCPAddr("tcp4", tcpAddr)
	if err != nil {
		tl.Panicln(err)
	}
	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		tl.Panicln(err)
	}
	defer l.Close()

	svr := make(chan func(*server))
	go func() {
		defer func() {
			if err := recover(); err != nil {
				ioutil.WriteFile(fmt.Sprint(time.Now().Unix())+".txt",
					[]byte(fmt.Sprintf("%v %s", err, string(debug.Stack()))), 0777)
				panic(err)
			}
		}()
		s := &server{
			players: make(map[conner]*player),
			boxes:   make(map[string][4]slotinfo),
			races:   make(map[conner]*race),
			chats:   make(map[string]chat),
		}
		for fn := range svr {
			fn(s)
		}
	}()

	tl.Println("started")
	var loginID int32
	for {
		tcpconn, err := l.AcceptTCP()
		if err != nil {
			tl.Panicln(err)
		}
		conn := newpr2conn(tcpconn)

		// login sentry
		go func() {
			id := atomic.AddInt32(&loginID, 1)
			s, err := conn.read()
			if err != nil {
				conn.close()
				return
			}

			if s[0] != "request_login_id" {
				tl.Printf("invalid login (%d): %s\n", id, s)
				conn.close()
				return
			}
			conn.send("setLoginID", id)

			loginCh := make(chan *player)
			loginsMu.Lock()
			logins[int(id)] = loginCh
			loginsMu.Unlock()

			defer func() {
				loginsMu.Lock()
				delete(logins, int(id))
				loginsMu.Unlock()
			}()

			// wait for login.php
			if p, ok := <-loginCh; ok {
				svr <- func(s *server) {
					s.players[conn] = p
				}

				tl.Println("LOGGED IN")
				// good to Go, player is permitted on the server
				go handle(conn, svr)
			}
		}()
	}
}
