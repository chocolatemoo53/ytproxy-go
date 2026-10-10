package httpc

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/quic-go/quic-go"
)

// SOCKS5 UDP ASSOCIATE support for QUIC (RFC 1928 §7).
//
// QUIC cannot traverse HTTP CONNECT proxies (TCP-only) nor plain
// net/http proxy URLs, but a SOCKS5 proxy with UDP ASSOCIATE acts as a
// generic UDP relay, which is enough to carry QUIC datagrams.
//
// The http3.Transport.Dial hook hands us the target authority; we open a
// UDP association on the proxy, expose it as a net.PacketConn that adds /
// strips the SOCKS5 UDP header, and let quic-go run on top of it.

type socksUDPConn struct {
	udp     *net.UDPConn
	control net.Conn      // TCP control connection; must stay open
	target  *net.UDPAddr  // ultimate destination (googlevideo)
	relay   *net.UDPAddr  // proxy-side relay endpoint
	// host is set when the proxy resolves the destination itself (socks5h):
	// datagrams then carry the name, target is only the address quic-go was
	// dialed with.
	host string
}

// socksProxy is how to reach a SOCKS5 proxy.
type socksProxy struct {
	Addr string
	// User names the session. A proxy that rotates its source addresses
	// (proxy-ipv6-rotation.py) keeps every connection with the same name on one
	// address, which a googlevideo URL needs: it only works from the address
	// it was issued to. The password is not used for anything.
	User string
	// RemoteDNS leaves the resolving of the destination to the proxy, needed
	// when it only has IPv6 source addresses and the local resolver prefers A
	// records.
	RemoteDNS bool
}

// socksDomainHeader builds "RSV FRAG ATYP(domain) LEN NAME PORT".
func socksDomainHeader(host string, port int) ([]byte, error) {
	if len(host) == 0 || len(host) > 255 {
		return nil, fmt.Errorf("socks5: invalid host name %q", host)
	}
	h := make([]byte, 0, 7+len(host))
	h = append(h, 0, 0, 0, 3, byte(len(host)))
	h = append(h, host...)
	h = binary.BigEndian.AppendUint16(h, uint16(port))
	return h, nil
}

var _ net.PacketConn = (*socksUDPConn)(nil)

func (c *socksUDPConn) LocalAddr() net.Addr                { return c.udp.LocalAddr() }
func (c *socksUDPConn) SetDeadline(t time.Time) error      { return c.udp.SetDeadline(t) }
func (c *socksUDPConn) SetReadDeadline(t time.Time) error  { return c.udp.SetReadDeadline(t) }
func (c *socksUDPConn) SetWriteDeadline(t time.Time) error { return c.udp.SetWriteDeadline(t) }

func (c *socksUDPConn) Close() error {
	err := c.udp.Close()
	if cerr := c.control.Close(); err == nil {
		err = cerr
	}
	return err
}

// socksHeader builds "RSV FRAG ATYP ADDR PORT".
func socksHeader(addr *net.UDPAddr) ([]byte, error) {
	if ip4 := addr.IP.To4(); ip4 != nil {
		h := make([]byte, 10)
		h[3] = 1
		copy(h[4:8], ip4)
		binary.BigEndian.PutUint16(h[8:10], uint16(addr.Port))
		return h, nil
	}
	ip16 := addr.IP.To16()
	if ip16 == nil {
		return nil, fmt.Errorf("socks5: unsupported target IP %v", addr.IP)
	}
	h := make([]byte, 22)
	h[3] = 4
	copy(h[4:20], ip16)
	binary.BigEndian.PutUint16(h[20:22], uint16(addr.Port))
	return h, nil
}

func (c *socksUDPConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	var (
		hdr []byte
		err error
	)
	if c.host != "" {
		hdr, err = socksDomainHeader(c.host, c.target.Port)
	} else {
		hdr, err = socksHeader(c.target)
	}
	if err != nil {
		return 0, err
	}
	buf := make([]byte, 0, len(hdr)+len(p))
	buf = append(buf, hdr...)
	buf = append(buf, p...)
	_, err = c.udp.WriteTo(buf, c.relay)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *socksUDPConn) ReadFrom(p []byte) (int, net.Addr, error) {
	// Max SOCKS5 UDP overhead: RSV(2)+FRAG(1)+ATYP(1)+domain(1+255)+PORT(2)
	buf := make([]byte, len(p)+262)
	n, _, err := c.udp.ReadFrom(buf)
	if err != nil {
		return 0, nil, err
	}
	if n < 5 {
		return 0, nil, errors.New("socks5: short UDP datagram")
	}
	if buf[2] != 0 {
		return 0, nil, errors.New("socks5: fragmented UDP datagrams not supported")
	}
	var (
		src     net.Addr
		payload []byte
	)
	switch buf[3] {
	case 1:
		if n < 10 {
			return 0, nil, errors.New("socks5: short IPv4 UDP datagram")
		}
		src = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:8]...)), Port: int(binary.BigEndian.Uint16(buf[8:10]))}
		payload = buf[10:n]
	case 4:
		if n < 22 {
			return 0, nil, errors.New("socks5: short IPv6 UDP datagram")
		}
		src = &net.UDPAddr{IP: net.IP(append([]byte(nil), buf[4:20]...)), Port: int(binary.BigEndian.Uint16(buf[20:22]))}
		payload = buf[22:n]
	case 3:
		dlen := int(buf[4])
		if n < 5+dlen+2 {
			return 0, nil, errors.New("socks5: short domain UDP datagram")
		}
		host := string(buf[5 : 5+dlen])
		port := int(binary.BigEndian.Uint16(buf[5+dlen : 7+dlen]))
		src, err = net.ResolveUDPAddr("udp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return 0, nil, fmt.Errorf("socks5: resolving relayed domain: %w", err)
		}
		payload = buf[5+dlen+2 : n]
	default:
		return 0, nil, fmt.Errorf("socks5: unknown ATYP %d", buf[3])
	}
	if len(payload) > len(p) {
		payload = payload[:len(p)]
	}
	copy(p, payload)
	if c.host != "" {
		// quic-go knows the connection by the address it was dialed with
		src = c.target
	}
	return len(payload), src, nil
}

// socks5UDPAssociate performs the TCP handshake (method negotiation +
// UDP ASSOCIATE) against proxyAddr and returns a PacketConn whose datagrams
// are relayed towards target by the proxy.
func socks5UDPAssociate(ctx context.Context, proxy socksProxy, target string) (*socksUDPConn, net.Addr, error) {
	proxyAddr := proxy.Addr
	var (
		targetAddr *net.UDPAddr
		remoteHost string
		err        error
	)
	if proxy.RemoteDNS {
		host, portStr, splitErr := net.SplitHostPort(target)
		if splitErr != nil {
			return nil, nil, fmt.Errorf("socks5: invalid target %s: %w", target, splitErr)
		}
		port, convErr := strconv.Atoi(portStr)
		if convErr != nil {
			return nil, nil, fmt.Errorf("socks5: invalid port in %s: %w", target, convErr)
		}
		if net.ParseIP(host) == nil {
			remoteHost = host
		}
		// What quic-go is dialed with; with a name, never used to send anything
		ip := net.ParseIP(host)
		if ip == nil {
			ip = net.ParseIP("2001:db8::1")
		}
		targetAddr = &net.UDPAddr{IP: ip, Port: port}
	} else {
		targetAddr, err = net.ResolveUDPAddr("udp", target)
		if err != nil {
			return nil, nil, fmt.Errorf("socks5: resolving %s: %w", target, err)
		}
	}

	var d net.Dialer
	control, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("socks5: connecting to proxy %s: %w", proxyAddr, err)
	}
	c, ok := control.(*net.TCPConn)
	if ok {
		_ = c.SetKeepAlive(true)
	}

	fail := func(format string, a ...any) (*socksUDPConn, net.Addr, error) {
		control.Close()
		return nil, nil, fmt.Errorf(format, a...)
	}

	// Method negotiation: a login that names the session when there is one,
	// no auth otherwise.
	if proxy.User == "" {
		if _, err := control.Write([]byte{0x05, 0x01, 0x00}); err != nil {
			return fail("socks5: sending greeting: %w", err)
		}
	} else if _, err := control.Write([]byte{0x05, 0x02, 0x00, 0x02}); err != nil {
		return fail("socks5: sending greeting: %w", err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(control, greeting); err != nil {
		return fail("socks5: reading method reply: %w", err)
	}
	switch {
	case greeting[0] == 0x05 && greeting[1] == 0x00:
	case greeting[0] == 0x05 && greeting[1] == 0x02 && proxy.User != "":
		if len(proxy.User) > 255 {
			return fail("socks5: session name too long")
		}
		login := []byte{0x01, byte(len(proxy.User))}
		login = append(login, proxy.User...)
		login = append(login, 0x01, 'x')
		if _, err := control.Write(login); err != nil {
			return fail("socks5: sending login: %w", err)
		}
		status := make([]byte, 2)
		if _, err := io.ReadFull(control, status); err != nil {
			return fail("socks5: reading login reply: %w", err)
		}
		if status[1] != 0x00 {
			return fail("socks5: login refused (status %d)", status[1])
		}
	default:
		return fail("socks5: proxy refused the method (got %x %x)", greeting[0], greeting[1])
	}

	// UDP ASSOCIATE with wildcard target.
	// VER CMD RSV ATYP(IPv4) ADDR(4×0) PORT(2×0) — exactly 10 bytes.
	if _, err := control.Write([]byte{0x05, 0x03, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return fail("socks5: sending UDP ASSOCIATE: %w", err)
	}
	reply := make([]byte, 262)
	if _, err := io.ReadFull(control, reply[:4]); err != nil {
		return fail("socks5: reading ASSOCIATE reply: %w", err)
	}
	if reply[1] != 0x00 {
		return fail("socks5: UDP ASSOCIATE rejected (code %d)", reply[1])
	}
	var relayHost string
	switch reply[3] {
	case 0x01:
		if _, err := io.ReadFull(control, reply[4:10]); err != nil {
			return fail("socks5: reading BND.ADDR: %w", err)
		}
		ip := net.IP(reply[4:8])
		port := binary.BigEndian.Uint16(reply[8:10])
		// BND.ADDR 0.0.0.0 => reach the relay at the proxy's own address.
		if ip.Equal(net.IPv4zero) {
			host, _, _ := net.SplitHostPort(proxyAddr)
			relayHost = host
		} else {
			relayHost = ip.String()
		}
		relayHost = net.JoinHostPort(relayHost, strconv.Itoa(int(port)))
	case 0x04:
		if _, err := io.ReadFull(control, reply[4:22]); err != nil {
			return fail("socks5: reading BND.ADDR: %w", err)
		}
		port := binary.BigEndian.Uint16(reply[20:22])
		relayHost = net.JoinHostPort(net.IP(reply[4:20]).String(), strconv.Itoa(int(port)))
	case 0x03:
		dlen := int(reply[4])
		if _, err := io.ReadFull(control, reply[4:4+dlen+2]); err != nil {
			return fail("socks5: reading BND.ADDR: %w", err)
		}
		relayHost = net.JoinHostPort(string(reply[5:5+dlen]), strconv.Itoa(int(binary.BigEndian.Uint16(reply[5+dlen:7+dlen]))))
	default:
		return fail("socks5: unknown BND ATYP %d", reply[3])
	}

	relay, err := net.ResolveUDPAddr("udp", relayHost)
	if err != nil {
		return fail("socks5: resolving relay %s: %w", relayHost, err)
	}

	// Keep the target address family compatible with the relay: a v4-only
	// relay cannot forward datagrams addressed to a literal IPv6 target
	// (e.g. googlevideo's AAAA), which would blackhole QUIC silently.
	// Re-resolve the target hostname for IPv4 in that case.
	if remoteHost == "" && relay.IP.To4() != nil && targetAddr.IP != nil && targetAddr.IP.To4() == nil {
		host, portStr, splitErr := net.SplitHostPort(target)
		if splitErr == nil && net.ParseIP(host) == nil {
			ips, lookupErr := net.DefaultResolver.LookupIP(ctx, "ip4", host)
			if lookupErr != nil || len(ips) == 0 {
				return fail("socks5: v4 relay but target %s has no A record: %v", host, lookupErr)
			}
			port, _ := strconv.Atoi(portStr)
			targetAddr = &net.UDPAddr{IP: ips[0], Port: port}
		}
	}

	udp, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return fail("socks5: binding local UDP socket: %w", err)
	}
	return &socksUDPConn{udp: udp, control: control, target: targetAddr, relay: relay, host: remoteHost}, targetAddr, nil
}

// socks5UDPProxyDial returns a Dial function suitable for http3.Transport
// that tunnels every QUIC connection through the SOCKS5 proxy at proxyAddr.
func socks5UDPProxyDial(proxy socksProxy) func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	return func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
		pconn, target, err := socks5UDPAssociate(ctx, proxy, addr)
		if err != nil {
			return nil, err
		}
		return quic.Dial(ctx, pconn, target, tlsCfg, cfg)
	}
}
