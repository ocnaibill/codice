package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestParsePublicURL(t *testing.T) {
	good := map[string]string{
		"":                            "",
		"   ":                         "",
		"https://livros.example.com":  "https://livros.example.com",
		"https://livros.example.com/": "https://livros.example.com",
		" http://192.168.1.6:8088 ":   "http://192.168.1.6:8088",
		"https://[::1]:8443":          "https://[::1]:8443",
	}
	for in, want := range good {
		got, err := ParsePublicURL(in)
		if err != nil || got != want {
			t.Errorf("ParsePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"livros.example.com", "ftp://livros.example.com", "https://", "https:///x", "//livros.example.com",
		"https://livros.example.com/opds", "https://livros.example.com?x=1", "https://livros.example.com/#a",
		"https://user:pw@livros.example.com", "https://:8443",
	} {
		if got, err := ParsePublicURL(in); err == nil {
			t.Errorf("ParsePublicURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestRequestScheme(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.77.10/32")}
	req := func(remote, proto string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if proto != "" {
			r.Header.Set("X-Forwarded-Proto", proto)
		}
		return r
	}
	cases := []struct {
		name  string
		r     *http.Request
		trust []netip.Prefix
		want  string
	}{
		{"proxy says https", req("172.30.77.10:5555", "https"), trusted, "https"},
		{"proxy says HTTPS in capitals, padded", req("172.30.77.10:5555", " HTTPS "), trusted, "https"},
		{"proxy says http", req("172.30.77.10:5555", "http"), trusted, "http"},
		{"proxy says nothing", req("172.30.77.10:5555", ""), trusted, "http"},
		{"first of a list is the person's", req("172.30.77.10:5555", "https, http"), trusted, "https"},
		{"first of a list is http", req("172.30.77.10:5555", "http, https"), trusted, "http"},
		{"garbage", req("172.30.77.10:5555", "gopher"), trusted, "http"},
		{"untrusted peer cannot say https", req("203.0.113.9:5555", "https"), trusted, "http"},
		{"no proxy trusted at all", req("172.30.77.10:5555", "https"), nil, "http"},
		{"address with no port", req("172.30.77.10", "https"), trusted, "https"},
		{"remote that is not an address", req("pipe", "https"), trusted, "http"},
		{"IPv4 as IPv6", req("[::ffff:172.30.77.10]:5555", "https"), trusted, "https"},
	}
	for _, c := range cases {
		if got := RequestScheme(c.r, c.trust); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	direct := req("203.0.113.9:5555", "")
	direct.TLS = &tls.ConnectionState{}
	if got := RequestScheme(direct, nil); got != "https" {
		t.Errorf("a TLS connection: got %q, want https", got)
	}
}
