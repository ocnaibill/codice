package middleware

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ParsePublicURL reads CODICE_PUBLIC_URL: the address people type to reach the app from
// outside, as "https://livros.example.com" (a port is fine). Empty means "not set". It is
// the scheme, the host and nothing else: the app lives at the root, so a path, a query or
// credentials are a mistake that would put wrong links in the OPDS catalog, and are
// refused at startup instead.
func ParsePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		return "", errors.New("CODICE_PUBLIC_URL must look like https://livros.example.com")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("CODICE_PUBLIC_URL is the address only (scheme, host and port), without a path, query or credentials")
	}
	return u.Scheme + "://" + u.Host, nil
}

// RequestScheme is "https" or "http", as the person who made the request sees it. The
// connection that reaches the app is usually plain HTTP from a proxy that ended the TLS,
// so the proxy's X-Forwarded-Proto says what the person used. Like X-Forwarded-For, it is
// believed only when the connection itself comes from a trusted proxy: anyone else can
// write that header.
func RequestScheme(r *http.Request, trusted []netip.Prefix) string {
	if r.TLS != nil {
		return "https"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil || !inAny(remote, trusted) {
		return "http"
	}
	// Several proxies in a row may each have added a value: the first is the one the
	// person's own connection used.
	first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	if strings.EqualFold(strings.TrimSpace(first), "https") {
		return "https"
	}
	return "http"
}
