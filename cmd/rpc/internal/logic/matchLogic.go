package logic

import (
        "context"
        "errors"
        "time"

        "apipro/cmd/rpc/internal/svc"
        "apipro/common/cache"
        "apipro/desc/proto/gen/apipro"
        "apipro/pkg/fixture"

        "github.com/zeromicro/go-zero/core/logx"
)

// ---- MatchListByDate ----
type MatchListByDateLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewMatchListByDateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MatchListByDateLogic {
        return &MatchListByDateLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MatchListByDateLogic) MatchListByDate(in *apipro.DateReq) (*apipro.MatchListResp, error) {
        date := in.Date
        if date == "" {
                date = fixture.Today()
        }
        if !fixture.DateIsValid(date) {
                return nil, errors.New("invalid date, expect YYYYMMDD")
        }
        page, pageSize := fixture.ParsePage(in.Page, in.PageSize)
        ttl := time.Duration(l.svcCtx.Config.CacheMatchListTtl) * time.Second
        list, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "date:"+date, ttl, func() ([]fixture.MatchItem, error) {
                return svc.LoadMatchesByDate(l.ctx, l.svcCtx.Models, date)
        })
        if err != nil {
                return nil, err
        }
        total := int64(len(list))
        start, end := pageBounds(page, pageSize, len(list))
        slice := list[start:end]
        return &apipro.MatchListResp{List: toMatchItems(slice), Total: total, Date: date}, nil
}

// ---- MatchRecommend ----
type MatchRecommendLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewMatchRecommendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MatchRecommendLogic {
        return &MatchRecommendLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MatchRecommendLogic) MatchRecommend(in *apipro.Empty) (*apipro.MatchRecommendResp, error) {
        ttl := time.Duration(l.svcCtx.Config.CacheMatchListTtl) * time.Second
        list, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "recommend", ttl, func() ([]fixture.MatchItem, error) {
                return svc.LoadRecommend(l.ctx, l.svcCtx.Models)
        })
        if err != nil {
                return nil, err
        }
        return &apipro.MatchRecommendResp{List: toMatchItems(list)}, nil
}

// ---- MatchDetail ----
type MatchDetailLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewMatchDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MatchDetailLogic {
        return &MatchDetailLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MatchDetailLogic) MatchDetail(in *apipro.IdReq) (*apipro.MatchDetailResp, error) {
        if in.Id == "" {
                return nil, errors.New("missing id")
        }
        ttl := time.Duration(l.svcCtx.Config.CacheMatchDetailTtl) * time.Second
        m, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "detail:"+in.Id, ttl, func() (fixture.MatchItem, error) {
                return svc.LoadMatchByID(l.ctx, l.svcCtx.Models, in.Id)
        })
        if err != nil {
                return nil, err
        }
        return &apipro.MatchDetailResp{Match: toMatchItem(m)}, nil
}

// ---- MatchCateList ----
type MatchCateListLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewMatchCateListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MatchCateListLogic {
        return &MatchCateListLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MatchCateListLogic) MatchCateList(in *apipro.Empty) (*apipro.CateListResp, error) {
        ttl := time.Duration(l.svcCtx.Config.CacheMatchListTtl) * time.Second
        cates, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "cates", ttl, func() ([]string, error) {
                return l.svcCtx.Models.Matches.ListCateNames(l.ctx)
        })
        if err != nil {
                return nil, err
        }
        return &apipro.CateListResp{CateNames: cates}, nil
}

// ---- MatchListByCate ----
type MatchListByCateLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewMatchListByCateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MatchListByCateLogic {
        return &MatchListByCateLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *MatchListByCateLogic) MatchListByCate(in *apipro.CateReq) (*apipro.MatchListResp, error) {
        if in.CateName == "" {
                return nil, errors.New("missing cateName")
        }
        page, pageSize := fixture.ParsePage(in.Page, in.PageSize)
        ttl := time.Duration(l.svcCtx.Config.CacheMatchListTtl) * time.Second
        list, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "cate:"+in.CateName, ttl, func() ([]fixture.MatchItem, error) {
                return svc.LoadMatchesByCate(l.ctx, l.svcCtx.Models, in.CateName)
        })
        if err != nil {
                return nil, err
        }
        total := int64(len(list))
        start, end := pageBounds(page, pageSize, len(list))
        return &apipro.MatchListResp{List: toMatchItems(list[start:end]), Total: total, Date: in.CateName}, nil
}

// ---- helpers ----
func pageBounds(page, pageSize, n int) (int, int) {
        start := (page - 1) * pageSize
        if start > n {
                start = n
        }
        end := start + pageSize
        if end > n {
                end = n
        }
        return start, end
}

func toMatchItem(m fixture.MatchItem) *apipro.MatchItem {
        return &apipro.MatchItem{
                ScheduleId: m.ScheduleId, SubCateName: m.SubCateName, CateName: m.CateName,
                MatchTime: m.MatchTime, HostName: m.HostName, HostIcon: m.HostIcon,
                GuestName: m.GuestName, GuestIcon: m.GuestIcon, Venue: m.Venue,
                Status: m.Status, ReservationStatus: m.ReservationStatus,
                Anchors: toCommentators(m.Anchors),
                CategoryId: m.CategoryId, CategoryName: m.CategoryName,
                CategoryIcon: m.CategoryIcon, MatchStatusDesc: m.MatchStatusDesc,
                HostScore: m.HostScore, GuestScore: m.GuestScore,
        }
}

func toMatchItems(ms []fixture.MatchItem) []*apipro.MatchItem {
        out := make([]*apipro.MatchItem, 0, len(ms))
        for _, m := range ms {
                out = append(out, toMatchItem(m))
        }
        return out
}
