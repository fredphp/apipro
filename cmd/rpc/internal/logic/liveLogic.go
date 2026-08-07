package logic

import (
	"context"
	"time"

	"apipro/cmd/rpc/internal/svc"
	"apipro/common/cache"
	"apipro/desc/proto/gen/apipro"
	"apipro/pkg/fixture"

	"github.com/zeromicro/go-zero/core/logx"
)

type LiveListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLiveListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LiveListLogic {
	return &LiveListLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *LiveListLogic) LiveList(in *apipro.PageReq) (*apipro.LiveListResp, error) {
	page, pageSize := fixture.ParsePage(in.Page, in.PageSize)
	ttl := time.Duration(l.svcCtx.Config.CacheLiveTtl) * time.Second
	all, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "live", "list", ttl, func() ([]fixture.LiveRoom, error) {
		return svc.LoadLiveRooms(l.ctx, l.svcCtx.Models)
	})
	if err != nil {
		return nil, err
	}
	total := int64(len(all))
	start, end := pageBounds(page, pageSize, len(all))
	return &apipro.LiveListResp{List: toLiveRooms(all[start:end]), Total: total}, nil
}

type LiveTypeListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLiveTypeListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LiveTypeListLogic {
	return &LiveTypeListLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *LiveTypeListLogic) LiveTypeList(in *apipro.Empty) (*apipro.LiveTypeListResp, error) {
	ttl := time.Duration(l.svcCtx.Config.CacheLiveTtl) * time.Second
	all, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "live", "types", ttl, func() ([]fixture.LiveType, error) {
		return svc.LoadLiveTypes(l.ctx, l.svcCtx.Models)
	})
	if err != nil {
		return nil, err
	}
	out := make([]*apipro.LiveType, 0, len(all))
	for _, t := range all {
		out = append(out, &apipro.LiveType{Code: t.Code, Name: t.Name, Icon: t.Icon})
	}
	return &apipro.LiveTypeListResp{List: out}, nil
}

type HotAnchorLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewHotAnchorLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HotAnchorLogic {
	return &HotAnchorLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *HotAnchorLogic) HotAnchor(in *apipro.Empty) (*apipro.HotAnchorResp, error) {
	ttl := time.Duration(l.svcCtx.Config.CacheLiveTtl) * time.Second
	all, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "live", "hot", ttl, func() ([]fixture.Commentator, error) {
		return svc.LoadHotAnchors(l.ctx, l.svcCtx.Models, 6)
	})
	if err != nil {
		return nil, err
	}
	return &apipro.HotAnchorResp{List: toCommentators(all)}, nil
}

func toLiveRooms(rs []fixture.LiveRoom) []*apipro.LiveRoom {
	out := make([]*apipro.LiveRoom, 0, len(rs))
	for _, r := range rs {
		out = append(out, &apipro.LiveRoom{
			RoomNum: r.RoomNum, Title: r.Title, Cover: r.Cover,
			Anchor: toCommentator(r.Anchor), ViewNum: r.ViewNum,
			LiveType: r.LiveType, CateName: r.CateName,
		})
	}
	return out
}
