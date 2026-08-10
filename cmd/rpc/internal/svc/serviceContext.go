package svc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"apipro/cmd/rpc/internal/config"
	"apipro/common/auth"
	"apipro/common/cache"
	"apipro/common/db"
	"apipro/common/degrade"
	"apipro/common/model"
	"apipro/common/observability"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// ServiceContext wires up all dependencies for the RPC server.
type ServiceContext struct {
	Config    config.Config
	Redis     *redis.Redis
	Cache     *cache.Cache
	Scheduler *cache.Scheduler

	Models *Models

	Sessions  *auth.SessionStore
	SmsStore  *SmsStore
	UserAsset UserAssetPrefixer

	// AllocUID atomically reserves a new uid via Redis INCR (AUDIT-008).
	AllocUID func(ctx context.Context) (int64, error)

	// ---- Phase 0 infrastructure (audit-1C) ----
	// Manager: L1+L2+SingleFlight+Breaker+Fallback+Degrade+Metrics (整合层)
	Manager *cache.Manager
	// Degrade: 全局降级状态机 (NORMAL/DEGRADED/PROTECTED/EMERGENCY)
	Degrade *degrade.Manager
	// Fallback: Level 1 接口的 stale fallback chain (L1 stale → OSS → CDN)
	Fallback *cache.FallbackManager
	// DBSem: DB 并发信号量 (Level 3 write 接口用 WithToken 包装 INSERT)
	DBSem *db.Semaphore
	// ObsMetrics: 启动时一次性创建（避免重复注册 panic）
	DegradeMetrics *observability.DegradeMetrics
	DBMetrics      *observability.DBMetrics
}

// Models bundles all data models for convenient access from logic.
type Models struct {
	Users        *model.UserModel
	Anchors      *model.AnchorModel
	Rooms        *model.LiveRoomModel
	Matches      *model.MatchModel
	LiveTypes    *model.LiveTypeModel
	GiftRanks    *model.RoomGiftRankModel
	ChatMessages *model.ChatRoomMessageModel
}

// UserAssetPrefixer prepends FileBaseURL to relative asset paths (icon/cover).
type UserAssetPrefixer struct {
	Base string
}

func (u UserAssetPrefixer) URL(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	if u.Base == "" {
		return p
	}
	return strings.TrimRight(u.Base, "/") + "/" + strings.TrimLeft(p, "/")
}

func NewServiceContext(c config.Config) *ServiceContext {
	rdb := redis.MustNewRedis(c.CacheRedis)
	ch := cache.New(rdb)
	sch := cache.NewScheduler()

	// Configure schema-name qualification BEFORE constructing models:
	//   MySQL  → use qualified <prefix>schema.table (e.g. zb_user.user)
	//   SQLite → use bare table names (single-file DB, no namespaces)
	switch strings.ToLower(strings.TrimSpace(c.DBDriver)) {
	case "sqlite", "sqlite3":
		model.SetNoSchemaPrefix()
	default:
		if p := strings.TrimSpace(c.SchemaPrefix); p != "" {
			model.SetSchemaPrefix(p)
		}
	}

	sqlDB := db.MustNew(c.DBDriver, c.DataSource)
	models := &Models{
		Users:        model.NewUserModel(sqlDB),
		Anchors:      model.NewAnchorModel(sqlDB),
		Rooms:        model.NewLiveRoomModel(sqlDB),
		Matches:      model.NewMatchModel(sqlDB),
		LiveTypes:    model.NewLiveTypeModel(sqlDB),
		GiftRanks:    model.NewRoomGiftRankModel(sqlDB),
		ChatMessages: model.NewChatRoomMessageModel(sqlDB),
	}

	// Sessions (Redis-backed opaque tokens).
	sessions := auth.NewSessionStore(rdb)

	// SMS code store (Redis).
	// AUDIT-002: use c.AppMode (dev|prod) — NOT c.Mode which is go-zero's
	// ServiceConf.Mode (defaults to "pro" and never equals "dev").
	sms := NewSmsStore(rdb, c.AppMode, c.SmsDevBypassCode)

	// Bootstrap chat message ID counter from DB.
	bootChatMsgIDCounter(context.Background(), rdb, models.ChatMessages)

	// AUDIT-008: bootstrap UID counter from MAX(uid) so the first INCR
	// returns a value strictly greater than any existing uid.
	bootUIDCounter(context.Background(), rdb, models.Users)

	// AllocUID reserves a new uid atomically via Redis INCR. Falls back to
	// the DB-based NextUID if Redis is unavailable (best-effort).
	allocUID := func(ctx context.Context) (int64, error) {
		if rdb != nil {
			uid, err := rdb.IncrCtx(ctx, "yuyan:uid:next")
			if err == nil {
				return uid, nil
			}
			logx.Errorf("AllocUID: redis incr failed: %v — falling back to DB", err)
		}
		return models.Users.NextUID(ctx)
	}

	// ---- Scheduled cache refresh ----
	// Each job reads from the DB and warms the Redis cache so that hot keys
	// are always fresh.
	sch.Add(cache.RefreshJob{
		Family: "match", Every: dur(c.RefreshMatchListTtl, 60),
		Run: func() error {
			ctx := context.Background()
			// Warm today + next 6 days + the catalog (matches.json) + recommend
			_ = warmMatchCatalog(ctx, ch, models, dur(c.CacheMatchListTtl, 60))
			_ = warmRecommend(ctx, ch, models, dur(c.CacheMatchListTtl, 60))
			for d := 0; d < 7; d++ {
				date := time.Now().AddDate(0, 0, d).Format("20060102")
				_ = warmMatchByDate(ctx, ch, models, date, dur(c.CacheMatchListTtl, 60))
			}
			return nil
		},
	})
	sch.Add(cache.RefreshJob{
		Family: "live", Every: dur(c.RefreshLiveTtl, 15),
		Run: func() error {
			ctx := context.Background()
			_ = warmLiveRooms(ctx, ch, models, dur(c.CacheLiveTtl, 15))
			_ = warmLiveTypes(ctx, ch, models, dur(c.CacheLiveTtl, 15))
			return nil
		},
	})
	sch.Add(cache.RefreshJob{
		Family: "commentator", Every: dur(c.RefreshCommentatorTtl, 120),
		Run: func() error {
			ctx := context.Background()
			_ = cache.Refresh(ctx, ch, "commentator", "list", dur(c.CacheCommentatorTtl, 120), func() ([]model.Anchor, error) {
				return models.Anchors.ListAll(ctx)
			})
			return nil
		},
	})

	logx.Infof("ServiceContext ready: DBDriver=%s mode=%s", c.DBDriver, c.Mode)

	// ---- Phase 0 infrastructure 初始化 ----
	// DegradeManager (单例)
	dgr := degrade.New()
	// DBSemaphore (max=50, 与连接池上限对齐)
	dbSem := db.NewSemaphore("mysql-main", 50)
	// FallbackManager: Level 1 链 L1 stale → OSS → CDN
	// OSS/CDN baseURL 暂时留空（生产时由 config 注入），CDNSource 始终非 nil
	// 保证 Emergency 模式下 Level 1 仍有兜底（即使返回的是 cdn_redirect 提示）
	fb := cache.NewFallbackManager(
		cache.NewL1StaleSource(nil),    // L1 source 在 Manager.SetL1 后注入；这里先 nil，Manager 会接管
		cache.NewOSSSnapshotSource(""), // empty baseURL → 该 source 返回 miss
		cache.NewCDNSource("https://cdn.example.com/apipro"),
	)
	// CacheManager (L1 64MB)
	mgr := cache.NewManager(64, rdb, dgr, fb)
	// 把 Manager 的 L1 注入回 FallbackManager 的 L1StaleSource（共享同一个 freecache 实例）
	// 重新构造 fb 使其持有 Manager 的 L1
	fb = cache.NewFallbackManager(
		cache.NewL1StaleSource(mgr.L1()),
		cache.NewOSSSnapshotSource(""),
		cache.NewCDNSource("https://cdn.example.com/apipro"),
	)
	// 由于 fb 已重新构造，需要重新设置给 mgr（mgr 内部还持有旧 fb）
	// 这里通过重新创建 mgr 简化逻辑（cache.NewManager 内部只赋值，无副作用）
	mgr = cache.NewManager(64, rdb, dgr, fb)

	// 启动时自检
	l1OK, l2OK, fbOK := mgr.SelfTest(context.Background())
	logx.Infof("Phase0 self-test: L1=%v L2=%v Fallback=%v", l1OK, l2OK, fbOK)
	if !l2OK {
		logx.Errorf("Phase0: Redis L2 unavailable — Level 1/2 will degrade to L1 only; Level 3 (write) still allowed but unprotected by cache")
		// 自动进入 Degraded 模式 (Level 3 仍可服务，Level 1/2 走 L1)
		dgr.Set(degrade.ModeDegraded)
	}

	// 启动 metrics
	dgrMetrics := observability.NewDegradeMetrics()
	dbMetrics := observability.NewDBMetrics("mysql-main")
	// 把 DegradeManager 的 mode 同步到 metrics gauge
	go func() {
		watch := dgr.Watch()
		dgrMetrics.Mode.Set(float64(dgr.Mode()))
		for mode := range watch {
			dgrMetrics.Mode.Set(float64(mode))
			logx.Infof("DegradeManager mode changed: %v", mode)
		}
	}()

	return &ServiceContext{
		Config: c, Redis: rdb, Cache: ch, Scheduler: sch,
		Models: models, Sessions: sessions, SmsStore: sms,
		UserAsset: UserAssetPrefixer{Base: c.FileBaseURL},
		AllocUID:  allocUID,

		// Phase 0 infrastructure
		Manager:        mgr,
		Degrade:        dgr,
		Fallback:       fb,
		DBSem:          dbSem,
		DegradeMetrics: dgrMetrics,
		DBMetrics:      dbMetrics,
	}
}

func dur(seconds, fallback int) time.Duration {
	if seconds <= 0 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

// bootChatMsgIDCounter seeds the Redis chat_room_message_id counter from DB MAX.
func bootChatMsgIDCounter(ctx context.Context, rdb *redis.Redis, m *model.ChatRoomMessageModel) {
	if rdb == nil {
		return
	}
	maxID, err := m.MaxID(ctx)
	if err != nil {
		logx.Errorf("boot chat msg id: %v", err)
		return
	}
	if maxID > 0 {
		// Try to set; if key already exists with a higher value, INCR will keep going.
		_ = rdb.Set("yuyan:chat:message_id", fmt.Sprintf("%d", maxID))
	}
}

// bootUIDCounter seeds the Redis yuyan:uid:next counter from DB MAX(uid) on
// first startup (AUDIT-008). Uses SETNX so a pre-existing counter (e.g. from
// a prior run with higher uids) is never lowered.
func bootUIDCounter(ctx context.Context, rdb *redis.Redis, m *model.UserModel) {
	if rdb == nil {
		return
	}
	maxUID, err := m.MaxUID(ctx)
	if err != nil {
		logx.Errorf("boot uid counter: %v", err)
		return
	}
	// Floor at 5000 so the first INCR returns 5001+ (avoids clashing with
	// seeded anchors 1001-1003 and the demo user 5001).
	if maxUID < 5000 {
		maxUID = 5000
	}
	// SETNX: only sets if missing — never overwrites a higher counter.
	_, _ = rdb.Setnx("yuyan:uid:next", fmt.Sprintf("%d", maxUID))
}

// ----- warm helpers (write-through to cache) -----

func warmMatchCatalog(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
	return cache.Refresh(ctx, ch, "match", "catalog", ttl, func() ([]byte, error) {
		out := BuildMatchCatalog(ctx, m)
		b, _ := json.Marshal(out)
		return b, nil
	})
}

func warmRecommend(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
	// Phase 1 fix: 返回 []MatchCatalogItem 而不是 []byte，让 cache.Refresh 的 json.Marshal
	// 正确序列化为 JSON 数组（之前返回 []byte 会被 marshal 成 base64 字符串，
	// 导致 Phase 1 样板 GetOrLoadT 无法反序列化为 []MatchCatalogItem）
	return cache.Refresh(ctx, ch, "match", "recommend", ttl, func() ([]MatchCatalogItem, error) {
		return BuildRecommend(ctx, m), nil
	})
}

func warmMatchByDate(ctx context.Context, ch *cache.Cache, m *Models, date string, ttl time.Duration) error {
	return cache.Refresh(ctx, ch, "match", "date:"+date, ttl, func() ([]MatchCatalogItem, error) {
		return BuildMatchByDate(ctx, m, date), nil
	})
}

func warmLiveRooms(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
	return cache.Refresh(ctx, ch, "live", "all", ttl, func() ([]byte, error) {
		out := BuildAllLiveRooms(ctx, m)
		b, _ := json.Marshal(out)
		return b, nil
	})
}

func warmLiveTypes(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
	return cache.Refresh(ctx, ch, "live", "types", ttl, func() ([]byte, error) {
		lts, err := m.LiveTypes.ListTopLevel(ctx)
		if err != nil {
			return nil, err
		}
		out := BuildLiveTypesJSON(lts)
		b, _ := json.Marshal(out)
		return b, nil
	})
}
