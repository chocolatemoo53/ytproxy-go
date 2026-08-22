package httpc

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
	"git.nadeko.net/Fijxu/ytproxy-go/internal/log"
	"github.com/quic-go/quic-go/http3"
)

var Client *http.Client

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

// NewH3Client builds the HTTP/3 client, routing through a SOCKS5 proxy's
// UDP relay when config.Cfg.Proxy is set.
func NewH3Client() *http.Client {
	t := &http3.Transport{}
	if config.Cfg.Proxy != "" {
		if u, err := url.Parse(config.Cfg.Proxy); err == nil && u.Host != "" {
			switch u.Scheme {
			case "socks5", "socks5h":
				t.Dial = socks5UDPProxyDial(u.Host)
			default:
				log.Logger.Warn().Str("scheme", u.Scheme).Msg("HTTP/3 mode only supports socks5:// proxies, ignoring proxy setting")
			}
		}
	}
	return &http.Client{
		Transport: t,
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
		Proxy: func(r *http.Request) (*url.URL, error) {
			if config.Cfg.Proxy != "" {
				return url.Parse(config.Cfg.Proxy)
			}
			return nil, nil
		},
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
		Proxy: func(r *http.Request) (*url.URL, error) {
			if config.Cfg.Proxy != "" {
				return url.Parse(config.Cfg.Proxy)
			}
			return nil, nil
		},
	},
}
