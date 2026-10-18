// Package metrics exposes Prometheus metrics for PermitPal on a separate
// internal listener, never on the public router. It owns its registry instead
// of using the global default, so only the collectors registered here are
// exported.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Login failure reasons. They match the reason attribute of the
// "login failed" log line.
const (
	LoginInvalidRequest  = "invalid_request"
	LoginRateLimited     = "rate_limited"
	LoginInvalidUsername = "invalid_username"
	LoginInvalidPassword = "invalid_password"
)

var loginFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "permitpal_login_failures_total",
	Help: "Refused password logins, by reason.",
}, []string{"reason"})

func init() {
	// Start every known series at zero so rate() and alerts see it before the
	// first failure.
	for _, reason := range []string{LoginInvalidRequest, LoginRateLimited, LoginInvalidUsername, LoginInvalidPassword} {
		loginFailures.WithLabelValues(reason)
	}
}

// NewRegistry returns a registry holding the Go runtime and process
// collectors, the HTTP RED metrics, the domain counters and any extra
// collectors, such as the database pool collector.
func NewRegistry(extra ...prometheus.Collector) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
		loginFailures,
	)
	reg.MustRegister(extra...)
	return reg
}

// Handler serves GET /metrics from reg. Every other path returns 404.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	return mux
}

// LoginFailure counts a refused login with one of the Login* reasons.
func LoginFailure(reason string) {
	loginFailures.WithLabelValues(reason).Inc()
}
