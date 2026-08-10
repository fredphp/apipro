package cache

// Fallback chain manager for the high-concurrency protection layer.
//
// Background (audit-1B DZ-4 + audit-1C decision #4):
//   When Redis is degraded, cached endpoints must NOT silently fall through to
//   the DB (which would cause a stampede). Instead, each consistency level has
//   its own fail-over chain:
//     Level 1 (display)        → fail-OPEN + stale fallback → L1 → OSS → CDN
//     Level 2 (auth/per-res)   → fail-OPEN + L1 fallback
//     Level 3 (write)          → fail-CLOSED → 503
//   This file implements ONLY the chain mechanism + 3 built-in sources. The
//   CacheManager (Phase 0-2) wires the appropriate chain per Level.
//
// Design:
//   - FallbackManager iterates sources in priority order; first hit wins.
//   - Real errors from a source are logged + counted, but do NOT abort the
//     chain — we keep trying the next source. Only "all sources missed" yields
//     ErrNoFallback.
//   - Per-source hit/miss/error counters use *int64 + atomic, populated once
//     at construction time (no map growth at runtime → no mutex on maps).
//   - L1StaleSource reads from in-process freecache. freecache v1.2.7's
//     GetWithExpiration still returns ErrNotFound for already-expired entries
//     (TTL is hard, not soft); so "stale" here means "still within L1 capacity
//     but possibly near eviction" — i.e. any L1 hit is treated as good-enough.
//     If the L1 is empty or evicted, the chain falls through to OSS/CDN.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coocood/freecache"
	"github.com/zeromicro/go-zero/core/logx"
)

// ErrNoFallback is returned by FallbackManager.GetOrError when every source in
// the chain was tried and none produced a hit.
var ErrNoFallback = errors.New("cache: all fallback sources exhausted")

// FallbackSource is a single fallback data source in the degradation chain.
type FallbackSource interface {
	// Name returns the source identifier (used for metrics + logs).
	// Examples: "l1_stale", "oss", "cdn".
	Name() string
	// Get attempts to fetch the cached value for (family, key).
	// Returns (data, hit, err):
	//   hit=false, err=nil    → miss (not an error)
	//   hit=true,  err=nil    → hit
	//   hit=false, err!=nil   → real error (e.g. OSS 500, network failure)
	Get(ctx context.Context, family, key string) (data []byte, hit bool, err error)
}

// FallbackManager owns the ordered list of fallback sources and per-source
// counters. It is safe for concurrent use.
type FallbackManager struct {
	sources []FallbackSource
	// Pre-populated per-source counters (keyed by source.Name()). Maps are
	// read-only after NewFallbackManager returns, so no mutex is needed —
	// only the *int64 values are mutated, via atomic ops.
	hits   map[string]*int64
	misses map[string]*int64
	errors map[string]*int64
}

// NewFallbackManager builds a manager that tries sources in priority order.
// Counters are initialized up-front for every source.
func NewFallbackManager(sources ...FallbackSource) *FallbackManager {
	fm := &FallbackManager{
		sources: sources,
		hits:    make(map[string]*int64, len(sources)),
		misses:  make(map[string]*int64, len(sources)),
		errors:  make(map[string]*int64, len(sources)),
	}
	for _, src := range sources {
		if src == nil {
			continue
		}
		name := src.Name()
		if _, ok := fm.hits[name]; !ok {
			var h, m, e int64
			fm.hits[name] = &h
			fm.misses[name] = &m
			fm.errors[name] = &e
		}
	}
	return fm
}

// GetOrError walks the chain. The first source that returns hit=true wins.
// Real errors are logged + counted but do not stop the chain — we keep
// trying. If every source misses (or errors), returns ErrNoFallback.
func (fm *FallbackManager) GetOrError(ctx context.Context, family, key string) (data []byte, source string, err error) {
	for _, src := range fm.sources {
		if src == nil {
			continue
		}
		name := src.Name()
		d, hit, e := src.Get(ctx, family, key)
		if e != nil {
			// Real error → count + log, then try the next source.
			if c := fm.errors[name]; c != nil {
				atomic.AddInt64(c, 1)
			}
			logx.Errorf("fallback source %s error family=%s key=%s: %v", name, family, key, e)
			continue
		}
		if hit {
			if c := fm.hits[name]; c != nil {
				atomic.AddInt64(c, 1)
			}
			return d, name, nil
		}
		// Clean miss.
		if c := fm.misses[name]; c != nil {
			atomic.AddInt64(c, 1)
		}
	}
	return nil, "", ErrNoFallback
}

// Sources returns the names of every source in chain order (for diagnostics).
func (fm *FallbackManager) Sources() []string {
	out := make([]string, 0, len(fm.sources))
	for _, src := range fm.sources {
		if src == nil {
			continue
		}
		out = append(out, src.Name())
	}
	return out
}

// Stats returns a snapshot of per-source (hit, miss, error) counts.
// The map key is source.Name(); the [3]int64 is {hits, misses, errors}.
func (fm *FallbackManager) Stats() map[string][3]int64 {
	out := make(map[string][3]int64, len(fm.sources))
	for _, src := range fm.sources {
		if src == nil {
			continue
		}
		name := src.Name()
		var entry [3]int64
		if c := fm.hits[name]; c != nil {
			entry[0] = atomic.LoadInt64(c)
		}
		if c := fm.misses[name]; c != nil {
			entry[1] = atomic.LoadInt64(c)
		}
		if c := fm.errors[name]; c != nil {
			entry[2] = atomic.LoadInt64(c)
		}
		out[name] = entry
	}
	return out
}

// ---------------------------------------------------------------------------
// Built-in source: L1StaleSource (in-process freecache).
// ---------------------------------------------------------------------------

// L1StaleSource reads from an in-process freecache L1. Any L1 hit is returned
// (we do NOT enforce TTL here — the L1 is configured by the CacheManager with
// a TTL longer than L2 Redis, so an L1 hit is by definition "fresher than
// nothing"). If the L1 is nil or the key is absent/expired/evicted, returns
// a clean miss.
type L1StaleSource struct {
	l1 *freecache.Cache
}

// NewL1StaleSource wraps a freecache L1 instance as a FallbackSource.
func NewL1StaleSource(l1 *freecache.Cache) *L1StaleSource {
	return &L1StaleSource{l1: l1}
}

// Name implements FallbackSource.
func (s *L1StaleSource) Name() string { return "l1_stale" }

// Get implements FallbackSource. Key format mirrors the L2 Redis cache:
// "apipro:<family>:<key>".
func (s *L1StaleSource) Get(ctx context.Context, family, key string) ([]byte, bool, error) {
	_ = ctx // freecache is in-process; no context plumbing needed.
	if s.l1 == nil {
		return nil, false, nil
	}
	fullKey := []byte("apipro:" + family + ":" + key)
	data, _, err := s.l1.GetWithExpiration(fullKey)
	if err == freecache.ErrNotFound || len(data) == 0 {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// ---------------------------------------------------------------------------
// Built-in source: OSSSnapshotSource (HTTP GET to an OSS snapshot bucket).
// ---------------------------------------------------------------------------

// OSSSnapshotSource fetches a JSON snapshot from an OSS-style HTTP endpoint.
// URL pattern: <baseURL>/<family>/<key-with-colons-as-slashes>.json
//   - 200 → hit, body returned
//   - 404 → clean miss (snapshot doesn't exist for this key)
//   - other non-200 / network error → real error (chain continues)
type OSSSnapshotSource struct {
	baseURL string
	client  *http.Client
}

// NewOSSSnapshotSource builds an OSS source with a 2s-timeout HTTP client.
// baseURL should NOT have a trailing slash (e.g.
// "https://oss.example.com/apipro-snapshots").
func NewOSSSnapshotSource(baseURL string) *OSSSnapshotSource {
	return &OSSSnapshotSource{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// Name implements FallbackSource.
func (s *OSSSnapshotSource) Name() string { return "oss" }

// Get implements FallbackSource.
func (s *OSSSnapshotSource) Get(ctx context.Context, family, key string) ([]byte, bool, error) {
	if s.baseURL == "" {
		return nil, false, nil
	}
	// Replace ":" with "/" so keys like "detail:12345" become path-layered
	// "detail/12345" — friendlier for OSS bucket layout.
	safeKey := strings.ReplaceAll(key, ":", "/")
	url := fmt.Sprintf("%s/%s/%s.json", s.baseURL, family, safeKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, false, err
		}
		if len(body) == 0 {
			// Treat empty body as a miss to avoid poisoning the chain with
			// a zero-length "hit".
			return nil, false, nil
		}
		return body, true, nil
	case resp.StatusCode == http.StatusNotFound:
		// Snapshot not present for this key — clean miss, not an error.
		return nil, false, nil
	default:
		return nil, false, fmt.Errorf("oss snapshot %s: unexpected status %d", url, resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Built-in source: CDNSource (terminal fallback — emit a CDN redirect hint).
// ---------------------------------------------------------------------------

// CDNSource is the terminal fallback. It does NOT download from CDN — instead
// it returns a small JSON document telling the caller (or the gateway) to
// fetch directly from the CDN. In production, CDN redirect is usually handled
// by nginx/edge; this source is an interface placeholder so the chain always
// has a non-failing last resort.
type CDNSource struct {
	baseURL string
}

// NewCDNSource builds a CDN source. baseURL should NOT have a trailing slash.
func NewCDNSource(baseURL string) *CDNSource {
	return &CDNSource{baseURL: strings.TrimRight(baseURL, "/")}
}

// Name implements FallbackSource.
func (s *CDNSource) Name() string { return "cdn" }

// Get implements FallbackSource. Always returns hit=true with a JSON redirect
// document; never returns an error.
func (s *CDNSource) Get(ctx context.Context, family, key string) ([]byte, bool, error) {
	_ = ctx
	safeKey := strings.ReplaceAll(key, ":", "/")
	redirect := fmt.Sprintf("%s/%s/%s.json", s.baseURL, family, safeKey)
	payload := fmt.Sprintf(`{"cdn_redirect":"%s"}`, redirect)
	return []byte(payload), true, nil
}
