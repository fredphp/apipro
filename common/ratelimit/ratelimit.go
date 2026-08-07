package ratelimit

// Sliding-window per-IP rate limiter backed by Redis (sorted set).
// Falls back to allow-all if Redis is unavailable, so a cache outage never blocks reads.

import (
        "context"
        "fmt"
        "net/http"
        "strconv"
        "time"

        "github.com/zeromicro/go-zero/core/stores/redis"
        "github.com/zeromicro/go-zero/rest/httpx"
)

type Limiter struct {
        rdb     *redis.Redis
        perMin  int
        window  time.Duration
        keyPref string
}

func New(rdb *redis.Redis, perMinute int, keyPrefix string) *Limiter {
        if perMinute <= 0 {
                perMinute = 120
        }
        return &Limiter{rdb: rdb, perMin: perMinute, window: time.Minute, keyPref: keyPrefix}
}

// Allow checks whether the identifier may proceed.
func (l *Limiter) Allow(ctx context.Context, id string) bool {
        if l == nil || l.rdb == nil {
                return true
        }
        now := time.Now().UnixNano()
        member := fmt.Sprintf("%d", now)
        key := l.keyPref + ":rl:" + id
        _, _ = l.rdb.Zremrangebyscore(key, 0, now-int64(l.window))
        cnt, err := l.rdb.Zcard(key)
        if err != nil {
                return true
        }
        if cnt >= l.perMin {
                return false
        }
        _, _ = l.rdb.Zadd(key, now, member)
        _ = l.rdb.Expire(key, int(l.window.Seconds())+1)
        return true
}

// HTTPMiddleware returns a go-zero middleware enforcing per-IP limits.
func (l *Limiter) HTTPMiddleware() func(http.HandlerFunc) http.HandlerFunc {
        return func(next http.HandlerFunc) http.HandlerFunc {
                return func(w http.ResponseWriter, r *http.Request) {
                        ip := clientIP(r)
                        if !l.Allow(r.Context(), ip) {
                                w.Header().Set("Retry-After", "60")
                                httpx.ErrorCtx(r.Context(), w, fmt.Errorf("rate limit exceeded for %s", ip))
                                return
                        }
                        next(w, r)
                }
        }
}

func clientIP(r *http.Request) string {
        if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
                for i := 0; i < len(xff); i++ {
                        if xff[i] == ',' {
                                return xff[:i]
                        }
                }
                return xff
        }
        if xri := r.Header.Get("X-Real-IP"); xri != "" {
                return xri
        }
        host := r.RemoteAddr
        for i := len(host) - 1; i >= 0; i-- {
                if host[i] == ':' {
                        return host[:i]
                }
        }
        return host
}

// helper to parse int safely
func atoi(s string) int {
        n, _ := strconv.Atoi(s)
        return n
}
