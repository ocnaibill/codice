package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func trust(t *testing.T, list string) []netip.Prefix {
	t.Helper()
	p, err := ParseTrustedProxies(list)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func req(remote string, xff ...string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for _, h := range xff {
		r.Header.Add("X-Forwarded-For", h)
	}
	return r
}

func TestClientIP_WithoutTrustedProxiesTheHeaderIsIgnored(t *testing.T) {
	if got := ClientIP(req("203.0.113.9:5555", "1.2.3.4"), nil); got != "203.0.113.9" {
		t.Errorf("got %s; anyone can write X-Forwarded-For, so it must not count", got)
	}
}

func TestClientIP_AHeaderFromAnUntrustedPeerIsIgnored(t *testing.T) {
	p := trust(t, "172.16.0.0/12")
	if got := ClientIP(req("203.0.113.9:5555", "1.2.3.4"), p); got != "203.0.113.9" {
		t.Errorf("got %s; a peer that is not a trusted proxy cannot vouch for anyone", got)
	}
}

func TestClientIP_BehindATrustedProxyTheClientIsTheFirstUntrustedFromTheRight(t *testing.T) {
	p := trust(t, "172.16.0.0/12,10.0.0.0/8")
	cases := map[string]struct {
		remote string
		xff    []string
		want   string
	}{
		"one proxy, one client":         {"172.18.0.5:1", []string{"198.51.100.7"}, "198.51.100.7"},
		"forged on the left is skipped": {"172.18.0.5:1", []string{"6.6.6.6, 198.51.100.7"}, "198.51.100.7"},
		"two proxies in a chain":        {"172.18.0.5:1", []string{"198.51.100.7, 10.1.2.3"}, "198.51.100.7"},
		"header sent twice":             {"172.18.0.5:1", []string{"6.6.6.6", "198.51.100.7"}, "198.51.100.7"},
		"no header: the proxy itself":   {"172.18.0.5:1", nil, "172.18.0.5"},
		"only trusted hops":             {"172.18.0.5:1", []string{"10.9.9.9"}, "172.18.0.5"},
		"garbage fails closed":          {"172.18.0.5:1", []string{"198.51.100.7, not-an-ip"}, "172.18.0.5"},
		"v4-mapped v6 folds":            {"[::ffff:172.18.0.5]:1", []string{"198.51.100.7"}, "198.51.100.7"},
		"ipv6 client":                   {"172.18.0.5:1", []string{"2001:db8::1"}, "2001:db8::1"},
	}
	for name, c := range cases {
		if got := ClientIP(req(c.remote, c.xff...), p); got != c.want {
			t.Errorf("%s: got %s, want %s", name, got, c.want)
		}
	}
}

func TestClientIP_AForgedTrustedAddressDoesNotHelpAnAttacker(t *testing.T) {
	p := trust(t, "172.16.0.0/12")
	// The attacker reaches the proxy and puts a trusted-looking address on the left.
	// The proxy appends the attacker's real address, which is what is found first.
	if got := ClientIP(req("172.18.0.5:1", "172.20.0.1, 203.0.113.66"), p); got != "203.0.113.66" {
		t.Errorf("got %s", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	if p, err := ParseTrustedProxies(""); err != nil || len(p) != 0 {
		t.Errorf("empty: %v %v", p, err)
	}
	p, err := ParseTrustedProxies(" 10.0.0.0/8 , 192.168.1.5,fd00::/8 ")
	if err != nil || len(p) != 3 || p[1].Bits() != 32 {
		t.Errorf("parsed = %v %v", p, err)
	}
	for _, bad := range []string{"10.0.0.0/33", "banana", "10.0.0.0/8,oops"} {
		if _, err := ParseTrustedProxies(bad); err == nil {
			t.Errorf("%q was accepted; a typo must stop the server, not silently trust the wrong thing", bad)
		}
	}
}

func TestRateLimitKey_GroupsIPv6ByPrefixAndFollowsTheTrustedHeader(t *testing.T) {
	key := RateLimitKey(nil)
	a, _ := key(req("[2001:db8:0:1::1]:1"))
	b, _ := key(req("[2001:db8:0:1::ffff]:1"))
	if a != b {
		t.Errorf("two hosts in one /64 got different keys: %s vs %s", a, b)
	}
	proxied := RateLimitKey(trust(t, "172.16.0.0/12"))
	x, _ := proxied(req("172.18.0.5:1", "198.51.100.1"))
	y, _ := proxied(req("172.18.0.5:1", "198.51.100.2"))
	if x == y {
		t.Error("two clients behind one proxy shared a rate-limit key")
	}
}
