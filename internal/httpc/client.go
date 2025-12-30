package httpc

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"git.nadeko.net/Fijxu/http3-ytproxy/internal/config"
	"github.com/quic-go/quic-go/http3"
)

var Client *http.Client

var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

// QUIC doesn't seem to support HTTP nor SOCKS5 proxies due to how it's made.
// (Since it's UDP)
var H3client = &http.Client{
	Transport: &http3.Transport{},
	Timeout:   10 * time.Second,
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
