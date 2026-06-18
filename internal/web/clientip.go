package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP returns the best-effort client IP for in-memory abuse controls.
//
// Forwarded headers (X-Forwarded-For, X-Real-IP) are honored only when the
// direct peer (RemoteAddr) is a configured trusted proxy. Otherwise they are
// attacker-controllable and ignored, so the direct peer address is used. The
// result is never persisted; it exists only for rate limiting.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return strings.TrimSpace(host)
	}
	if !proxyTrusted(peer, trusted) {
		return peer.String()
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Left-most entry is the original client as seen by the first proxy.
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if addr, err := netip.ParseAddr(first); err == nil {
			return addr.String()
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if addr, err := netip.ParseAddr(realIP); err == nil {
			return addr.String()
		}
	}
	return peer.String()
}

func proxyTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	addr = addr.Unmap()
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
