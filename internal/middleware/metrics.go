package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hostus_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hostus_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	RateLimitRejects = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "hostus_rate_limit_rejects_total",
			Help: "Total number of rate-limited requests",
		},
	)

	LoadSheddingActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "hostus_load_shedding_active",
			Help: "Whether load shedding is currently active (1) or not (0)",
		},
	)
)

type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (mrw *metricsResponseWriter) WriteHeader(status int) {
	mrw.status = status
	mrw.ResponseWriter.WriteHeader(status)
}

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		mrw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(mrw, r)

		duration := time.Since(start)

		httpRequestsTotal.WithLabelValues(
			r.Method,
			r.URL.Path,
			strconv.Itoa(mrw.status),
		).Inc()

		httpRequestDuration.WithLabelValues(
			r.Method,
			r.URL.Path,
		).Observe(duration.Seconds())
	})
}
