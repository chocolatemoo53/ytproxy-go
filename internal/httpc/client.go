package httpc

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"github.com/quic-go/quic-go/http3"
)

var Client *http.Client

type sessionKey struct{}

// sessionPassword goes with the session name when it is sent as a proxy login.
// Proxies that rotate their source address only look at the name.
const sessionPassword = "companion"

// WithSession names the session a request belongs to (the YouTube video id).
// A proxy that rotates its source addresses keeps all requests of a session on
// one address, which googlevideo needs: a URL only works from the address it
// was issued to.
func WithSession(ctx context.Context, name string) context.Context {
	if name == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, name)
}

func sessionFrom(ctx context.Context) string {
	name, _ := ctx.Value(sessionKey{}).(string)
	return name
}

// proxyFor is the proxy of a request: the configured one, logged in with the
// session name of the request unless the URL has a login of its own.
func proxyFor(r *http.Request) (*url.URL, error) {
	if config.Cfg.Proxy == "" {
		return nil, nil
	}
	u, err := url.Parse(config.Cfg.Proxy)
	if err != nil {
		return nil, err
	}
	if name := sessionFrom(r.Context()); name != "" && u.User == nil {
		u.User = url.UserPassword(name, sessionPassword)
	}
	return u, nil
}

var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

// QUIC doesn't seem to support HTTP nor SOCKS5 proxies due to how it's made.
// (Since it's UDP)
//
// However, SOCKS5 UDP ASSOCIATE (RFC 1928 §7) turns a proxy into a generic
// UDP relay, which is enough to carry QUIC. NewH3Client wires that up via
// the http3.Transport.Dial hook when a socks5:// proxy is configured.
//
// NOTE: no overall client timeout on purpose; Client.Timeout also caps the
// time spent streaming the response body, which would abort long
// /videoplayback transfers mid-stream.
var H3client = &http.Client{
	Transport: &http3.Transport{},
}

// Every session has its own HTTP/3 transport (and so its own QUIC
// connections) when the proxy is told the session name: a connection reused for
// another video would leave from the address of the first one.
//
// There is no cap on the number of sessions (a cap closed the transports of
// videos still playing once more were open). A transport is closed once none of
// its requests has been in flight for h3SessionIdle instead.
const h3SessionIdle = 10 * time.Minute

type h3Session struct {
	t        *http3.Transport
	inFlight int
	lastUsed time.Time
}

type h3Sessions struct {
	mu        sync.Mutex
	proxy     socksProxy
	base      *http3.Transport
	entries   map[string]*h3Session
	lastSweep time.Time
}

// acquire returns the session's transport and counts a request in flight on
// it; release ends that request's turn (safe to call more than once).
func (p *h3Sessions) acquire(name string) (t *http3.Transport, release func()) {
	if name == "" || p.proxy.Addr == "" {
		return p.base, func() {}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if now.Sub(p.lastSweep) > time.Minute {
		p.lastSweep = now
		for key, s := range p.entries {
			if s.inFlight == 0 && now.Sub(s.lastUsed) > h3SessionIdle {
				delete(p.entries, key)
				_ = s.t.Close()
			}
		}
	}
	s, ok := p.entries[name]
	if !ok {
		proxy := p.proxy
		proxy.User = name
		s = &h3Session{t: &http3.Transport{Dial: socks5UDPProxyDial(proxy)}}
		p.entries[name] = s
	}
	s.inFlight++
	s.lastUsed = now
	var once sync.Once
	return s.t, func() {
		once.Do(func() {
			p.mu.Lock()
			s.inFlight--
			s.lastUsed = time.Now()
			p.mu.Unlock()
		})
	}
}

func (p *h3Sessions) RoundTrip(req *http.Request) (*http.Response, error) {
	t, release := p.acquire(sessionFrom(req.Context()))
	resp, err := t.RoundTrip(req)
	if err != nil || resp.Body == nil {
		release()
		return resp, err
	}
	resp.Body = &releasingBody{ReadCloser: resp.Body, release: release}
	return resp, nil
}

// releasingBody ends the request's turn on its session once the body is closed.
type releasingBody struct {
	io.ReadCloser
	release func()
}

func (b *releasingBody) Close() error {
	err := b.ReadCloser.Close()
	b.release()
	return err
}

// socksProxyFrom reads a socks5:// or socks5h:// proxy URL. socks5h leaves the
// resolving of googlevideo's name to the proxy.
func socksProxyFrom(u *url.URL) (socksProxy, bool) {
	switch u.Scheme {
	case "socks5", "socks5h":
		return socksProxy{Addr: u.Host, RemoteDNS: u.Scheme == "socks5h"}, u.Host != ""
	}
	return socksProxy{}, false
}

// NewH3Client builds the HTTP/3 client, routing through a SOCKS5 proxy's
// UDP relay when config.Cfg.Proxy is set.
func NewH3Client() *http.Client {
	pool := &h3Sessions{base: &http3.Transport{}, entries: map[string]*h3Session{}}
	if config.Cfg.Proxy != "" {
		u, err := url.Parse(config.Cfg.Proxy)
		if proxy, ok := socksProxyFrom(u); err == nil && ok {
			pool.proxy = proxy
			pool.base.Dial = socks5UDPProxyDial(proxy)
		} else {
			log.Logger.Warn().Str("scheme", u.Scheme).Msg("HTTP/3 mode only supports socks5:// and socks5h:// proxies, ignoring proxy setting")
		}
	}
	return &http.Client{
		Transport: pool,
	}
}

// http/1.1 client
var H1_1client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			var net string
			if config.Cfg.Ipv6_only {
				net = "tcp6"
			} else {
				net = "tcp"
			}
			return dialer.Dial(net, addr)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		ReadBufferSize:        16 * 1024,
		MaxConnsPerHost:       0,
		MaxIdleConnsPerHost:   10,
		MaxIdleConns:          0,
		Proxy:                 proxyFor,
		// Prevent switching to HTTP/2
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	},
}

// http/2 client
var H2client = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		Dial: func(network, addr string) (net.Conn, error) {
			var net string
			if config.Cfg.Ipv6_only {
				net = "tcp6"
			} else {
				net = "tcp"
			}
			return dialer.Dial(net, addr)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		ReadBufferSize:        16 * 1024,
		ForceAttemptHTTP2:     true,
		MaxConnsPerHost:       0,
		MaxIdleConnsPerHost:   10,
		MaxIdleConns:          0,
		Proxy:                 proxyFor,
	},
}
