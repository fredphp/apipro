package cache

// Redis cache layer with TTL + scheduled background refresh.
// Uses go-zero core stores/redis. Every Get is a read-through: cache hit -> return;
// miss -> compute from loader -> write back with TTL. A background goroutine per cache
// family proactively refreshes hot keys on a schedule so the cache stays warm.

import (
        "context"
        "encoding/json"
        "errors"
        "strings"
        "sync"
        "sync/atomic"
        "time"

        "github.com/zeromicro/go-zero/core/logx"
        "github.com/zeromicro/go-zero/core/stores/redis"
)

var ErrCacheUnavailable = errors.New("cache unavailable")

// Stats tracks hit/miss per cache family.
type Stats struct {
        mu     sync.Mutex
        hits   map[string]*int64
        misses map[string]*int64
}

func newStats() *Stats {
        return &Stats{hits: map[string]*int64{}, misses: map[string]*int64{}}
}

func (s *Stats) counter(m map[string]*int64, family string) *int64 {
        s.mu.Lock()
        defer s.mu.Unlock()
        c, ok := m[family]
        if !ok {
                var v int64
                c = &v
                m[family] = c
        }
        return c
}

func (s *Stats) Hit(family string)  { atomic.AddInt64(s.counter(s.hits, family), 1) }
func (s *Stats) Miss(family string) { atomic.AddInt64(s.counter(s.misses, family), 1) }

func (s *Stats) Snapshot() (hits, misses map[string]int64) {
        s.mu.Lock()
        defer s.mu.Unlock()
        hits = map[string]int64{}
        misses = map[string]int64{}
        for k, v := range s.hits {
                hits[k] = atomic.LoadInt64(v)
        }
        for k, v := range s.misses {
                misses[k] = atomic.LoadInt64(v)
        }
        return
}

// Cache is the read-through Redis cache.
type Cache struct {
        rdb   *redis.Redis
        stats *Stats
}

// New creates a cache wrapping a go-zero redis instance.
func New(rdb *redis.Redis) *Cache {
        return &Cache{rdb: rdb, stats: newStats()}
}

// Rdb exposes the underlying redis client (for user store / rate limit reuse).
func (c *Cache) Rdb() *redis.Redis { return c.rdb }

// Stats returns the hit/miss snapshot struct.
func (c *Cache) Stats() *Stats { return c.stats }

// GetOrLoad reads JSON value from Redis; on miss calls loader, stores with TTL, returns.
func GetOrLoad[T any](ctx context.Context, c *Cache, family, key string, ttl time.Duration, loader func() (T, error)) (T, error) {
        var zero T
        if c == nil || c.rdb == nil {
                return loader()
        }
        redisKey := buildKey(family, key)
        raw, err := c.rdb.Get(redisKey)
        if err == nil && raw != "" {
                c.stats.Hit(family)
                var v T
                if jerr := json.Unmarshal([]byte(raw), &v); jerr == nil {
                        return v, nil
                }
        }
        if err != nil && !errors.Is(err, redis.Nil) {
                logx.Errorf("cache get error family=%s key=%s: %v", family, key, err)
        }
        c.stats.Miss(family)
        v, lerr := loader()
        if lerr != nil {
                return zero, lerr
        }
        go func() {
                b, jerr := json.Marshal(v)
                if jerr != nil {
                        return
                }
                if serr := c.rdb.Setex(redisKey, string(b), int(ttl.Seconds())); serr != nil {
                        logx.Errorf("cache set error family=%s key=%s: %v", family, key, serr)
                }
        }()
        return v, nil
}

// Refresh forces a reload (bypass cache read) and writes back.
func Refresh[T any](ctx context.Context, c *Cache, family, key string, ttl time.Duration, loader func() (T, error)) error {
        if c == nil || c.rdb == nil {
                _, err := loader()
                return err
        }
        v, lerr := loader()
        if lerr != nil {
                return lerr
        }
        b, jerr := json.Marshal(v)
        if jerr != nil {
                return jerr
        }
        return c.rdb.Setex(buildKey(family, key), string(b), int(ttl.Seconds()))
}

// Invalidate deletes a key.
func (c *Cache) Invalidate(ctx context.Context, family, key string) error {
        if c == nil || c.rdb == nil {
                return nil
        }
        _, err := c.rdb.Del(buildKey(family, key))
        return err
}

// InvalidateFamily deletes all keys of a family (scan-based).
func (c *Cache) InvalidateFamily(ctx context.Context, family string) (int, error) {
        if c == nil || c.rdb == nil {
                return 0, nil
        }
        keys, err := c.rdb.Keys("apipro:" + family + ":*")
        if err != nil {
                return 0, err
        }
        if len(keys) == 0 {
                return 0, nil
        }
        n, err := c.rdb.Del(keys...)
        if err != nil {
                return 0, err
        }
        return n, nil
}

// CountKeys returns total cache keys matching apipro:*.
func (c *Cache) CountKeys(ctx context.Context) int64 {
        if c == nil || c.rdb == nil {
                return 0
        }
        keys, err := c.rdb.Keys("apipro:*")
        if err != nil {
                return 0
        }
        return int64(len(keys))
}

// CountFamily returns key count for a family.
func (c *Cache) CountFamily(ctx context.Context, family string) int64 {
        if c == nil || c.rdb == nil {
                return 0
        }
        keys, err := c.rdb.Keys("apipro:" + family + ":*")
        if err != nil {
                return 0
        }
        return int64(len(keys))
}

func buildKey(family, key string) string {
        return "apipro:" + family + ":" + strings.TrimSpace(key)
}

// ---------- Scheduler ----------

// RefreshJob is a scheduled cache refresh.
type RefreshJob struct {
        Family string
        Run    func() error
        Every  time.Duration
}

// Scheduler runs refresh jobs on a ticker.
type Scheduler struct {
        jobs []RefreshJob
        stop chan struct{}
}

func NewScheduler() *Scheduler {
        return &Scheduler{stop: make(chan struct{})}
}

func (s *Scheduler) Add(job RefreshJob) {
        if job.Every <= 0 {
                return
        }
        s.jobs = append(s.jobs, job)
}

func (s *Scheduler) Start() {
        for _, j := range s.jobs {
                j := j
                go func() {
                        ticker := time.NewTicker(j.Every)
                        defer ticker.Stop()
                        if err := j.Run(); err != nil {
                                logx.Errorf("refresh warm family=%s: %v", j.Family, err)
                        }
                        for {
                                select {
                                case <-ticker.C:
                                        if err := j.Run(); err != nil {
                                                logx.Errorf("refresh family=%s: %v", j.Family, err)
                                        }
                                case <-s.stop:
                                        return
                                }
                        }
                }()
        }
        logx.Infof("cache scheduler started with %d jobs", len(s.jobs))
}

func (s *Scheduler) Stop() {
        close(s.stop)
}
