package svc

import (
        "time"

        "apipro/cmd/rpc/internal/config"
        "apipro/common/cache"
        "apipro/common/store"
        "apipro/pkg/fixture"

        "github.com/zeromicro/go-zero/core/stores/redis"
)

type ServiceContext struct {
        Config     config.Config
        Redis      *redis.Redis
        Cache      *cache.Cache
        Scheduler  *cache.Scheduler
        Users      *store.UserStore
}

func NewServiceContext(c config.Config) *ServiceContext {
        rdb := redis.MustNewRedis(c.CacheRedis)
        ch := cache.New(rdb)
        sch := cache.NewScheduler()

        users := store.NewUserStore(rdb)
        // Register scheduled refresh jobs (定时刷新缓存).
        sch.Add(cache.RefreshJob{
                Family: "match", Every: dur(c.RefreshMatchListTtl, 60),
                Run: func() error {
                        // warm today + next 6 days
                        for d := 0; d < 7; d++ {
                                date := time.Now().AddDate(0, 0, d).Format("20060102")
                                _ = cache.Refresh(nil, ch, "match", "date:"+date, dur(c.CacheMatchListTtl, 60), func() ([]fixture.MatchItem, error) {
                                        return fixture.MatchesByDate(date), nil
                                })
                        }
                        _ = cache.Refresh(nil, ch, "match", "recommend", dur(c.CacheMatchListTtl, 60), func() ([]fixture.MatchItem, error) {
                                return fixture.Recommend(), nil
                        })
                        _ = cache.Refresh(nil, ch, "match", "cates", dur(c.CacheMatchListTtl, 60), func() ([]string, error) {
                                return fixture.CateNames(), nil
                        })
                        return nil
                },
        })
        sch.Add(cache.RefreshJob{
                Family: "live", Every: dur(c.RefreshLiveTtl, 15),
                Run: func() error {
                        _ = cache.Refresh(nil, ch, "live", "list", dur(c.CacheLiveTtl, 15), func() ([]fixture.LiveRoom, error) {
                                return fixture.Lives(), nil
                        })
                        _ = cache.Refresh(nil, ch, "live", "types", dur(c.CacheLiveTtl, 15), func() ([]fixture.LiveType, error) {
                                return fixture.LiveTypes(), nil
                        })
                        _ = cache.Refresh(nil, ch, "live", "hot", dur(c.CacheLiveTtl, 15), func() ([]fixture.Commentator, error) {
                                return fixture.HotAnchors(6), nil
                        })
                        // refresh each room detail cache
                        for _, r := range fixture.Rooms() {
                                rn := r.RoomNum
                                _ = cache.Refresh(nil, ch, "room", "num:"+rn, dur(c.CacheRoomDetailTtl, 30), func() (fixture.RoomDetail, error) {
                                        rr, _ := fixture.RoomByNum(rn)
                                        return rr, nil
                                })
                        }
                        return nil
                },
        })
        sch.Add(cache.RefreshJob{
                Family: "commentator", Every: dur(c.RefreshCommentatorTtl, 120),
                Run: func() error {
                        _ = cache.Refresh(nil, ch, "commentator", "list", dur(c.CacheCommentatorTtl, 120), func() ([]fixture.Commentator, error) {
                                return fixture.Commentators(), nil
                        })
                        for _, a := range fixture.Commentators() {
                                uid := a.Uid
                                _ = cache.Refresh(nil, ch, "commentator", "uid:"+uid, dur(c.CacheCommentatorTtl, 120), func() (fixture.Commentator, error) {
                                        cc, _ := fixture.CommentatorByID(uid)
                                        return cc, nil
                                })
                        }
                        return nil
                },
        })

        return &ServiceContext{Config: c, Redis: rdb, Cache: ch, Scheduler: sch, Users: users}
}

func dur(seconds, fallback int) time.Duration {
        if seconds <= 0 {
                seconds = fallback
        }
        return time.Duration(seconds) * time.Second
}
