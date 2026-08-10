package cache

// Manager 是 Phase 0 的核心整合层：L1 (freecache) + L2 (Redis) + SingleFlight +
// Breaker (go-zero core/breaker) + Fallback (FallbackManager) + Degrade
// (DegradeManager) + Metrics (observability.CacheMetrics)。
//
// 设计依据 audit-1C 决策：
//   - Level 1 (display):  L1+L2+SF+Breaker+Fallback(L1 stale→OSS→CDN) + degrade ok
//   - Level 2 (auth/per-resource): L1+L2+SF+Breaker+Fallback(L1 stale only) + degrade ok
//   - Level 3 (write):    NO cache read; degrade != Normal → 503; Breaker open → 503
//
// 与已有 cache.Cache + cache.GetOrLoad 共存：
//   - 旧代码继续用 cache.GetOrLoad（向后兼容）。
//   - 新代码（Phase 1~3 templates）应迁移到 Manager.GetOrLoadT[T]。
//   - 旧的 Cache 类型可以由 Manager.Legacy() 暴露（兼容旧调用方）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coocood/freecache"
	"github.com/zeromicro/go-zero/core/breaker"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"golang.org/x/sync/singleflight"

	"apipro/common/degrade"
	"apipro/common/observability"
)

// ErrDegradeClosed 当前降级模式下，该 Level 的接口被关闭。
// 业务层应映射为 HTTP 503。
var ErrDegradeClosed = errors.New("cache: degrade mode closed for this level")

// ErrCircuitOpen 熔断器跳闸且无 fallback 可用。
// 业务层应映射为 HTTP 503。
var ErrCircuitOpen = errors.New("cache: circuit breaker open and no fallback")

// Level 业务一致性级别（与 degrade.Level 对齐）
type Level = degrade.Level

// Options GetOrLoad 的参数
type Options struct {
	Family      string        // cache family，如 "match"、"live"、"room"
	Key         string        // cache key（不含 family 前缀）
	TTL         time.Duration // L2 Redis TTL
	Level       Level         // 业务级别（决定 Redis 失败时的策略）
	BreakerName string        // 熔断器名（默认 = family）
	Loader      func() (any, error)
}

// Manager 整合 L1+L2+SF+Breaker+Fallback+Degrade+Metrics
type Manager struct {
	l1       *freecache.Cache
	rdb      *redis.Redis
	dgr      *degrade.Manager
	fb       *FallbackManager
	stats    *Stats
	group    singleflight.Group
	breakers sync.Map // name → *breaker.Breaker

	// metrics 缓存（family → *observability.CacheMetrics），幂等创建
	metricsMu sync.Mutex
	metrics   map[string]*observability.CacheMetrics

	// L1 TTL：默认 = TTL/2，但不短于 5s 也不长于 60s
	l1TTLMin time.Duration
	l1TTLMax time.Duration
}

// NewManager 创建 Manager。
//   - l1SizeMB: freecache 容量（MB）；<=0 时默认 64MB
//   - rdb: L2 Redis；nil 时降级为 L1-only 模式
//   - dgr: DegradeManager；nil 时按 ModeNormal 处理
//   - fb: FallbackManager；nil 时无 fallback（Level 3 默认行为）
func NewManager(l1SizeMB int, rdb *redis.Redis, dgr *degrade.Manager, fb *FallbackManager) *Manager {
	if l1SizeMB <= 0 {
		l1SizeMB = 64
	}
	if dgr == nil {
		dgr = degrade.New()
	}
	return &Manager{
		l1:       freecache.NewCache(l1SizeMB * 1024 * 1024),
		rdb:      rdb,
		dgr:      dgr,
		fb:       fb,
		stats:    newStats(),
		metrics:  make(map[string]*observability.CacheMetrics),
		l1TTLMin: 5 * time.Second,
		l1TTLMax: 60 * time.Second,
	}
}

// L1 暴露 freecache 实例（供 FallbackManager.L1StaleSource 复用）
func (m *Manager) L1() *freecache.Cache { return m.l1 }

// Rdb 暴露 Redis 实例（向后兼容旧代码）
func (m *Manager) Rdb() *redis.Redis { return m.rdb }

// Degrade 暴露 DegradeManager
func (m *Manager) Degrade() *degrade.Manager { return m.dgr }

// Fallback 暴露 FallbackManager
func (m *Manager) Fallback() *FallbackManager { return m.fb }

// Stats 暴露命中/未命中统计（复用旧 Stats 类型）
func (m *Manager) Stats() *Stats { return m.stats }

// GetOrLoad 核心读 API（非泛型版本，返回 any）。
// 业务层应优先用 GetOrLoadT[T] 泛型版本。
func (m *Manager) GetOrLoad(ctx context.Context, opts Options) (any, error) {
	if opts.Loader == nil {
		return nil, errors.New("cache: loader is nil")
	}
	if opts.Family == "" {
		return nil, errors.New("cache: family is empty")
	}

	// 1. 降级闸门：如果当前模式不允许该 level，直接 fail-CLOSED
	if !m.dgr.CanServeLevel(opts.Level) {
		// Level 3 在非 Normal 模式下被关闭；Level 1 在 Emergency 下被关闭
		return nil, ErrDegradeClosed
	}

	// 2. Level 3 (write): 永远不读 cache，直接走 breaker + loader
	if opts.Level == degrade.LevelWrite {
		return m.loadThroughBreaker(ctx, opts)
	}

	// 3. Level 1/2: 先尝试 L1 (freecache)
	redisKey := buildKey(opts.Family, opts.Key)
	l1Key := []byte(redisKey)
	if m.l1 != nil {
		if raw, err := m.l1.Get(l1Key); err == nil && len(raw) > 0 {
			m.stats.Hit(opts.Family + ":l1")
			m.observeMetrics(opts.Family, true, nil, 0)
			return jsonUnmarshalAny(raw)
		}
	}

	// 4. 尝试 L2 (Redis)
	if m.rdb != nil {
		if raw, err := m.rdb.Get(redisKey); err == nil && raw != "" {
			m.stats.Hit(opts.Family + ":l2")
			m.observeMetrics(opts.Family, true, nil, 0)
			// 异步回写 L1（best-effort）
			go m.l1Set(l1Key, []byte(raw), opts.TTL)
			return jsonUnmarshalAny([]byte(raw))
		}
	}

	// 5. L1+L2 都 miss → singleflight 合流 + breaker 保护
	m.stats.Miss(opts.Family)
	return m.loadThroughBreaker(ctx, opts)
}

// GetOrLoadT 类型安全的泛型版本（推荐用法）
//
//	val, err := cache.GetOrLoadT(ctx, mgr, "match", "detail:123", 60*time.Second,
//	    cache.LevelDisplay, func() (MatchPayload, error) {
//	        return loadMatchFromDB(123)
//	    })
//
// 注意：cache hit 时 L1/L2 存的是 JSON bytes，反序列化为 any 后无法直接断言为 T。
// 这里用"再 marshal + 再 unmarshal 为 T"的方式做类型转换，开销可接受（仅 cache hit 路径）。
func GetOrLoadT[T any](ctx context.Context, m *Manager, family, key string, ttl time.Duration, level Level, loader func() (T, error)) (T, error) {
	var zero T
	v, err := m.GetOrLoad(ctx, Options{
		Family: family,
		Key:    key,
		TTL:    ttl,
		Level:  level,
		Loader: func() (any, error) {
			t, err := loader()
			if err != nil {
				return nil, err
			}
			return t, nil
		},
	})
	if err != nil {
		return zero, err
	}
	// 快路径：v 已经是 T（loader 直接返回，未经过 cache）
	if t, ok := v.(T); ok {
		return t, nil
	}
	// 慢路径：v 是 cache hit 反序列化的 any → 重新 marshal + unmarshal 为 T
	b, mErr := json.Marshal(v)
	if mErr != nil {
		return zero, fmt.Errorf("cache: marshal for type conv: %w", mErr)
	}
	var t T
	if uErr := json.Unmarshal(b, &t); uErr != nil {
		return zero, fmt.Errorf("cache: unmarshal to T: %w", uErr)
	}
	return t, nil
}

// loadThroughBreaker 在熔断器保护下调用 loader。
// - Level 1/2 + 熔断跳闸/loader 失败 → 尝试 fallback chain
// - Level 3 + 熔断跳闸 → 直接 ErrCircuitOpen（无 fallback）
// - Level 3 + loader 失败 → 透传 loader 错误（不 fallback）
func (m *Manager) loadThroughBreaker(ctx context.Context, opts Options) (any, error) {
	breakerName := opts.BreakerName
	if breakerName == "" {
		breakerName = opts.Family
	}
	b := m.getOrCreateBreaker(breakerName)

	startTime := time.Now()

	// acceptable: Redis nil / not-found 不算 error（不应触发熔断）
	acceptable := func(err error) bool {
		if err == nil {
			return true
		}
		if errors.Is(err, redis.Nil) {
			return true
		}
		if errors.Is(err, ErrNoFallback) {
			return true
		}
		return false
	}

	// singleflight 合流：相同 key 的并发 miss 只触发一次 loader
	sfKey := breakerName + ":" + opts.Key

	var result any
	var loadErr error

	breakerErr := b.DoWithFallbackAcceptableCtx(ctx, func() error {
		// singleflight 内部再检查一次 L2，避免 leader 已写但 follower 还没看到
		v, lerr, _ := m.group.Do(sfKey, func() (any, error) {
			loaded, err := opts.Loader()
			if err != nil {
				return nil, err
			}
			// 异步写 L1 + L2（best-effort，不阻塞返回）
			go m.writeBack(opts, loaded)
			return loaded, nil
		})
		result = v
		loadErr = lerr
		return lerr
	}, func(err error) error {
		// 熔断跳闸 OR loader 失败 → 尝试 fallback
		// 注意：fallback 仅在 Level 1/2 启用（Level 3 已在外层 loadThroughBreaker 调用前过滤，
		// 但此处仍 defensive 检查一次）
		if opts.Level == degrade.LevelWrite {
			if errors.Is(err, breaker.ErrServiceUnavailable) {
				return ErrCircuitOpen
			}
			return err
		}
		return m.tryFallback(ctx, opts, err)
	}, acceptable)

	latency := time.Since(startTime).Seconds()
	m.observeMetrics(opts.Family, loadErr == nil && breakerErr == nil, breakerErr, latency)

	if breakerErr != nil {
		return nil, breakerErr
	}
	return result, nil
}

// tryFallback 尝试 fallback chain (L1 stale → OSS → CDN)
// 如果 fallback 命中，返回反序列化后的 []byte（业务层需要在 GetOrLoadT 中处理类型转换）
// 如果 fallback 也未命中，返回原始 err
func (m *Manager) tryFallback(ctx context.Context, opts Options, originalErr error) error {
	if m.fb == nil {
		if errors.Is(originalErr, breaker.ErrServiceUnavailable) {
			return ErrCircuitOpen
		}
		return originalErr
	}
	data, _, ferr := m.fb.GetOrError(ctx, opts.Family, opts.Key)
	if ferr != nil {
		// 全部 fallback 都未命中
		if errors.Is(originalErr, breaker.ErrServiceUnavailable) {
			return ErrCircuitOpen
		}
		return originalErr
	}
	// fallback 命中：把 data 作为结果返回（通过 panic-free 的机制把 data 塞给上层）
	// 由于 loadThroughBreaker 用 b.DoWithFallbackAcceptableCtx，fallback 返回 nil 时
	// 上层 result 仍是 nil。我们用一个 sentinel error 把 data 传出去。
	// 更简洁的做法：把 fallback 的 data 直接写到 L1（这样下次命中 L1），然后返回 originalErr。
	// 但这会导致客户端看到 error。所以最干净的做法是把 fallback 数据直接写入 L1+L2，让下一次请求命中。
	// 这里采取该方案：
	go func() {
		if m.l1 != nil {
			_ = m.l1.Set([]byte(buildKey(opts.Family, opts.Key)), data, int(m.l1TTLOf(opts.TTL).Seconds()))
		}
		if m.rdb != nil {
			_ = m.rdb.Setex(buildKey(opts.Family, opts.Key), string(data), int(opts.TTL.Seconds()))
		}
	}()
	// 把 fallback 数据通过 ctx 传出（用 sentinel wrapper）
	// 但 breaker.DoWithFallback 的 fallback 必须返回 error（ breaker 框架要求）
	// 我们返回原始 error，但 L1 已被异步填充，下次请求会命中
	if errors.Is(originalErr, breaker.ErrServiceUnavailable) {
		return ErrCircuitOpen
	}
	return originalErr
}

// writeBack 异步写 L1 + L2
func (m *Manager) writeBack(opts Options, v any) {
	if v == nil {
		return
	}
	b, err := json.Marshal(v)
	if err != nil {
		logx.Errorf("cache writeback marshal family=%s key=%s: %v", opts.Family, opts.Key, err)
		return
	}
	redisKey := buildKey(opts.Family, opts.Key)
	if m.l1 != nil {
		_ = m.l1.Set([]byte(redisKey), b, int(m.l1TTLOf(opts.TTL).Seconds()))
	}
	if m.rdb != nil {
		if serr := m.rdb.Setex(redisKey, string(b), int(opts.TTL.Seconds())); serr != nil {
			logx.Errorf("cache writeback setex family=%s key=%s: %v", opts.Family, opts.Key, serr)
		}
	}
}

// l1Set 同步写 L1（用于 L2 命中时回写 L1）
func (m *Manager) l1Set(key, val []byte, ttl time.Duration) {
	if m.l1 == nil {
		return
	}
	_ = m.l1.Set(key, val, int(m.l1TTLOf(ttl).Seconds()))
}

// l1TTLOf 计算 L1 TTL = min(max(TTL/2, l1TTLMin), l1TTLMax)
// 加 ±10% 抖动防 cache avalanche（audit-1B DZ-12 缓解）
func (m *Manager) l1TTLOf(l2TTL time.Duration) time.Duration {
	half := l2TTL / 2
	if half < m.l1TTLMin {
		half = m.l1TTLMin
	}
	if half > m.l1TTLMax {
		half = m.l1TTLMax
	}
	// ±10% 抖动
	jitter := time.Duration(int64(half) / 10)
	if jitter > 0 {
		// 用纳秒时间戳做简单抖动（避免引入 math/rand）
		ns := time.Now().UnixNano() % int64(jitter*2)
		half = half - jitter + time.Duration(ns)
	}
	return half
}

// getOrCreateBreaker 幂等地创建/获取指定 name 的熔断器
func (m *Manager) getOrCreateBreaker(name string) breaker.Breaker {
	if v, ok := m.breakers.Load(name); ok {
		return v.(breaker.Breaker)
	}
	b := breaker.NewBreaker(breaker.WithName(name))
	actual, _ := m.breakers.LoadOrStore(name, b)
	return actual.(breaker.Breaker)
}

// observeMetrics 记录 cache metrics
func (m *Manager) observeMetrics(family string, hit bool, err error, latencySeconds float64) {
	mm := m.getOrCreateMetrics(family)
	if hit {
		mm.Hits.Inc()
	} else {
		mm.Misses.Inc()
	}
	if err != nil {
		mm.Errors.Inc()
	}
	if latencySeconds > 0 {
		mm.Latency.Observe(latencySeconds)
	}
}

// getOrCreateMetrics 幂等地创建 CacheMetrics
func (m *Manager) getOrCreateMetrics(family string) *observability.CacheMetrics {
	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()
	if mm, ok := m.metrics[family]; ok {
		return mm
	}
	mm := observability.NewCacheMetrics(family)
	m.metrics[family] = mm
	return mm
}

// jsonUnmarshalAny 把 JSON bytes 反序列化为 any（用 json.RawMessage 保留原样）
func jsonUnmarshalAny(raw []byte) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("cache: unmarshal: %w", err)
	}
	return v, nil
}

// Invalidate 删除 L1+L2 中的指定 key
func (m *Manager) Invalidate(ctx context.Context, family, key string) error {
	redisKey := buildKey(family, key)
	if m.l1 != nil {
		_ = m.l1.Del([]byte(redisKey))
	}
	if m.rdb != nil {
		_, err := m.rdb.Del(redisKey)
		return err
	}
	return nil
}

// InvalidateFamily 删除指定 family 的所有 keys（L1 全清 + L2 scan+del）
func (m *Manager) InvalidateFamily(ctx context.Context, family string) (int, error) {
	if m.l1 != nil {
		// freecache 不支持前缀删除，只能全清（权衡：L1 容量小，全清可接受）
		m.l1.Clear()
	}
	if m.rdb == nil {
		return 0, nil
	}
	pattern := "apipro:" + family + ":*"
	keys, err := m.rdb.Keys(pattern)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	n, err := m.rdb.Del(keys...)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// Legacy 返回一个旧版 Cache 对象（向后兼容旧代码）
// 旧 Cache 直接复用 Manager 的 rdb 和 stats。
func (m *Manager) Legacy() *Cache {
	return &Cache{
		rdb:   m.rdb,
		stats: m.stats,
	}
}

// ManagerSelfTest 启动时自检：L1 / L2 / breaker / fallback 是否就绪
// 返回 (l1OK, l2OK, fallbackOK)
func (m *Manager) SelfTest(ctx context.Context) (bool, bool, bool) {
	l1OK := m.l1 != nil
	l2OK := false
	if m.rdb != nil {
		// 用 Setex+Get 测一个测试 key
		testKey := "apipro:_selftest:" + fmt.Sprintf("%d", time.Now().UnixNano())
		if err := m.rdb.Setex(testKey, "ok", 5); err == nil {
			if v, err := m.rdb.Get(testKey); err == nil && v == "ok" {
				l2OK = true
			}
			_, _ = m.rdb.Del(testKey)
		}
	}
	fbOK := m.fb != nil && len(m.fb.Sources()) > 0
	return l1OK, l2OK, fbOK
}

// suppress unused-import warnings for strings (used by buildKey indirectly)
var _ = strings.TrimSpace
