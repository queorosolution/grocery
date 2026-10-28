package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, header, remote, want string
	}{
		{"cloudflare header", "203.0.113.9", "10.0.0.1:443", "203.0.113.9"},
		{"bad header falls back", "not-an-ip", "10.0.0.1:443", "10.0.0.1"},
		{"remote only", "", "198.51.100.7:5555", "198.51.100.7"},
		{"ipv6", "", "[2001:db8::1]:80", "2001:db8::1"},
		{"garbage", "", "???", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remote
			if tt.header != "" {
				r.Header.Set("CF-Connecting-IP", tt.header)
			}
			if got := ClientIP(r); got != tt.want {
				t.Fatalf("ClientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
