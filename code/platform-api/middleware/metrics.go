package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics records RED metrics per route pattern. Using r.Pattern (Go 1.23+)
// instead of the raw path keeps label cardinality bounded.
//
// The mux sets r.Pattern on the *Request it receives, so Metrics must be the
// innermost middleware (directly wrapping the mux) to observe it.
func Metrics(reg prometheus.Registerer) Middleware {
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "platform_api_http_requests_total",
		Help: "HTTP requests by route, method and status code.",
	}, []string{"route", "method", "code"})
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "platform_api_http_request_duration_seconds",
		Help:    "HTTP request latency by route and method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})
	reg.MustRegister(requests, duration)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			duration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}
