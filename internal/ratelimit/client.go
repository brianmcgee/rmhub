package ratelimit

import (
	"net/http"
	"net/netip"
	"strings"
)

func contains(nets []netip.Prefix, a netip.Addr) bool {
	for _, n := range nets {
		if n.Contains(a) {
			return true
		}
	}
	return false
}

// clientAddr returns the address of the client of the request. That is
// the peer address of the connection. If the peer is a trusted proxy,
// it is the rightmost address in X-Forwarded-For that is not a trusted
// proxy: each proxy appends the address that it saw, so the addresses
// to the left of that one come from the client and can be false.
func clientAddr(r *http.Request, trusted []netip.Prefix) netip.Addr {
	var peer netip.Addr
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		peer = ap.Addr().Unmap()
	}
	if !peer.IsValid() || !contains(trusted, peer) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		if a = a.Unmap(); !contains(trusted, a) {
			return a
		}
	}
	return peer
}

// ClientKey returns the limiter key of the client of the request. The
// key of an IPv6 address is its /64 network, because one client usually
// controls a full /64. See clientAddr for the trusted proxies.
func ClientKey(r *http.Request, trusted []netip.Prefix) string {
	a := clientAddr(r, trusted)
	switch {
	case !a.IsValid():
		return "unknown"
	case a.Is4():
		return a.String()
	}
	p, _ := a.WithZone("").Prefix(64)
	return p.String()
}
