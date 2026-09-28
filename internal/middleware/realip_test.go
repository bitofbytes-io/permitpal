package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestRealIP(t *testing.T) {
	proxies := []netip.Prefix{netip.MustParsePrefix("10.0.1.0/24")}
	tests := []struct {
		name       string
		trusted    []netip.Prefix
		remoteAddr string
		forwarded  []string
		want       string
	}{
		{"no trusted proxies ignores forwarded header", nil, "203.0.113.7:5555", []string{"198.51.100.1"}, "203.0.113.7"},
		{"untrusted peer cannot spoof", proxies, "203.0.113.7:5555", []string{"198.51.100.1"}, "203.0.113.7"},
		{"trusted proxy forwards client", proxies, "10.0.1.5:5555", []string{"198.51.100.1"}, "198.51.100.1"},
		{"client-supplied prefix is ignored", proxies, "10.0.1.5:5555", []string{"192.0.2.99, 198.51.100.1"}, "198.51.100.1"},
		{"multiple headers and trusted hops", proxies, "10.0.1.5:5555", []string{"192.0.2.99", "198.51.100.1, 10.0.1.9"}, "198.51.100.1"},
		{"trusted proxy without header", proxies, "10.0.1.5:5555", nil, "10.0.1.5"},
		{"malformed hop stops the walk", proxies, "10.0.1.5:5555", []string{"198.51.100.1, not-an-ip"}, "10.0.1.5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			handler := RealIP(tt.trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = ClientIP(r)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for _, value := range tt.forwarded {
				req.Header.Add("X-Forwarded-For", value)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Fatalf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}
