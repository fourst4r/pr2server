package main

import (
	"os"
	"os/signal"
	"strings"
)

func trimport(addr string) string {
	return addr[:strings.LastIndex(addr, ":")]
}

type msg struct {
	addr string
	data interface{}
}

// http-to-tcp tunnel
var rootTunnel = make(chan msg)

func main() {
	go runTCP()
	go runHTTP()
	go runPolicy()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
