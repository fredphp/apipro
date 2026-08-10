package observability

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// latencyBuckets covers 1ms -> ~8s for cache and DB latencies.
// 1ms, 2ms, 4ms, 8ms, 16ms, 32ms, 64ms, 128ms, 256ms, 512ms,
// 1.024s, 2.048s, 4.096s, 8.192s (14 buckets).
var latencyBuckets = prometheus.ExponentialBuckets(0.001, 2, 14)

// ---------------------------------------------------------------------------
// CacheMetrics — one group per cache family (e.g. "match", "live", "room")
// ---------------------------------------------------------------------------

// CacheMetrics groups the Prometheus metrics for a single cache family.
// Every counter uses ConstLabels so that multiple families can be selected
// independently in Prometheus queries.
type CacheMetrics struct {
	Hits    prometheus.Counter   // total cache hits
	Misses  prometheus.Counter   // total cache misses (loader invoked)
	Errors  prometheus.Counter   // total cache loader errors
	Latency prometheus.Histogram // GetOrLoad latency in seconds
}

var (
	cacheMetricsMu    sync.Mutex
	cacheMetricsCache = map[string]*CacheMetrics{}
)

// NewCacheMetrics returns the metrics group for the given family
// (e.g. "match", "live", "room"). Repeated calls with the same family return
// the SAME instance, so it is safe to invoke from multiple constructors.
// Registration is therefore idempotent and never panics on duplicates.
func NewCacheMetrics(family string) *CacheMetrics {
	cacheMetricsMu.Lock()
	defer cacheMetricsMu.Unlock()
	if m, ok := cacheMetricsCache[family]; ok {
		return m
	}
	constLabels := prometheus.Labels{"family": family}
	m := &CacheMetrics{
		Hits: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_cache_hits_total",
			Help:        "Total number of cache hits.",
			ConstLabels: constLabels,
		}),
		Misses: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_cache_misses_total",
			Help:        "Total number of cache misses (loader invoked).",
			ConstLabels: constLabels,
		}),
		Errors: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_cache_errors_total",
			Help:        "Total number of cache loader errors.",
			ConstLabels: constLabels,
		}),
		Latency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "apipro_cache_latency_seconds",
			Help:        "Cache GetOrLoad latency in seconds.",
			Buckets:     latencyBuckets,
			ConstLabels: constLabels,
		}),
	}
	cacheMetricsCache[family] = m
	return m
}

// ---------------------------------------------------------------------------
// DegradeMetrics — singleton, exposes the global degrade_mode gauge
// ---------------------------------------------------------------------------

// DegradeMetrics exposes the DegradeManager's current mode.
//
//	0 = NORMAL
//	1 = DEGRADED
//	2 = PROTECTED
//	3 = EMERGENCY
//
// Required by audit-1C SAFEGUARD 2: "DegradeManager must expose degrade_mode
// Prometheus gauge (0=NORMAL/1=DEGRADED/2=PROTECTED/3=EMERGENCY) per service."
type DegradeMetrics struct {
	Mode prometheus.Gauge
}

// Degrade mode constants — see DegradeMetrics.Mode help text.
const (
	DegradeModeNormal    float64 = 0
	DegradeModeDegraded  float64 = 1
	DegradeModeProtected float64 = 2
	DegradeModeEmergency float64 = 3
)

var (
	degradeMetricsOnce sync.Once
	degradeMetricsInst *DegradeMetrics
)

// NewDegradeMetrics returns the singleton DegradeMetrics instance. Multiple
// calls return the same Gauge so it is safe to invoke from each subsystem
// that needs to read/write the mode.
func NewDegradeMetrics() *DegradeMetrics {
	degradeMetricsOnce.Do(func() {
		degradeMetricsInst = &DegradeMetrics{
			Mode: promauto.NewGauge(prometheus.GaugeOpts{
				Name: "apipro_degrade_mode",
				Help: "Current degrade mode (0=NORMAL, 1=DEGRADED, 2=PROTECTED, 3=EMERGENCY).",
			}),
		}
		// Initialize to NORMAL so the gauge is exported from process start.
		degradeMetricsInst.Mode.Set(DegradeModeNormal)
	})
	return degradeMetricsInst
}

// ---------------------------------------------------------------------------
// RateLimitMetrics — one group per rate-limit dimension
// ---------------------------------------------------------------------------

// RateLimitMetrics groups Prometheus metrics for a single rate-limit dimension.
type RateLimitMetrics struct {
	Denied prometheus.Counter
}

var (
	rateLimitMetricsMu    sync.Mutex
	rateLimitMetricsCache = map[string]*RateLimitMetrics{}
)

// NewRateLimitMetrics returns the metrics group for the given dimension
// (e.g. "ip", "user", "api", "global", "concurrent"). Idempotent: repeated
// calls with the same dim return the SAME instance.
func NewRateLimitMetrics(dim string) *RateLimitMetrics {
	rateLimitMetricsMu.Lock()
	defer rateLimitMetricsMu.Unlock()
	if m, ok := rateLimitMetricsCache[dim]; ok {
		return m
	}
	m := &RateLimitMetrics{
		Denied: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_ratelimit_denied_total",
			Help:        "Total number of requests denied by the rate limiter.",
			ConstLabels: prometheus.Labels{"dim": dim},
		}),
	}
	rateLimitMetricsCache[dim] = m
	return m
}

// ---------------------------------------------------------------------------
// DBMetrics — one group per DB instance
// ---------------------------------------------------------------------------

// DBMetrics groups Prometheus metrics for a single DB instance. The semaphore
// gauges (SemWaiting / SemAcquired) allow operators to see when the DB pool
// semaphore is saturated before requests start to time out.
type DBMetrics struct {
	Queries     prometheus.Counter
	Errors      prometheus.Counter
	Latency     prometheus.Histogram
	SemWaiting  prometheus.Gauge // goroutines currently waiting on the semaphore
	SemAcquired prometheus.Gauge // currently held semaphore permits
}

var (
	dbMetricsMu    sync.Mutex
	dbMetricsCache = map[string]*DBMetrics{}
)

// NewDBMetrics returns the metrics group for the given DB instance name
// (e.g. "mysql-main", "sqlite-eim"). Idempotent.
func NewDBMetrics(name string) *DBMetrics {
	dbMetricsMu.Lock()
	defer dbMetricsMu.Unlock()
	if m, ok := dbMetricsCache[name]; ok {
		return m
	}
	constLabels := prometheus.Labels{"name": name}
	m := &DBMetrics{
		Queries: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_db_queries_total",
			Help:        "Total number of DB queries executed.",
			ConstLabels: constLabels,
		}),
		Errors: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "apipro_db_errors_total",
			Help:        "Total number of DB query errors.",
			ConstLabels: constLabels,
		}),
		Latency: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "apipro_db_latency_seconds",
			Help:        "DB query latency in seconds.",
			Buckets:     latencyBuckets,
			ConstLabels: constLabels,
		}),
		SemWaiting: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "apipro_db_sem_waiting",
			Help:        "Number of goroutines currently waiting to acquire the DB semaphore.",
			ConstLabels: constLabels,
		}),
		SemAcquired: promauto.NewGauge(prometheus.GaugeOpts{
			Name:        "apipro_db_sem_acquired",
			Help:        "Number of DB semaphore permits currently held.",
			ConstLabels: constLabels,
		}),
	}
	dbMetricsCache[name] = m
	return m
}

// ---------------------------------------------------------------------------
// FallbackMetrics — singleton, fallback source hits/misses (L1/OSS/CDN)
// ---------------------------------------------------------------------------

// FallbackMetrics tracks hits/misses across fallback sources used by the
// cache layer when the primary cache misses (e.g. L1 freecache, OSS snapshot,
// CDN snapshot).
//
// Hits is a CounterVec keyed by "source" so that operators can break down
// which fallback layer is doing the work; Misses is a single Counter because
// a miss is always "no fallback was available".
type FallbackMetrics struct {
	Hits   *prometheus.CounterVec // labels: source="l1"|"oss"|"cdn"
	Misses prometheus.Counter
}

var (
	fallbackMetricsOnce sync.Once
	fallbackMetricsInst *FallbackMetrics
)

// NewFallbackMetrics returns the singleton FallbackMetrics instance.
func NewFallbackMetrics() *FallbackMetrics {
	fallbackMetricsOnce.Do(func() {
		fallbackMetricsInst = &FallbackMetrics{
			Hits: promauto.NewCounterVec(prometheus.CounterOpts{
				Name: "apipro_fallback_hits_total",
				Help: "Total number of fallback source hits, labeled by source (l1/oss/cdn).",
			}, []string{"source"}),
			Misses: promauto.NewCounter(prometheus.CounterOpts{
				Name: "apipro_fallback_misses_total",
				Help: "Total number of fallback source misses (no fallback was available).",
			}),
		}
	})
	return fallbackMetricsInst
}
