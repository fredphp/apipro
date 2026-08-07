package commentator

import (
	"context"

	"apipro/cmd/api/internal/conv"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLogic {
	return &ListLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ListLogic) List(req *types.PageReq) (resp *types.CommentatorListResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.CommentatorList(l.ctx, &apiproClient.PageReq{Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		return nil, err
	}
	return &types.CommentatorListResp{List: conv.Commentators(out.List), Total: out.Total}, nil
}

type DetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DetailLogic {
	return &DetailLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *DetailLogic) Detail(req *types.IdReq) (resp *types.CommentatorDetailResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.CommentatorDetail(l.ctx, &apiproClient.IdReq{Id: req.Id})
	if err != nil {
		return nil, err
	}
	c := conv.Commentator(out.Commentator)
	return &types.CommentatorDetailResp{Commentator: c}, nil
}
