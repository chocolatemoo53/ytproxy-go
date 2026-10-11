package httpc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"github.com/quic-go/quic-go/http3"
)

// A SOCKS5 server for one UDP association that checks what the client sends: the
// login that names the session and a host name as the destination of a datagram.
type fakeSocks struct {
	ln      net.Listener
	user    chan string
	dstHost chan string
	dstPort chan int
	relay   *net.UDPConn
}

func startFakeSocks(t *testing.T, replyWithIPv6Source bool) *fakeSocks {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSocks{ln: ln, relay: relay, user: make(chan string, 1), dstHost: make(chan string, 1), dstPort: make(chan int, 1)}
	t.Cleanup(func() { ln.Close(); relay.Close() })

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		hello := make([]byte, 4)
		if _, err := io.ReadFull(c, hello); err != nil || hello[1] != 2 {
			return
		}
		c.Write([]byte{5, 2}) // the login method
		head := make([]byte, 2)
		io.ReadFull(c, head)
		name := make([]byte, head[1])
		io.ReadFull(c, name)
		plen := make([]byte, 1)
		io.ReadFull(c, plen)
		io.ReadFull(c, make([]byte, plen[0]))
		f.user <- string(name)
		c.Write([]byte{1, 0})

		req := make([]byte, 10)
		io.ReadFull(c, req) // VER CMD RSV ATYP ADDR(4) PORT(2)
		port := relay.LocalAddr().(*net.UDPAddr).Port
		reply := []byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}
		binary.BigEndian.PutUint16(reply[8:], uint16(port))
		c.Write(reply)

		buf := make([]byte, 2048)
		n, from, err := relay.ReadFromUDP(buf)
		if err != nil || n < 7 || buf[3] != 3 {
			return
		}
		l := int(buf[4])
		f.dstHost <- string(buf[5 : 5+l])
		f.dstPort <- int(binary.BigEndian.Uint16(buf[5+l : 7+l]))
		payload := buf[7+l : n]

		// Answer from an IPv6 address, as the proxy does
		out := append([]byte{0, 0, 0, 4}, net.ParseIP("2001:db8::7").To16()...)
		out = binary.BigEndian.AppendUint16(out, 443)
		relay.WriteToUDP(append(out, payload...), from)
		io.Copy(io.Discard, c)
	}()
	return f
}

func TestSocksSessionLoginAndHostName(t *testing.T) {
	f := startFakeSocks(t, true)
	proxy := socksProxy{Addr: f.ln.Addr().String(), User: "dQw4w9WgXcQ", RemoteDNS: true}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pconn, target, err := socks5UDPAssociate(ctx, proxy, "rr3---sn-example.googlevideo.com:443")
	if err != nil {
		t.Fatal(err)
	}
	defer pconn.Close()

	if got := <-f.user; got != "dQw4w9WgXcQ" {
		t.Fatalf("login name = %q", got)
	}
	if _, err := pconn.WriteTo([]byte("quic initial"), target); err != nil {
		t.Fatal(err)
	}
	if h := <-f.dstHost; h != "rr3---sn-example.googlevideo.com" {
		t.Fatalf("destination host = %q (the name must go to the proxy, not an address)", h)
	}
	if p := <-f.dstPort; p != 443 {
		t.Fatalf("destination port = %d", p)
	}

	pconn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, src, err := pconn.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "quic initial" {
		t.Fatalf("payload = %q", buf[:n])
	}
	if src.String() != target.String() {
		t.Fatalf("source = %v, want the dialed address %v (quic-go knows the connection by it)", src, target)
	}
}

func TestSocksNoLoginWithoutSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan []byte, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		hello := make([]byte, 3)
		io.ReadFull(c, hello)
		got <- hello
		c.Write([]byte{5, 0xff})
	}()
	_, _, err = socks5UDPAssociate(context.Background(), socksProxy{Addr: ln.Addr().String()}, "127.0.0.1:443")
	if err == nil {
		t.Fatal("a refused method must fail")
	}
	if h := <-got; h[0] != 5 || h[1] != 1 || h[2] != 0 {
		t.Fatalf("greeting without a session = %x, want no-auth only", h)
	}
}

func TestProxyForNamesTheSession(t *testing.T) {
	t.Setenv("YTPROXY_PROXY", "http://proxy.internal:8899")
	config.LoadConfig()
	req, _ := http.NewRequest("GET", "https://example.com/", nil)

	u, err := proxyFor(req)
	if err != nil || u.User != nil {
		t.Fatalf("no session: user = %v, err = %v", u.User, err)
	}
	u, _ = proxyFor(req.WithContext(WithSession(req.Context(), "abc123")))
	if pw, _ := u.User.Password(); u.User.Username() != "abc123" || pw != sessionPassword {
		t.Fatalf("session login = %v", u.User)
	}

	t.Setenv("YTPROXY_PROXY", "http://user:secret@proxy.internal:8899")
	config.LoadConfig()
	u, _ = proxyFor(req.WithContext(WithSession(req.Context(), "abc123")))
	if u.User.Username() != "user" {
		t.Fatalf("a login in the proxy URL must stay, got %v", u.User)
	}
}

func TestH3SessionsUseOneTransportPerSession(t *testing.T) {
	pool := &h3Sessions{
		proxy:   socksProxy{Addr: "127.0.0.1:1080", RemoteDNS: true},
		base:    &http3.Transport{},
		entries: map[string]*h3Session{},
	}
	transport := func(name string) *http3.Transport {
		tr, release := pool.acquire(name)
		release()
		return tr
	}
	a := transport("videoA")
	if a != transport("videoA") {
		t.Fatal("the same session must reuse its transport (and its QUIC connections)")
	}
	if a == transport("videoB") {
		t.Fatal("another session must not share a transport")
	}
	if transport("") != pool.base {
		t.Fatal("a request without a session uses the base transport")
	}
	// No cap: a busy instance keeps every playing video's connections
	for i := 0; i < 300; i++ {
		transport(fmt.Sprintf("v%d", i))
	}
	if len(pool.entries) != 302 {
		t.Fatalf("every session keeps its transport, got %d", len(pool.entries))
	}

	// Idle sessions are closed, one with a request in flight is kept however old
	_, release := pool.acquire("streaming")
	old := time.Now().Add(-2 * h3SessionIdle)
	for _, s := range pool.entries {
		s.lastUsed = old
	}
	pool.lastSweep = old
	transport("fresh")
	if _, ok := pool.entries["streaming"]; !ok {
		t.Fatal("a session with a request in flight must not be closed")
	}
	if _, ok := pool.entries["videoA"]; ok || len(pool.entries) != 2 {
		t.Fatalf("idle sessions must be closed, %d left", len(pool.entries))
	}
	release()

	direct := &h3Sessions{base: &http3.Transport{}, entries: map[string]*h3Session{}}
	if tr, _ := direct.acquire("videoA"); tr != direct.base {
		t.Fatal("without a proxy there is nothing to name a session to")
	}
}
