package main

import (
	"os"
	"os/signal"
	"sync"
)

const TODO = `to-do list:
- sort finish times
- fix finish time display error when some1 quits during drawing
- objective mode
- slot times
- jelly & jigg hats
- prob other things
<img src='https://cdn.discordapp.com/emojis/640357767970029569.gif?v=1'>
`

var (
	logins   map[int]chan *player = make(map[int]chan *player)
	loginsMu sync.RWMutex
)

func main() {

	go runTCP()
	go runHTTP()
	go runPolicy()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
