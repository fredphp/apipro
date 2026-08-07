package svc

import (
        "context"
        "time"

        "apipro/cmd/rpc/internal/config"
        "apipro/common/cache"
        "apipro/common/db"
        "apipro/common/model"
        "apipro/common/store"
        "apipro/pkg/fixture"

        "github.com/zeromicro/go-zero/core/logx"
        "github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
        Config    config.Config
        Redis     *redis.Redis
        Cache     *cache.Cache
        Scheduler *cache.Scheduler
        Users     *store.UserStore
        Models    *Models
}

// Models bundles all data models for convenient access from logic.
type Models struct {
        Users    *model.UserModel
        Anchors  *model.AnchorModel
        Rooms    *model.RoomModel
        Matches  *model.MatchModel
}

func NewServiceContext(c config.Config) *ServiceContext {
        rdb := redis.MustNewRedis(c.CacheRedis)
        ch := cache.New(rdb)
        sch := cache.NewScheduler()

        // ---- Database (data source) ----
        sqlDB := db.MustNew(c.DBDriver, c.DataSource)
        models := &Models{
                Users:   model.NewUserModel(sqlDB),
                Anchors: model.NewAnchorModel(sqlDB),
                Rooms:   model.NewRoomModel(sqlDB),
                Matches: model.NewMatchModel(sqlDB),
        }
        users := store.NewUserStore(models.Users, rdb)

        // ---- Scheduled cache refresh (定时刷新缓存) ----
        // Each job reads from the DB and warms the Redis cache so that hot keys
        // are always fresh and requests rarely hit the DB.
        sch.Add(cache.RefreshJob{
                Family: "match", Every: dur(c.RefreshMatchListTtl, 60),
                Run: func() error {
                        ctx := context.Background()
                        // warm today + next 6 days
                        for d := 0; d < 7; d++ {
                                date := time.Now().AddDate(0, 0, d).Format("20060102")
                                _ = cache.Refresh(ctx, ch, "match", "date:"+date, dur(c.CacheMatchListTtl, 60), func() ([]fixture.MatchItem, error) {
                                        return LoadMatchesByDate(ctx, models, date)
                                })
                        }
                        _ = cache.Refresh(ctx, ch, "match", "recommend", dur(c.CacheMatchListTtl, 60), func() ([]fixture.MatchItem, error) {
                                return LoadRecommend(ctx, models)
                        })
                        _ = cache.Refresh(ctx, ch, "match", "cates", dur(c.CacheMatchListTtl, 60), func() ([]string, error) {
                                return models.Matches.ListCateNames(ctx)
                        })
                        return nil
                },
        })
        sch.Add(cache.RefreshJob{
                Family: "live", Every: dur(c.RefreshLiveTtl, 15),
                Run: func() error {
                        ctx := context.Background()
                        _ = cache.Refresh(ctx, ch, "live", "list", dur(c.CacheLiveTtl, 15), func() ([]fixture.LiveRoom, error) {
                                return LoadLiveRooms(ctx, models)
                        })
                        _ = cache.Refresh(ctx, ch, "live", "types", dur(c.CacheLiveTtl, 15), func() ([]fixture.LiveType, error) {
                                return LoadLiveTypes(ctx, models)
                        })
                        _ = cache.Refresh(ctx, ch, "live", "hot", dur(c.CacheLiveTtl, 15), func() ([]fixture.Commentator, error) {
                                return LoadHotAnchors(ctx, models, 6)
                        })
                        // refresh each room detail cache
                        rooms, _ := models.Rooms.ListAll(ctx)
                        for _, r := range rooms {
                                rn := r.RoomNum
                                _ = cache.Refresh(ctx, ch, "room", "num:"+rn, dur(c.CacheRoomDetailTtl, 30), func() (fixture.RoomDetail, error) {
                                        return LoadRoomDetail(ctx, models, rn)
                                })
                        }
                        return nil
                },
        })
        sch.Add(cache.RefreshJob{
                Family: "commentator", Every: dur(c.RefreshCommentatorTtl, 120),
                Run: func() error {
                        ctx := context.Background()
                        _ = cache.Refresh(ctx, ch, "commentator", "list", dur(c.CacheCommentatorTtl, 120), func() ([]fixture.Commentator, error) {
                                return LoadCommentators(ctx, models)
                        })
                        anchors, _ := models.Anchors.ListAll(ctx)
                        for _, a := range anchors {
                                uid := a.Uid
                                _ = cache.Refresh(ctx, ch, "commentator", "uid:"+uid, dur(c.CacheCommentatorTtl, 120), func() (fixture.Commentator, error) {
                                        return LoadCommentator(ctx, models, uid)
                                })
                        }
                        return nil
                },
        })

        logx.Infof("ServiceContext ready: DBDriver=%s", c.DBDriver)
        return &ServiceContext{
                Config: c, Redis: rdb, Cache: ch, Scheduler: sch,
                Users: users, Models: models,
        }
}

func dur(seconds, fallback int) time.Duration {
        if seconds <= 0 {
                seconds = fallback
        }
        return time.Duration(seconds) * time.Second
}
