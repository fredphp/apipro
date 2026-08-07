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

type RoomDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRoomDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RoomDetailLogic {
	return &RoomDetailLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RoomDetailLogic) RoomDetail(in *apipro.RoomNumReq) (*apipro.RoomDetailResp, error) {
	if in.RoomNum == "" {
		return nil, errors.New("missing roomNum")
	}
	ttl := time.Duration(l.svcCtx.Config.CacheRoomDetailTtl) * time.Second
	r, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "num:"+in.RoomNum, ttl, func() (fixture.RoomDetail, error) {
		rr, ok := fixture.RoomByNum(in.RoomNum)
		if !ok {
			return fixture.RoomDetail{}, errors.New("room not found")
		}
		return rr, nil
	})
	if err != nil {
		return nil, err
	}
	return &apipro.RoomDetailResp{Room: toRoomDetail(r)}, nil
}

type RoomScheduleLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRoomScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RoomScheduleLogic {
	return &RoomScheduleLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RoomScheduleLogic) RoomSchedule(in *apipro.RoomNumReq) (*apipro.RoomScheduleResp, error) {
	if in.RoomNum == "" {
		return nil, errors.New("missing roomNum")
	}
	ttl := time.Duration(l.svcCtx.Config.CacheRoomDetailTtl) * time.Second
	list, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "schedule:"+in.RoomNum, ttl, func() ([]fixture.MatchItem, error) {
		// matches where any anchor's roomNum == in.RoomNum
		var out []fixture.MatchItem
		for _, m := range fixture.AllMatches() {
			for _, a := range m.Anchors {
				if a.Anchor.RoomNum == in.RoomNum {
					out = append(out, m)
					break
				}
			}
		}
		return out, nil
	})
	if err != nil {
		return nil, err
	}
	return &apipro.RoomScheduleResp{List: toMatchItems(list)}, nil
}

type RoomRankLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewRoomRankLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RoomRankLogic {
	return &RoomRankLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RoomRankLogic) RoomRank(in *apipro.RoomNumReq) (*apipro.RoomRankResp, error) {
	if in.RoomNum == "" {
		return nil, errors.New("missing roomNum")
	}
	ttl := time.Duration(l.svcCtx.Config.CacheRoomDetailTtl) * time.Second
	list, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "rank:"+in.RoomNum, ttl, func() ([]fixture.RoomRankItem, error) {
		return fixture.RankByRoom(in.RoomNum), nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]*apipro.RoomRankItem, 0, len(list))
	for _, r := range list {
		out = append(out, &apipro.RoomRankItem{Uid: r.Uid, NickName: r.NickName, Icon: r.Icon, Score: r.Score, Rank: r.Rank})
	}
	return &apipro.RoomRankResp{List: out}, nil
}

func toRoomDetail(r fixture.RoomDetail) *apipro.RoomDetail {
	return &apipro.RoomDetail{
		RoomNum: r.RoomNum, Title: r.Title, Cover: r.Cover, Live: r.Live,
		ViewNum: r.ViewNum, LiveType: r.LiveType, Anchor: toCommentator(r.Anchor),
		StreamUrls: r.StreamUrls, Notice: r.Notice, Tags: r.Tags,
	}
}
