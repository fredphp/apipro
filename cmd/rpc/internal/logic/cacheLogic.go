package logic

import (
	"context"
	"fmt"

	"apipro/cmd/rpc/internal/svc"
	"apipro/desc/proto/gen/apipro"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshCacheLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRefreshCacheLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshCacheLogic {
	return &RefreshCacheLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RefreshCacheLogic) RefreshCache(in *apipro.CacheRefreshReq) (*apipro.CacheRefreshResp, error) {
	family := in.Family
	if family == "" {
		family = "all"
	}
	families := []string{family}
	if family == "all" {
		families = []string{"match", "live", "commentator", "room", "user"}
	}
	total := 0
	for _, f := range families {
		n, err := l.svcCtx.Cache.InvalidateFamily(l.ctx, f)
		if err != nil {
			return &apipro.CacheRefreshResp{Ok: false, Message: err.Error()}, nil
		}
		total += n
	}
	return &apipro.CacheRefreshResp{Ok: true, Message: fmt.Sprintf("invalidated %d keys across %v", total, families)}, nil
}

type CacheStatsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCacheStatsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CacheStatsLogic {
	return &CacheStatsLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CacheStatsLogic) CacheStats(in *apipro.Empty) (*apipro.CacheStatsResp, error) {
	hits, misses := l.svcCtx.Cache.Stats().Snapshot()
	var totalHits, totalMisses int64
	families := []string{"match", "live", "commentator", "room", "user", "chat"}
	out := make([]*apipro.FamilyStat, 0, len(families))
	for _, f := range families {
		h := hits[f]
		m := misses[f]
		totalHits += h
		totalMisses += m
		out = append(out, &apipro.FamilyStat{
			Family: f, Keys: l.svcCtx.Cache.CountFamily(l.ctx, f),
			Hits: h, Misses: m,
		})
	}
	return &apipro.CacheStatsResp{
		Hits: totalHits, Misses: totalMisses,
		Keys: l.svcCtx.Cache.CountKeys(l.ctx), Families: out,
	}, nil
}
