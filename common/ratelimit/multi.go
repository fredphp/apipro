package ratelimit

// Multi-dimensional rate limiter backed by Redis.
//
// Provides 6 dimensions (IP / User / Device / API / Global / Concurrent) that
// the upper layer (CacheManager / business middleware) composes into a Rule
// list per request via HTTPMiddleware(ruleGen).
//
// Design (audit-1C):
//   - DimConcurrent uses INCR + EXPIRE (TTL=300s on first INCR) to bound
//     in-flight requests; Release DECRs, clamping to 0 to avoid negative drift.
//   - All other dimensions reuse the same ZREMRANGEBYSCORE + ZCARD + ZADD
//     sliding-window pattern as Limiter.Allow in ratelimit.go.
//   - Redis errors are fail-OPEN (logx.Errorf + return pass): the limiter is a
//     protection layer, a Redis blip must NOT take down all traffic.
//   - Acquire is check-all-then-acquire-all with best-effort rollback on
//     mid-loop failure (acceptable because fail-OPEN).
//
// Key format:  apipro:rl:<dimension>:<key>
//   e.g. apipro:rl:ip:1.2.3.4
//        apipro:rl:api:/login/login
//        apipro:rl:concurrent:/login/login

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// Dimension 限流维度
type Dimension string

const (
	DimIP         Dimension = "ip"         // 客户端 IP
	DimUser       Dimension = "user"       // 用户 UID
	DimDevice     Dimension = "device"     // 设备 ID
	DimAPI        Dimension = "api"        // API 路径（如 "/login/login"）
	DimGlobal     Dimension = "global"     // 全局（所有请求共享一个 bucket）
	DimConcurrent Dimension = "concurrent" // 并发数（用 INCR + EXPIRE 实现）
)

// Level 接口的业务级别（用于和 degrade.Manager 配合）
type Level int

const (
	LevelDisplay Level = 1
	LevelAuth    Level = 2
	LevelWrite   Level = 3
)

// Rule 单个限流规则
type Rule struct {
	Dimension     Dimension
	Key           string // 如 "ip:1.2.3.4" / "api:/login/login" / "user:12345" / "global" / "device:abc"
	PerMinute     int    // 每分钟允许的请求数（0 = 不限）。对 DimConcurrent 无意义。
	MaxConcurrent int    // 最大并发（仅 DimConcurrent 有意义，0 = 不限）。对其他维度无意义。
}

const (
	// multiKeyPrefix 多维限流器在 Redis 中的全局 key 前缀
	multiKeyPrefix = "apipro:rl"
	// concurrentTTL 并发计数器 TTL（秒），防止因 Release 失败/遗漏导致长期泄漏
	concurrentTTL = 300
	// slidingWindowTTL 滑动窗口 ZSET 的 TTL（秒）
	slidingWindowTTL = 61
)

// MultiLimiter 多维限流器。底层复用 Redis sliding-window + INCR/DECR。
type MultiLimiter struct {
	rdb *redis.Redis
}

// NewMulti 创建多维限流器
func NewMulti(rdb *redis.Redis) *MultiLimiter {
	return &MultiLimiter{rdb: rdb}
}

// redisKey 构造 Redis key：apipro:rl:<dimension>:<key>
func redisKey(dim Dimension, key string) string {
	return fmt.Sprintf("%s:%s:%s", multiKeyPrefix, dim, key)
}

// active 判断 rule 是否需要参与限流（任一阈值 > 0 即生效）
func (r *Rule) active() bool {
	if r.Dimension == DimConcurrent {
		return r.MaxConcurrent > 0
	}
	return r.PerMinute > 0
}

// Check 检查一组 rules 是否全部可通过（不扣减）。
// 返回首个被拒的 rule（如有），nil 表示全部通过。
// 仅对 PerMinute / MaxConcurrent > 0 的 rules 检查。
func (ml *MultiLimiter) Check(ctx context.Context, rules []Rule) (denied *Rule) {
	if ml == nil || ml.rdb == nil {
		return nil
	}
	for i := range rules {
		r := &rules[i]
		if !r.active() {
			continue
		}
		ok, err := ml.checkOne(ctx, r)
		if err != nil {
			// Redis 出错时 fail-OPEN（与已有 Limiter 一致；audit-1C 决策）
			logx.Errorf("[ratelimit.MultiLimiter.Check] redis err=%v dim=%s key=%s; fail-OPEN",
				err, r.Dimension, r.Key)
			return nil
		}
		if !ok {
			return r
		}
	}
	return nil
}

// checkOne 检查单个 rule（不扣减）。
func (ml *MultiLimiter) checkOne(ctx context.Context, r *Rule) (bool, error) {
	key := redisKey(r.Dimension, r.Key)
	if r.Dimension == DimConcurrent {
		// GetCtx 在 key 不存在时返回 ("", nil)（已处理 redis.Nil）
		v, err := ml.rdb.GetCtx(ctx, key)
		if err != nil {
			return false, err
		}
		if v == "" {
			return true, nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			// 计数器被污染（非数字），fail-OPEN
			return true, nil
		}
		return n < int64(r.MaxConcurrent), nil
	}

	// sliding window：清理过期 + 统计当前窗口内请求数
	now := time.Now().UnixNano()
	if _, err := ml.rdb.ZremrangebyscoreCtx(ctx, key, 0, now-int64(time.Minute)); err != nil {
		return false, err
	}
	cnt, err := ml.rdb.ZcardCtx(ctx, key)
	if err != nil {
		return false, err
	}
	return cnt < r.PerMinute, nil
}

// Acquire 检查并扣减（原子）。返回是否全部成功 + 首个被拒的 rule。
// 对 DimConcurrent rules，成功时 INCR；对其他维度，成功时 ZADD 到 sliding window。
func (ml *MultiLimiter) Acquire(ctx context.Context, rules []Rule) (ok bool, denied *Rule) {
	if ml == nil || ml.rdb == nil {
		return true, nil
	}
	// 1) 先 check 所有 rules（fail-OPEN：Redis 出错时 Check 已返回 nil）
	if d := ml.Check(ctx, rules); d != nil {
		return false, d
	}
	// 2) 全部通过后逐个扣减；中途失败则 best-effort 回滚并 fail-OPEN
	for i := range rules {
		r := &rules[i]
		if !r.active() {
			continue
		}
		if err := ml.acquireOne(ctx, r); err != nil {
			logx.Errorf("[ratelimit.MultiLimiter.Acquire] redis err=%v dim=%s key=%s; fail-OPEN; rollback [0,%d)",
				err, r.Dimension, r.Key, i)
			ml.rollback(ctx, rules, i)
			return true, nil
		}
	}
	return true, nil
}

// acquireOne 扣减单个 rule。
func (ml *MultiLimiter) acquireOne(ctx context.Context, r *Rule) error {
	key := redisKey(r.Dimension, r.Key)
	if r.Dimension == DimConcurrent {
		n, err := ml.rdb.IncrCtx(ctx, key)
		if err != nil {
			return err
		}
		// 首次 INCR 时设置 TTL，防止因 Release 失败导致长期泄漏
		if n == 1 {
			if err := ml.rdb.ExpireCtx(ctx, key, concurrentTTL); err != nil {
				return err
			}
		}
		return nil
	}

	// sliding window：再次清理过期项 + ZADD 当前请求 + 刷新 TTL
	now := time.Now().UnixNano()
	if _, err := ml.rdb.ZremrangebyscoreCtx(ctx, key, 0, now-int64(time.Minute)); err != nil {
		return err
	}
	member := fmt.Sprintf("%d", now)
	if _, err := ml.rdb.ZaddCtx(ctx, key, now, member); err != nil {
		return err
	}
	if err := ml.rdb.ExpireCtx(ctx, key, slidingWindowTTL); err != nil {
		return err
	}
	return nil
}

// rollback 回滚 rules[0, untilIdx) 中已扣减的并发占位。
//
// 非 concurrent 维度的 sliding-window 项无法精确移除（ZADD 多一条不会立即触发限流，
// 最坏情况是下一次 check 多算 1 — 与 fail-OPEN 设计一致，可接受）。
func (ml *MultiLimiter) rollback(ctx context.Context, rules []Rule, untilIdx int) {
	for i := 0; i < untilIdx; i++ {
		r := &rules[i]
		if r.Dimension != DimConcurrent || r.MaxConcurrent <= 0 {
			continue
		}
		if _, err := ml.rdb.DecrCtx(ctx, redisKey(r.Dimension, r.Key)); err != nil {
			logx.Errorf("[ratelimit.MultiLimiter.rollback] decr err=%v dim=%s key=%s",
				err, r.Dimension, r.Key)
		}
	}
}

// Release 释放并发占位（仅对 DimConcurrent rules 有意义，DECR）。
// 对其他维度 rules 是 no-op。
// 必须与成功 Acquire 一一配对调用（用 defer）。
func (ml *MultiLimiter) Release(ctx context.Context, rules []Rule) {
	if ml == nil || ml.rdb == nil {
		return
	}
	for i := range rules {
		r := &rules[i]
		if r.Dimension != DimConcurrent || r.MaxConcurrent <= 0 {
			continue
		}
		key := redisKey(r.Dimension, r.Key)
		n, err := ml.rdb.DecrCtx(ctx, key)
		if err != nil {
			// DECR 失败只记录日志，不 panic（与 spec 一致）
			logx.Errorf("[ratelimit.MultiLimiter.Release] decr err=%v dim=%s key=%s",
				err, r.Dimension, r.Key)
			continue
		}
		// DECR 后若 < 0（如 Release 被重复调用），重置为 0 防止负数累积
		if n < 0 {
			if err := ml.rdb.SetCtx(ctx, key, "0"); err != nil {
				logx.Errorf("[ratelimit.MultiLimiter.Release] reset err=%v dim=%s key=%s",
					err, r.Dimension, r.Key)
			}
		}
	}
}

// HTTPMiddleware 工厂：根据请求生成 rules 并执行 Acquire/Release。
// ruleGen 在请求处理前调用，返回 rules；handler 完成后 Release（仅并发 rules）。
// 被拒时返回 429 + Retry-After: 60。
//
// ruleGen 返回 nil/空切片时直接放行（调用方负责根据请求决定是否限流）。
func (ml *MultiLimiter) HTTPMiddleware(ruleGen func(*http.Request) []Rule) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			rules := ruleGen(r)
			if len(rules) == 0 {
				next(w, r)
				return
			}
			ok, denied := ml.Acquire(ctx, rules)
			if !ok {
				w.Header().Set("Retry-After", "60")
				httpx.ErrorCtx(ctx, w, fmt.Errorf("rate limit: %s %s", denied.Dimension, denied.Key))
				return
			}
			defer ml.Release(ctx, rules)
			next(w, r)
		}
	}
}
