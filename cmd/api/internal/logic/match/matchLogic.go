package match

import (
	"context"

	"apipro/cmd/api/internal/conv"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListByDateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListByDateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListByDateLogic {
	return &ListByDateLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListByDateLogic) ListByDate(req *types.DateReq) (resp *types.MatchListResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.MatchListByDate(l.ctx, &apiproClient.DateReq{Date: req.Date, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		return nil, err
	}
	return &types.MatchListResp{List: conv.MatchItems(out.List), Total: out.Total, Date: out.Date}, nil
}

type RecommendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRecommendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RecommendLogic {
	return &RecommendLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RecommendLogic) Recommend() (resp *types.MatchRecommendResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.MatchRecommend(l.ctx, &apiproClient.Empty{})
	if err != nil {
		return nil, err
	}
	return &types.MatchRecommendResp{List: conv.MatchItems(out.List)}, nil
}

type DetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DetailLogic {
	return &DetailLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DetailLogic) Detail(req *types.IdReq) (resp *types.MatchDetailResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.MatchDetail(l.ctx, &apiproClient.IdReq{Id: req.Id})
	if err != nil {
		return nil, err
	}
	m := conv.MatchItem(out.Match)
	return &types.MatchDetailResp{Match: m}, nil
}

type CateListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCateListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CateListLogic {
	return &CateListLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *CateListLogic) CateList() (resp *types.CateListResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.MatchCateList(l.ctx, &apiproClient.Empty{})
	if err != nil {
		return nil, err
	}
	return &types.CateListResp{CateNames: out.CateNames}, nil
}

type ListByCateLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListByCateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListByCateLogic {
	return &ListByCateLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListByCateLogic) ListByCate(req *types.CateReq) (resp *types.MatchListResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.MatchListByCate(l.ctx, &apiproClient.CateReq{CateName: req.CateName, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		return nil, err
	}
	return &types.MatchListResp{List: conv.MatchItems(out.List), Total: out.Total, Date: out.Date}, nil
}
