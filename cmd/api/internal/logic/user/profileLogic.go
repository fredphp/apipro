package user

import (
	"context"

	"apipro/cmd/api/internal/conv"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"
	"apipro/common/ctxdata"

	"github.com/zeromicro/go-zero/core/logx"
)

type ProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProfileLogic {
	return &ProfileLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *ProfileLogic) Profile() (resp *types.UserProfileResp, err error) {
	uid, err := ctxdata.UidFromCtx(l.ctx)
	if err != nil {
		return nil, err
	}
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.GetUserProfile(l.ctx, &apiproClient.UidReq{Uid: uid})
	if err != nil {
		return nil, err
	}
	r := conv.UserProfile(out.User)
	return &r, nil
}
