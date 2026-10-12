package httpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// The session file as proxy-ipv6-rotation.py writes it.
func writeAffinity(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipv6_rotation_affinity.json")
	now := time.Now().Unix()
	data := fmt.Sprintf(`{"next_block": 1, "sessions": {
		"videoA.1": {"addr": "::1", "expires": %d},
		"old.1": {"addr": "2001:db8::2", "expires": %d}}}`, now+3600, now-10)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strings.TrimSuffix(path, ".json")+".lock", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSessionSourceReadsTheProxysAddress(t *testing.T) {
	path := writeAffinity(t)
	if got := sessionSource(path, "videoA.1"); !got.Equal(net.ParseIP("::1")) {
		t.Fatalf("videoA.1 leaves from ::1, got %v", got)
	}
	if got := sessionSource(path, "old.1"); got != nil {
		t.Fatalf("an expired session has no address, got %v", got)
	}
	if got := sessionSource(path, "unknown"); got != nil {
		t.Fatalf("an unknown session has no address, got %v", got)
	}
	if got := sessionSource(filepath.Join(t.TempDir(), "missing.json"), "videoA.1"); got != nil {
		t.Fatalf("no file, no address, got %v", got)
	}
	// The proxy (root) can't open a lock in /dev/shm that ytproxy-go created
	os.Remove(strings.TrimSuffix(path, ".json") + ".lock")
	if got := sessionSource(path, "videoA.1"); got != nil {
		t.Fatalf("without the proxy's lock, no address, got %v", got)
	}
	if _, err := os.Stat(strings.TrimSuffix(path, ".json") + ".lock"); err == nil {
		t.Fatal("ytproxy-go must never create the proxy's lock file")
	}
}

func TestH3SessionsDialFromTheSessionAddressOrFallBack(t *testing.T) {
	fallback := &http.Transport{}
	pool := &h3Sessions{
		base:        &http3.Transport{},
		entries:     map[string]*h3Session{},
		sourceState: writeAffinity(t),
		fallback:    fallback,
	}
	acquire := func(name string) http.RoundTripper {
		tr, release := pool.acquire(name)
		release()
		return tr
	}
	if acquire("") != fallback {
		t.Fatal("a request without a session must go through the proxy")
	}
	if acquire("unknown") != fallback {
		t.Fatal("a session the proxy hasn't named yet must go through the proxy")
	}
	if _, ok := pool.entries["unknown"]; ok {
		t.Fatal("a fallback must not be remembered, the next request may find the address")
	}
	a := acquire("videoA.1")
	if a == fallback || a == pool.base || a != acquire("videoA.1") {
		t.Fatal("a named session must have its own transport, reused")
	}
}

func TestBoundDialLeavesFromTheSource(t *testing.T) {
	server, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback})
	if err != nil {
		t.Skip("no IPv6 loopback:", err)
	}
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	go boundDial(net.IPv6loopback)(ctx, server.LocalAddr().String(), &tls.Config{NextProtos: []string{"h3"}}, nil)

	server.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, from, err := server.ReadFromUDP(make([]byte, 2048))
	if err != nil {
		t.Fatal("no QUIC Initial arrived:", err)
	}
	if !from.IP.Equal(net.IPv6loopback) {
		t.Fatalf("QUIC must leave from the session address ::1, came from %v", from.IP)
	}
}
