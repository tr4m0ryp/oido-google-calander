// Package clientip finds the real client address behind trusted reverse
// proxies, for per-client rate limiting.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// Resolver trusts X-Forwarded-For only from configured proxy ranges.
type Resolver struct {
	trusted []netip.Prefix
}

// New builds a resolver; an empty list trusts no proxy at all.
func New(trusted []netip.Prefix) *Resolver { return &Resolver{trusted: trusted} }

func (r *Resolver) isTrusted(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Of returns the client IP. It walks X-Forwarded-For from the right, past
// trusted proxies, and stops at the first untrusted hop: entries left of it
// were written by the client and cannot be believed.
func (r *Resolver) Of(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	if !r.isTrusted(peer) {
		return peer.Unmap().String()
	}
	hops := strings.Split(strings.Join(req.Header.Values("X-Forwarded-For"), ","), ",")
	client := peer.Unmap()
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		client = hop.Unmap()
		if !r.isTrusted(client) {
			break
		}
	}
	return client.String()
}
