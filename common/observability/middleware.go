package observability

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/zeromicro/go-zero/core/logx"
)

// statusRecorder wraps an http.ResponseWriter to capture the status code that
// was written. It is safe for use by a single goroutine handling one request.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.status = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// RequestIDMiddleware injects an X-Request-ID into every request: if the
// client supplied one it is reused, otherwise a fresh ID is generated. The ID
// is propagated via the request context (so downstream logic can attach it to
// logs / RPC metadata) AND written back to the response header so that the
// client can correlate.
func RequestIDMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if id == "" {
				id = NewRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			r = r.WithContext(WithRequestID(r.Context(), id))
			next(w, r)
		}
	}
}

// LoggingMiddleware logs each request's method, path, status, latency and
// request ID via logx.Infof (structured). It must be chained AFTER
// RequestIDMiddleware so that the request ID is already in the context.
func LoggingMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newStatusRecorder(w)
			next(rec, r)
			latency := time.Since(start)
			reqID := RequestIDFromContext(r.Context())
			logx.Infof("http: method=%s path=%s status=%d latency=%s request_id=%s remote=%s",
				r.Method, r.URL.Path, rec.status, latency, reqID, r.RemoteAddr)
		}
	}
}

// HTTP metrics singleton — registered exactly once with the default
// Prometheus registry via promauto.
var (
	httpMetricsOnce     sync.Once
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
)

func initHTTPMetrics() {
	httpMetricsOnce.Do(func() {
		httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "apipro_http_requests_total",
			Help: "Total number of HTTP requests.",
		}, []string{"method", "path", "status"})
		httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "apipro_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: latencyBuckets,
		}, []string{"method", "path"})
	})
}

// HTTPMetricsMiddleware records request count (labels: method, path, status)
// and latency (labels: method, path) for every HTTP request.
//
// NOTE: path is the raw r.URL.Path with no cardinality control. This is
// acceptable for v1 because the API surface is small and route-bound; Phase 1
// should collapse numeric/:id segments to avoid label explosion on routes
// such as /api/v1/room/:roomNum.
func HTTPMetricsMiddleware() func(http.HandlerFunc) http.HandlerFunc {
	initHTTPMetrics()
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newStatusRecorder(w)
			next(rec, r)
			latency := time.Since(start).Seconds()
			status := strconv.Itoa(rec.status)
			httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(latency)
		}
	}
}
