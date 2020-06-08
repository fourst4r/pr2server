package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

const (
	httpAddr  = ":80"
	pr2hubURL = "https://pr2hub.com/"
)

var hl = log.New(os.Stdout, "{HTTP} ", 0)

func jsonerr(err error) []byte {
	return []byte(fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error()))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	b, err := decryptLoginString(r.PostForm.Get("i"))
	if err != nil {
		hl.Println("i:", r.PostForm.Get("i"))
		w.Write(jsonerr(err))
		return
	}
	hl.Println(string(b))
	var l login
	err = json.Unmarshal(b, &l)
	if err != nil {
		w.Write(jsonerr(err))
		return
	}

	// net.ParseIP()
	addr := r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	hl.Println("login:", addr, l.Server.ServerName, l.UserName)
	rootTunnel <- msg{addr: addr, data: l}

	w.Write([]byte(`{"success":true,"token":"813531-b6c8b446b427d6bb42462e48722e26","email":false,"ant":false,"time":1591549666,"lastRead":"25354696","lastRecv":"25354696","guild":"0","guildOwner":0,"guildName":"","emblem":"","userId":813531}`))
}

func crossdomainHandler(w http.ResponseWriter, r *http.Request) {
	hl.Println("serving crossdomain.xml")

	w.Header().Add("Content-Type", "text/x-cross-domain-policy")
	w.Write([]byte(`
	<?xml version="1.0" ?>
	<cross-domain-policy>
	<allow-access-from domain="*" />
	</cross-domain-policy>
	`))
}

func serverstatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"servers":[{"server_id":"1","server_name":"Alpha","address":"localhost","port":"9160","population":"33","status":"open","guild_id":"0","tournament":"0","happy_hour":0}]}`))
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	redirectedurl := pr2hubURL + r.URL.Path + "?" + r.URL.RawQuery
	hl.Println("proxying", r.Method, redirectedurl)

	req, err := http.NewRequest(r.Method, redirectedurl, r.Body)
	if err != nil {
		log.Println(err)
		return
	}
	for name, values := range r.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// the sole reason this proxy exists...
	req.Header.Set("Referer", pr2hubURL)

	resp, err := http.DefaultClient.Do(req)
	r.Body.Close()

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	resp.Body.Close()
}

func runHTTP() {
	http.HandleFunc("/files/server_status_2.txt", serverstatusHandler)
	http.HandleFunc("/crossdomain.xml", crossdomainHandler)
	http.HandleFunc("/login.php", loginHandler)
	http.HandleFunc("/", proxyHandler)

	hl.Println("started")
	err := http.ListenAndServe(httpAddr, nil)
	if err != nil {
		log.Panicln(err)
	}
}

func httpgetjson(url string, v interface{}) error {
	b, err := httpget(url)
	if err != nil {
		return err
	}
	hl.Println(string(b))
	return json.Unmarshal(b, v)
}
func httpget(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	respBody, err := ioutil.ReadAll(resp.Body)

	return respBody, err
}
