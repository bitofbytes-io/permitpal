package middleware

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP sets r.RemoteAddr to the client IP. X-Forwarded-For is honored only
// when the direct peer is in trusted; the client is then the rightmost
// forwarded address that is not itself a trusted proxy. With no trusted
// proxies, the TCP peer address is used and forwarded headers are ignored.
func RealIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ip, ok := clientIP(r, trusted); ok {
				r.RemoteAddr = ip.String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the client IP from r.RemoteAddr, with or without a port.
func ClientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func clientIP(r *http.Request, trusted []netip.Prefix) (netip.Addr, bool) {
	peer, err := netip.ParseAddr(ClientIP(r))
	if err != nil {
		return netip.Addr{}, false
	}
	peer = peer.Unmap()
	if !isTrusted(peer, trusted) {
		return peer, true
	}
	var hops []string
	for _, header := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(header, ",")...)
	}
	client := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		client = addr.Unmap()
		if !isTrusted(client, trusted) {
			break
		}
	}
	return client, true
}

func isTrusted(addr netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
