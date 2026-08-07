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

type CommentatorListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCommentatorListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CommentatorListLogic {
	return &CommentatorListLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CommentatorListLogic) CommentatorList(in *apipro.PageReq) (*apipro.CommentatorListResp, error) {
	page, pageSize := fixture.ParsePage(in.Page, in.PageSize)
	ttl := time.Duration(l.svcCtx.Config.CacheCommentatorTtl) * time.Second
	all, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "commentator", "list", ttl, func() ([]fixture.Commentator, error) {
		return fixture.Commentators(), nil
	})
	if err != nil {
		return nil, err
	}
	total := int64(len(all))
	start, end := pageBounds(page, pageSize, len(all))
	return &apipro.CommentatorListResp{List: toCommentators(all[start:end]), Total: total}, nil
}

type CommentatorDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCommentatorDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CommentatorDetailLogic {
	return &CommentatorDetailLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *CommentatorDetailLogic) CommentatorDetail(in *apipro.IdReq) (*apipro.CommentatorDetailResp, error) {
	if in.Id == "" {
		return nil, errors.New("missing id")
	}
	ttl := time.Duration(l.svcCtx.Config.CacheCommentatorTtl) * time.Second
	c, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "commentator", "uid:"+in.Id, ttl, func() (fixture.Commentator, error) {
		cc, ok := fixture.CommentatorByID(in.Id)
		if !ok {
			return fixture.Commentator{}, errors.New("commentator not found")
		}
		return cc, nil
	})
	if err != nil {
		return nil, err
	}
	return &apipro.CommentatorDetailResp{Commentator: toCommentator(c)}, nil
}

func toCommentator(c fixture.Commentator) *apipro.Commentator {
	return &apipro.Commentator{
		Uid: c.Uid, NickName: c.NickName, Icon: c.Icon, CutOutIcon: c.CutOutIcon,
		Intro: c.Intro, Fans: c.Fans, Follow: c.Follow, Hot: c.Hot,
		Anchor: &apipro.Anchor{
			Uid: c.Anchor.Uid, RoomNum: c.Anchor.RoomNum,
			Detail: c.Anchor.Detail, Notice: c.Anchor.Notice, Live: c.Anchor.Live,
		},
	}
}

func toCommentators(cs []fixture.Commentator) []*apipro.Commentator {
	out := make([]*apipro.Commentator, 0, len(cs))
	for _, c := range cs {
		out = append(out, toCommentator(c))
	}
	return out
}
