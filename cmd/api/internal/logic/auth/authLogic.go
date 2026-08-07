package auth

import (
	"context"

	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"apipro/cmd/rpc/apiproClient"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RegisterLogic) Register(req *types.RegisterReq) (resp *types.TokenResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.Register(l.ctx, &apiproClient.RegisterReq{
		LoginName: req.LoginName, Phone: req.Phone, CountryCode: req.CountryCode,
		Password: req.Password, SmsCode: req.SmsCode,
	})
	if err != nil {
		return nil, err
	}
	return &types.TokenResp{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresAt: out.ExpiresAt}, nil
}

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.TokenResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.Login(l.ctx, &apiproClient.LoginReq{
		Phone: req.Phone, CountryCode: req.CountryCode, Password: req.Password,
	})
	if err != nil {
		return nil, err
	}
	return &types.TokenResp{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresAt: out.ExpiresAt}, nil
}

type GuestLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGuestLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GuestLogic {
	return &GuestLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *GuestLogic) Guest() (resp *types.TokenResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.GuestLogin(l.ctx, &apiproClient.GuestReq{})
	if err != nil {
		return nil, err
	}
	return &types.TokenResp{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresAt: out.ExpiresAt}, nil
}

type RefreshLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRefreshLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshLogic {
	return &RefreshLogic{Logger: logx.WithContext(ctx), ctx: ctx, svcCtx: svcCtx}
}

func (l *RefreshLogic) Refresh(req *types.RefreshReq) (resp *types.TokenResp, err error) {
	cli := apiproClient.NewApipro(l.svcCtx.ApiproRpc)
	out, err := cli.RefreshToken(l.ctx, &apiproClient.RefreshReq{RefreshToken: req.RefreshToken})
	if err != nil {
		return nil, err
	}
	return &types.TokenResp{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresAt: out.ExpiresAt}, nil
}
