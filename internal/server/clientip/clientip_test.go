package clientip

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestOf(t *testing.T) {
	r := New([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12")})
	tests := []struct {
		name   string
		remote string
		xff    string
		want   string
	}{
		{"direct client ignores XFF", "203.0.113.7:5000", "1.2.3.4", "203.0.113.7"},
		{"behind caddy", "127.0.0.1:5000", "198.51.100.9", "198.51.100.9"},
		{"docker bridge then caddy", "172.17.0.1:5000", "198.51.100.9, 127.0.0.1", "198.51.100.9"},
		{"spoofed left entry ignored", "127.0.0.1:5000", "6.6.6.6, 198.51.100.9", "198.51.100.9"},
		{"no header falls back to peer", "127.0.0.1:5000", "", "127.0.0.1"},
		{"garbage hop stops walk", "127.0.0.1:5000", "nonsense", "127.0.0.1"},
		{"ipv4-mapped peer", "[::ffff:203.0.113.7]:80", "", "203.0.113.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remote
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := r.Of(req); got != tc.want {
				t.Errorf("Of = %s, want %s", got, tc.want)
			}
		})
	}
}
