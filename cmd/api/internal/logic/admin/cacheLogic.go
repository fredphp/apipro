package admin

import (
	"context"

	"apipro/cmd/api/internal/conv"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"

	"github.com/zeromicro/go-zero/core/logx"
)

type RefreshLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRefreshLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshLogic {
	return &RefreshLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RefreshLogic) Refresh(req *types.CacheRefreshReq) (resp *types.CacheRefreshResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.RefreshCache(l.ctx, &apiproClient.CacheRefreshReq{Family: req.Family})
	if err != nil {
		return nil, err
	}
	return &types.CacheRefreshResp{Ok: out.Ok, Message: out.Message}, nil
}

type StatsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStatsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StatsLogic {
	return &StatsLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *StatsLogic) Stats() (resp *types.CacheStatsResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.CacheStats(l.ctx, &apiproClient.Empty{})
	if err != nil {
		return nil, err
	}
	return &types.CacheStatsResp{Hits: out.Hits, Misses: out.Misses, Keys: out.Keys, Families: conv.FamilyStats(out.Families)}, nil
}
