package main

import (
	"io"
	"log"
	"net"
	"os"
	"strings"
)

const (
	policyAddr = ":843"
	policyFile = `
		<?xml version="1.0"?>
		<!DOCTYPE cross-domain-policy SYSTEM "/xml/dtds/cross-domain-policy.dtd">
		<!-- Policy file for xmlsocket://socks.example.com -->
		<cross-domain-policy>
			<!-- This is a master socket policy file -->
			<!-- No other socket policies on the host will be permitted -->
			<site-control permitted-cross-domain-policies="all"/>
			<allow-access-from domain="*" to-ports="9000-10000" />
		</cross-domain-policy>` + string(0x00)
)

var pl = log.New(os.Stdout, "{POLICY} ", 0)

func runPolicy() {
	addr, err := net.ResolveTCPAddr("tcp4", policyAddr)
	if err != nil {
		pl.Fatalln(err)
	}
	l, err := net.ListenTCP("tcp4", addr)
	if err != nil {
		pl.Fatalln(err)
	}
	defer l.Close()
	pl.Println("started")
	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			pl.Fatalln(err)
		}
		go func() {
			for {
				buf := make([]byte, 512)
				nread, err := conn.Read(buf)
				if err != nil {
					if err == io.EOF {
						break
					}
					pl.Fatalln(err)
				}
				s := string(buf[:nread])
				pl.Println("<-", s)
				if s == "<policy-file-request/>\x00" {
					pl.Println("writing policy file")
					_, err := conn.Write([]byte(policyFile))
					if err != nil {
						pl.Fatalln(err)
					}
				} else if strings.Contains(s, "status") {
					conn.Write([]byte("ok\x04"))
				}
			}
		}()
	}
}
