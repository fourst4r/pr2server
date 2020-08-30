package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

const (
	delim = ''
	sep   = "`"
)

type conner interface {
	send(s ...interface{}) (int, error)
	read() ([]string, error)
	close() error
}

var badVCR = errors.New("vcrconn should only exist in a race")

func newvcr(out io.Writer) *vcrconn {
	return &vcrconn{
		out: out,
	}
}

type vcrconn struct {
	out     io.Writer
	sendNum int32
}

func (c *vcrconn) send(s ...interface{}) (int, error) {
	n := atomic.LoadInt32(&c.sendNum)
	s = append([]interface{}{n}, s...)
	atomic.AddInt32(&c.sendNum, 1)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprint(s[0]))
	for i := 1; i < len(s); i++ {
		buf.WriteString(sep)
		buf.WriteString(fmt.Sprint(s[i]))
	}
	hash := subhash(buf.Bytes()) + sep
	b := append([]byte(hash), buf.Bytes()...)

	// tl.Println("->", string(b))
	unixMilli := time.Now().UnixNano() / 1e6
	return fmt.Fprintf(c.out, "%d`%s\n", unixMilli, b)
}

func (c *vcrconn) read() ([]string, error) {
	panic(badVCR)
}

func (c *vcrconn) close() error {
	panic(badVCR)
}

func newpr2conn(conn *net.TCPConn) *pr2conn {
	return &pr2conn{
		conn: conn,
		r:    bufio.NewReader(conn),
	}
}

type pr2conn struct {
	conn    *net.TCPConn
	sendNum int32
	r       *bufio.Reader
}

func (c *pr2conn) send(s ...interface{}) (int, error) {
	n := atomic.LoadInt32(&c.sendNum)
	s = append([]interface{}{n}, s...)
	atomic.AddInt32(&c.sendNum, 1)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprint(s[0]))
	for i := 1; i < len(s); i++ {
		buf.WriteString(sep)
		buf.WriteString(fmt.Sprint(s[i]))
	}
	hash := subhash(buf.Bytes()) + sep
	b := append([]byte(hash), buf.Bytes()...)

	return c.conn.Write(append(b, delim))
}

// pls dont read from multiple goroutines 🙏
func (c *pr2conn) read() ([]string, error) {
	s, err := c.r.ReadString(delim)
	if err != nil {
		return nil, err
	}
	seg := strings.Split(strings.TrimSuffix(s, string(delim)), string(sep))
	if len(seg) < 3 {
		return nil, fmt.Errorf("unable to parse packet %q", s)
	}
	return seg[2:], nil
}

func (c *pr2conn) close() error {
	return c.conn.Close()
}
