package live

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

func (l *ListLogic) List(req *types.PageReq) (resp *types.LiveListResp, err error) {
        cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
        out, err := cli.LiveList(l.ctx, &apiproClient.PageReq{Page: req.Page, PageSize: req.PageSize})
        if err != nil {
                return nil, err
        }
        return &types.LiveListResp{
                List:       conv.LiveRooms(out.List),
                Total:      out.Total,
                All:        conv.LiveRooms(out.All),
                Football:   conv.LiveRooms(out.Football),
                Basketball: conv.LiveRooms(out.Basketball),
                Hot:        conv.LiveRooms(out.Hot),
        }, nil
}

type TypesLogic struct {
        logx.Logger
        ctx    context.Context
        svcCtx *svc.ServiceContext
}

func NewTypesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *TypesLogic {
        return &TypesLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *TypesLogic) Types() (resp *types.LiveTypeListResp, err error) {
        cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
        out, err := cli.LiveTypeList(l.ctx, &apiproClient.Empty{})
        if err != nil {
                return nil, err
        }
        return &types.LiveTypeListResp{List: conv.LiveTypes(out.List)}, nil
}

type HotAnchorLogic struct {
        logx.Logger
        ctx    context.Context
        svcCtx *svc.ServiceContext
}

func NewHotAnchorLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HotAnchorLogic {
        return &HotAnchorLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *HotAnchorLogic) HotAnchor() (resp *types.HotAnchorResp, err error) {
        cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
        out, err := cli.HotAnchor(l.ctx, &apiproClient.Empty{})
        if err != nil {
                return nil, err
        }
        return &types.HotAnchorResp{List: conv.Commentators(out.List)}, nil
}
