package main

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

func Replay(r io.Reader) <-chan []string {
	ch := make(chan []string)
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Split(bufio.ScanLines)

		prev := time.Unix(0, 0)
		for scanner.Scan() {
			next, pkt, err := parsePkt(scanner.Text())
			if err != nil {
				tl.Println("bad replay packet:", err)
			}
			tl.Println("time.After() ", next.Sub(prev))
			if prev != time.Unix(0, 0) {
				<-time.After(next.Sub(prev))
			}
			ch <- pkt
			prev = next
		}
		if err := scanner.Err(); err != nil {
			tl.Println("scanner stopped:", err)
		}
		close(ch)
	}()
	return ch
}

func parsePkt(pkt string) (t time.Time, args []string, err error) {
	// spl := strings.SplitN(pkt, "`", 2)
	spl := strings.Split(pkt, "`")
	if len(spl) < 3 {
		err = errors.New("replay: short pkt")
		return
	}
	nano, err := strconv.Atoi(spl[0])
	if err != nil {
		return
	}
	return time.Unix(0, int64(nano)*1e6), spl[3:], nil
}
