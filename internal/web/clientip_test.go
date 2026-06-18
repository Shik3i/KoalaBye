package web

import (
	"net/http"
	"net/netip"
	"testing"
)

func mustPrefixes(t *testing.T, entries ...string) []netip.Prefix {
	t.Helper()
	var out []netip.Prefix
	for _, e := range entries {
		p, err := netip.ParsePrefix(e)
		if err != nil {
			t.Fatalf("parse prefix %q: %v", e, err)
		}
		out = append(out, p)
	}
	return out
}

func TestClientIPIgnoresForwardedHeadersFromUntrustedPeer(t *testing.T) {
	r := &http.Request{Header: http.Header{}, RemoteAddr: "203.0.113.7:5555"}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")

	if got := ClientIP(r, nil); got != "203.0.113.7" {
		t.Fatalf("untrusted peer: expected direct peer IP, got %q", got)
	}
}

func TestClientIPHonorsForwardedHeaderFromTrustedProxy(t *testing.T) {
	r := &http.Request{Header: http.Header{}, RemoteAddr: "10.0.0.1:5555"}
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")

	got := ClientIP(r, mustPrefixes(t, "10.0.0.0/8"))
	if got != "1.2.3.4" {
		t.Fatalf("trusted proxy: expected forwarded client IP, got %q", got)
	}
}

func TestClientIPFallsBackToPeerWithoutHeaders(t *testing.T) {
	r := &http.Request{Header: http.Header{}, RemoteAddr: "192.0.2.9:443"}
	if got := ClientIP(r, mustPrefixes(t, "192.0.2.0/24")); got != "192.0.2.9" {
		t.Fatalf("expected peer fallback, got %q", got)
	}
}
