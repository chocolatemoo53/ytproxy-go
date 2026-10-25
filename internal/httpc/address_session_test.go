package httpc

import (
	"testing"

	"git.nadeko.net/Fijxu/ytproxy-go/internal/config"
)

func TestAddressSessionOnlyForTheProxysOwnBlocks(t *testing.T) {
	config.LoadConfig()
	config.Cfg.Source_prefixes = []string{"2602:fb54:1434:60c0::/58"}
	defer func() { config.Cfg.Source_prefixes = nil }()

	cases := map[string]string{
		"2602:fb54:1434:60ea:c7c7:4c4b:d61a:71": "ip:2602:fb54:1434:60ea:c7c7:4c4b:d61a:71",
		"2602:fb54:1434:6037::1":                "",
		"2001:db8::1":                           "",
		"203.0.113.7":                           "",
		"":                                      "",
		"not-an-address":                        "",
	}
	for ip, want := range cases {
		if got := AddressSession(ip); got != want {
			t.Errorf("%q: got %q, want %q", ip, got, want)
		}
	}
}
