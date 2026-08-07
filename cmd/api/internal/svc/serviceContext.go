package svc

import (
	"apipro/cmd/api/internal/config"
	"apipro/common/ratelimit"
	"apipro/pkg/wschat"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config      config.Config
	ApiproRpc   zrpc.Client
	Redis       *redis.Redis
	RateLimiter *ratelimit.Limiter
	AuthLimiter *ratelimit.Limiter
	ChatHub     *wschat.Hub
}

func NewServiceContext(c config.Config) *ServiceContext {
	rdb := redis.MustNewRedis(c.Redis)
	return &ServiceContext{
		Config:      c,
		ApiproRpc:   zrpc.MustNewClient(c.ApiproRpc),
		Redis:       rdb,
		RateLimiter: ratelimit.New(rdb, c.RateLimitPerMinute, "api"),
		AuthLimiter: ratelimit.New(rdb, c.RateLimitAuthPerMinute, "auth"),
		ChatHub: wschat.NewHub(rdb, c.Auth.AccessSecret, c.ChatMaxMsgLen, c.ChatHistoryLim, c.ChatRatePerMin),
	}
}
