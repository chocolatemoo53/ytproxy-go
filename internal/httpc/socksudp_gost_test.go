package httpc

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

// Integration test: requires a real SOCKS5 server with UDP ASSOCIATE on
// 127.0.0.1:1080 (e.g.: gost -L 'socks5://127.0.0.1:1080').
// Skips silently when no listener is present.
func TestGostUDPAssociateEndToEnd(t *testing.T) {
	if _, err := net.DialTimeout("tcp", "127.0.0.1:1080", time.Second); err != nil {
		t.Skip("no SOCKS5 server on 127.0.0.1:1080")
	}

	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := echo.ReadFrom(buf)
			if err != nil {
				return
			}
			echo.WriteTo(buf[:n], from)
		}
	}()

	pconn, relayedTo, err := socks5UDPAssociate(
		context.Background(), "127.0.0.1:1080", echo.LocalAddr().String(),
	)
	if err != nil {
		t.Fatalf("ASSOCIATE via gost failed: %v", err)
	}
	defer pconn.Close()
	t.Logf("associated: relay=%v target=%v", pconn.relay, relayedTo)

	payload := []byte("hello quic-over-gost")
	if _, err := pconn.WriteTo(payload, relayedTo); err != nil {
		t.Fatalf("write: %v", err)
	}
	pconn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, len(payload)+64)
	n, _, err := pconn.ReadFrom(got)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got[:n], payload) {
		t.Fatalf("echo mismatch: %q", got[:n])
	}
}
