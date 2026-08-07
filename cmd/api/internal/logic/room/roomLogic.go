package room

import (
	"context"

	"apipro/cmd/api/internal/conv"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"

	"github.com/zeromicro/go-zero/core/logx"
)

type DetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DetailLogic {
	return &DetailLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DetailLogic) Detail(req *types.RoomNumReq) (resp *types.RoomDetailResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.RoomDetail(l.ctx, &apiproClient.RoomNumReq{RoomNum: req.RoomNum})
	if err != nil {
		return nil, err
	}
	r := conv.RoomDetail(out.Room)
	return &types.RoomDetailResp{Room: r}, nil
}

type ScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ScheduleLogic {
	return &ScheduleLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ScheduleLogic) Schedule(req *types.RoomNumReq) (resp *types.RoomScheduleResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.RoomSchedule(l.ctx, &apiproClient.RoomNumReq{RoomNum: req.RoomNum})
	if err != nil {
		return nil, err
	}
	return &types.RoomScheduleResp{List: conv.MatchItems(out.List)}, nil
}

type RankLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRankLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RankLogic {
	return &RankLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RankLogic) Rank(req *types.RoomNumReq) (resp *types.RoomRankResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.RoomRank(l.ctx, &apiproClient.RoomNumReq{RoomNum: req.RoomNum})
	if err != nil {
		return nil, err
	}
	return &types.RoomRankResp{List: conv.RoomRankItems(out.List)}, nil
}
