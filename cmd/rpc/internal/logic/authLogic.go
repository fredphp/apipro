package logic

import (
        "context"
        "errors"
        "strings"
        "time"

        "apipro/cmd/rpc/internal/svc"
        "apipro/common/cache"
        "apipro/common/jwtx"
        "apipro/common/store"
        "apipro/desc/proto/gen/apipro"
        "apipro/pkg/fixture"

        "github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
        return &RegisterLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RegisterLogic) Register(in *apipro.RegisterReq) (*apipro.TokenResp, error) {
        in.LoginName = strings.TrimSpace(in.LoginName)
        in.Phone = strings.TrimSpace(in.Phone)
        in.CountryCode = strings.TrimSpace(in.CountryCode)
        if in.LoginName == "" || len(in.LoginName) > 32 {
                return nil, errors.New("invalid loginName")
        }
        if in.Phone == "" || len(in.Phone) > 20 {
                return nil, errors.New("invalid phone")
        }
        if in.Password == "" || len(in.Password) < 6 || len(in.Password) > 64 {
                return nil, errors.New("password length must be 6-64")
        }
        // smsCode check: accept "1234" in dev; in production wire a real SMS provider
        if in.SmsCode != "1234" {
                return nil, errors.New("invalid sms code")
        }
        rec, err := l.svcCtx.Users.Register(l.ctx, in.LoginName, in.CountryCode, in.Phone, in.Password)
        if err != nil {
                if errors.Is(err, store.ErrUserExists) {
                        return nil, errors.New("phone or loginName already registered")
                }
                return nil, err
        }
        access, exp, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypAccess, l.svcCtx.Config.AccessTtl)
        if err != nil {
                return nil, err
        }
        refresh, _, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypRefresh, l.svcCtx.Config.RefreshTtl)
        if err != nil {
                return nil, err
        }
        l.Infof("register ok uid=%s", rec.Uid)
        return &apipro.TokenResp{AccessToken: access, RefreshToken: refresh, ExpiresAt: exp}, nil
}

type LoginLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
        return &LoginLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *LoginLogic) Login(in *apipro.LoginReq) (*apipro.TokenResp, error) {
        in.Phone = strings.TrimSpace(in.Phone)
        in.CountryCode = strings.TrimSpace(in.CountryCode)
        if in.Phone == "" || in.Password == "" {
                return nil, errors.New("missing phone or password")
        }
        rec, err := l.svcCtx.Users.Login(l.ctx, in.CountryCode, in.Phone, in.Password)
        if err != nil {
                if errors.Is(err, store.ErrUserNotFound) || errors.Is(err, store.ErrInvalidPassword) {
                        return nil, errors.New("invalid credentials")
                }
                return nil, err
        }
        access, exp, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypAccess, l.svcCtx.Config.AccessTtl)
        if err != nil {
                return nil, err
        }
        refresh, _, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypRefresh, l.svcCtx.Config.RefreshTtl)
        if err != nil {
                return nil, err
        }
        return &apipro.TokenResp{AccessToken: access, RefreshToken: refresh, ExpiresAt: exp}, nil
}

type GuestLoginLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewGuestLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GuestLoginLogic {
        return &GuestLoginLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GuestLoginLogic) GuestLogin(in *apipro.GuestReq) (*apipro.TokenResp, error) {
        rec, err := l.svcCtx.Users.CreateGuest(l.ctx)
        if err != nil {
                return nil, err
        }
        access, exp, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypAccess, l.svcCtx.Config.AccessTtl)
        if err != nil {
                return nil, err
        }
        refresh, _, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypRefresh, l.svcCtx.Config.RefreshTtl)
        if err != nil {
                return nil, err
        }
        return &apipro.TokenResp{AccessToken: access, RefreshToken: refresh, ExpiresAt: exp}, nil
}

type RefreshTokenLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewRefreshTokenLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RefreshTokenLogic {
        return &RefreshTokenLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *RefreshTokenLogic) RefreshToken(in *apipro.RefreshReq) (*apipro.TokenResp, error) {
        if in.RefreshToken == "" {
                return nil, errors.New("missing refresh token")
        }
        c, err := jwtx.Verify(l.svcCtx.Config.JwtSecret, in.RefreshToken)
        if err != nil {
                return nil, errors.New("invalid refresh token")
        }
        if c.Typ != jwtx.TypRefresh {
                return nil, errors.New("not a refresh token")
        }
        rec, err := l.svcCtx.Users.GetByUid(l.ctx, c.Uid)
        if err != nil {
                return nil, errors.New("user not found")
        }
        access, exp, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypAccess, l.svcCtx.Config.AccessTtl)
        if err != nil {
                return nil, err
        }
        refresh, _, err := jwtx.Sign(l.svcCtx.Config.JwtSecret, rec.Uid, rec.NickName, rec.IsUser, jwtx.TypRefresh, l.svcCtx.Config.RefreshTtl)
        if err != nil {
                return nil, err
        }
        return &apipro.TokenResp{AccessToken: access, RefreshToken: refresh, ExpiresAt: exp}, nil
}

type GetUserProfileLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
        logx.Logger
}

func NewGetUserProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserProfileLogic {
        return &GetUserProfileLogic{ctx: ctx, svcCtx: svcCtx, Logger: logx.WithContext(ctx)}
}

func (l *GetUserProfileLogic) GetUserProfile(in *apipro.UidReq) (*apipro.UserProfileResp, error) {
        if in.Uid == "" {
                return nil, errors.New("missing uid")
        }
        ttl := time.Duration(l.svcCtx.Config.CacheUserProfileTtl) * time.Second
        rec, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "user", "profile:"+in.Uid, ttl, func() (fixture.UserRecord, error) {
                r, e := l.svcCtx.Users.GetByUid(l.ctx, in.Uid)
                if e != nil {
                        return fixture.UserRecord{}, e
                }
                return r, nil
        })
        if err != nil {
                return nil, err
        }
        return &apipro.UserProfileResp{User: toUserInfo(rec)}, nil
}

func toUserInfo(r fixture.UserRecord) *apipro.UserInfo {
        return &apipro.UserInfo{
                Uid: r.Uid, LoginName: r.LoginName, NickName: r.NickName,
                Phone: r.Phone, CountryCode: r.CountryCode,
                Grow: r.Grow, Score: r.Score, Level: r.Level,
                Avatar: r.Avatar, IsUser: r.IsUser, CreatedAt: r.CreatedAt,
        }
}
