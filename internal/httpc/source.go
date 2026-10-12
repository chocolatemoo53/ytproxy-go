package httpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
)

// Sending QUIC from the rotation proxy's own source addresses.
//
// The IPv6 rotation proxy (proxy-ipv6-rotation.py of the companion) keeps the
// address of every named session in a JSON file in /dev/shm. Run in the proxy's
// network and IPC namespaces, ytproxy-go reads a session's address from that
// file and binds its QUIC socket to it: HTTP/3 leaves from the address the
// video's URLs were issued to, with no relay in between.

type affinityState struct {
	Sessions map[string]struct {
		Addr    string  `json:"addr"`
		Expires float64 `json:"expires"`
	} `json:"sessions"`
}

// sessionSource is the address the proxy gave the session `name`, nil when it
// has none (yet).
func sessionSource(path, name string) net.IP {
	// The proxy rewrites the file under an exclusive flock of the .lock next to it.
	// Never create the lock: in sticky /dev/shm (fs.protected_regular) the proxy's
	// root couldn't open one we own, and would lose its shared session state.
	lock, err := os.Open(strings.TrimSuffix(path, ".json") + ".lock")
	if err != nil {
		return nil
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_SH); err != nil {
		return nil
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state affinityState
	if json.Unmarshal(data, &state) != nil {
		return nil
	}
	entry, ok := state.Sessions[name]
	if !ok || entry.Expires < float64(time.Now().Unix()) {
		return nil
	}
	return net.ParseIP(entry.Addr)
}

// boundDial dials QUIC from a socket bound to `source`.
func boundDial(source net.IP) func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	return func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
		target, err := net.ResolveUDPAddr("udp6", addr)
		if err != nil {
			return nil, err
		}
		pconn, err := net.ListenUDP("udp6", &net.UDPAddr{IP: source})
		if err != nil {
			return nil, err
		}
		return dialOwning(ctx, pconn, target, tlsCfg, cfg)
	}
}

// dialOwning is quic.Dial, closing pconn once the connection ends: quic.Dial
// leaves a PacketConn it was handed open.
func dialOwning(ctx context.Context, pconn net.PacketConn, target net.Addr, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	conn, err := quic.Dial(ctx, pconn, target, tlsCfg, cfg)
	if err != nil {
		pconn.Close()
		return nil, err
	}
	go func() {
		<-conn.Context().Done()
		pconn.Close()
	}()
	return conn, nil
}
