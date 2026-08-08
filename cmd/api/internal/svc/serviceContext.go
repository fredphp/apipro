package svc

import (
	"apipro/cmd/api/internal/config"
	"apipro/common/ratelimit"
	"apipro/pkg/codec"
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

	// Codec keys for the encrypted transport middleware.
	Transport codec.TransportConfig
}

func NewServiceContext(c config.Config) *ServiceContext {
	rdb := redis.MustNewRedis(c.Redis)

	// Codec keys — fall back to production defaults if unset.
	reqKey := []byte(c.ApiKeyReq)
	if len(reqKey) == 0 {
		reqKey = []byte(codec.DefaultRequestKey)
	}
	respKey := []byte(c.ApiKeyResp)
	if len(respKey) == 0 {
		respKey = []byte(codec.DefaultResponseKey)
	}

	hub := wschat.NewHub(rdb, respKey, reqKey, c.ChatMaxMsgLen, c.ChatHistoryLim, c.ChatRatePerMin)

	return &ServiceContext{
		Config:      c,
		ApiproRpc:   zrpc.MustNewClient(c.ApiproRpc),
		Redis:       rdb,
		RateLimiter: ratelimit.New(rdb, c.RateLimitPerMinute, "api"),
		AuthLimiter: ratelimit.New(rdb, c.RateLimitAuthPerMinute, "auth"),
		ChatHub:     hub,
		Transport: codec.TransportConfig{
			RequestKey:  reqKey,
			ResponseKey: respKey,
		},
	}
}
