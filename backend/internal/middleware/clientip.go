package middleware

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/go-chi/httprate"
)

// ParseTrustedProxies reads a comma-separated list of CIDRs (or single addresses)
// naming the reverse proxies allowed to say who the client is. An empty list
// means no proxy is trusted. A bad entry is an error, so a typo stops the server
// at startup instead of silently trusting nothing, or worse, everything.
func ParseTrustedProxies(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(part)
		if err != nil {
			return nil, errors.New("CODICE_TRUSTED_PROXIES: " + part + " is neither a CIDR nor an address")
		}
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

func inAny(addr netip.Addr, prefixes []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIP returns the address of whoever is really calling.
//
// By default it is the address of the connection, and X-Forwarded-For is ignored:
// anyone can send that header. Only when the connection itself comes from one of
// the trusted proxies is the header read, from the right, skipping other trusted
// proxies: the first address that is not trusted is the client. Whatever a client
// wrote further left is never reached, so it cannot be forged into the answer.
// If the header is malformed the proxy's own address is used (fail closed).
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if len(trusted) == 0 || !inAny(remote, trusted) {
		return remote.String()
	}

	var chain []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		chain = append(chain, strings.Split(header, ",")...)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return remote.String() // garbage in the chain: trust nothing left of it
		}
		if !inAny(ip, trusted) {
			return ip.Unmap().String()
		}
	}
	return remote.String()
}

// RateLimitKey is the key for per-client rate limits: the client address, with IPv6
// grouped by /64 so one host cannot dodge the limit by rotating addresses.
func RateLimitKey(trusted []netip.Prefix) func(*http.Request) (string, error) {
	return func(r *http.Request) (string, error) {
		return httprate.CanonicalizeIP(ClientIP(r, trusted)), nil
	}
}
