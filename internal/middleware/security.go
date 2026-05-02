package middleware

import (
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
	host := strings.ToLower(r.Host)
	if host == "" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return originMatchesRequest(u, r)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		u, err := url.Parse(referer)
		if err != nil {
			return false
		}
		return originMatchesRequest(u, r)
	}
	return false
}

func originMatchesRequest(u *url.URL, r *http.Request) bool {
	return u.Scheme != "" &&
		strings.EqualFold(u.Scheme, requestScheme(r)) &&
		strings.EqualFold(u.Host, r.Host)
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
