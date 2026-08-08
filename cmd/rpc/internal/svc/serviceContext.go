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
        "apipro/common/model"

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
        sms := NewSmsStore(rdb, c.Mode, c.SmsDevBypassCode)

        // Bootstrap chat message ID counter from DB.
        bootChatMsgIDCounter(context.Background(), rdb, models.ChatMessages)

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
        return &ServiceContext{
                Config: c, Redis: rdb, Cache: ch, Scheduler: sch,
                Models: models, Sessions: sessions, SmsStore: sms,
                UserAsset: UserAssetPrefixer{Base: c.FileBaseURL},
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

// ----- warm helpers (write-through to cache) -----

func warmMatchCatalog(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
        return cache.Refresh(ctx, ch, "match", "catalog", ttl, func() ([]byte, error) {
                out := BuildMatchCatalog(ctx, m)
                b, _ := json.Marshal(out)
                return b, nil
        })
}

func warmRecommend(ctx context.Context, ch *cache.Cache, m *Models, ttl time.Duration) error {
        return cache.Refresh(ctx, ch, "match", "recommend", ttl, func() ([]byte, error) {
                out := BuildRecommend(ctx, m)
                b, _ := json.Marshal(out)
                return b, nil
        })
}

func warmMatchByDate(ctx context.Context, ch *cache.Cache, m *Models, date string, ttl time.Duration) error {
        return cache.Refresh(ctx, ch, "match", "date:"+date, ttl, func() ([]byte, error) {
                out := BuildMatchByDate(ctx, m, date)
                b, _ := json.Marshal(out)
                return b, nil
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
