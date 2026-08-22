package httpc

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestSocksHeader(t *testing.T) {
	h, err := socksHeader(&net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 443})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0, 1, 1, 2, 3, 4, 0x01, 0xbb}
	if !bytes.Equal(h, want) {
		t.Fatalf("ipv4 header = %x, want %x", h, want)
	}

	ip6 := net.ParseIP("2001:db8::1")
	h, err = socksHeader(&net.UDPAddr{IP: ip6, Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	if h[3] != 4 || len(h) != 22 {
		t.Fatalf("ipv6 header shape = %x", h)
	}
	if !bytes.Equal(h[4:20], ip6.To16()) || binary.BigEndian.Uint16(h[20:22]) != 80 {
		t.Fatalf("ipv6 header content = %x", h)
	}
}

// Echo relay: strips the SOCKS5 header from incoming datagrams, re-wraps the
// payload with a source header, sends it back. Validates both directions.
func TestSocksUDPConnRoundTrip(t *testing.T) {
	relay, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := relay.ReadFrom(buf)
			if err != nil {
				return
			}
			// buf: RSV FRAG ATYP ADDR PORT payload
			payload := buf[10:n] // assume IPv4 ATYP written by us
			out := make([]byte, 10+len(payload))
			copy(out, []byte{0, 0, 0, 1, 9, 9, 9, 9})
			binary.BigEndian.PutUint16(out[8:10], 1234)
			copy(out[10:], payload)
			relay.WriteTo(out, from)
		}
	}()

	target := &net.UDPAddr{IP: net.IPv4(200, 1, 2, 3), Port: 443}
	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	c := &socksUDPConn{
		udp:     local,
		control: nil,
		target:  target,
		relay:   relay.LocalAddr().(*net.UDPAddr),
	}

	payload := []byte("quic initial packet bytes")
	if _, err := c.WriteTo(payload, target); err != nil {
		t.Fatal(err)
	}

	got := make([]byte, len(payload)+64)
	n, src, err := c.ReadFrom(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:n], payload) {
		t.Fatalf("payload mismatch: %q", got[:n])
	}
	srcAddr, ok := src.(*net.UDPAddr)
	if !ok || !srcAddr.IP.Equal(net.IPv4(9, 9, 9, 9)) || srcAddr.Port != 1234 {
		t.Fatalf("parsed source addr = %v", src)
	}
}
