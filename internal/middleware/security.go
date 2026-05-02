package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStateChangingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "Invalid cross-site request", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func LimitBodyBytes(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sameOrigin(r *http.Request) bool {
	scheme := requestScheme(r)
	host, port, ok := splitHostPort(r.Host, scheme)
	if !ok {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return originMatches(u, scheme, host, port)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		u, err := url.Parse(referer)
		if err != nil {
			return false
		}
		return originMatches(u, scheme, host, port)
	}
	return false
}

func originMatches(u *url.URL, scheme, host, port string) bool {
	return u.Scheme != "" &&
		strings.EqualFold(u.Scheme, scheme) &&
		strings.EqualFold(u.Hostname(), host) &&
		effectivePort(u.Scheme, u.Port()) == port
}

func requestScheme(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		if scheme, _, ok := strings.Cut(proto, ","); ok {
			return strings.TrimSpace(scheme)
		}
		return strings.TrimSpace(proto)
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func splitHostPort(hostport, scheme string) (string, string, bool) {
	if hostport == "" {
		return "", "", false
	}
	host, port, err := net.SplitHostPort(hostport)
	if err == nil {
		return host, port, true
	}
	host = strings.Trim(hostport, "[]")
	if host == "" {
		return "", "", false
	}
	return host, effectivePort(scheme, ""), true
}

func effectivePort(scheme, port string) string {
	if port != "" {
		return port
	}
	if strings.EqualFold(scheme, "https") {
		return "443"
	}
	return "80"
}
