package httpx

import (
	"net"
	"net/http"
	"net/netip"
)

// ClientIP returns the caller's address for rate limits and session hints.
// Behind Cloudflare the real address is in CF-Connecting-IP; otherwise it is
// the connection address. Only use it for throttling and hashing, never for
// access decisions: a request that reaches API Gateway directly can forge the
// header (docs/10 §4.8).
func ClientIP(r *http.Request) string {
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		if a, err := netip.ParseAddr(v); err == nil {
			return a.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return a.String()
	}
	return "unknown"
}
