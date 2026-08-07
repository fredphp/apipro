package config

import (
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	rest.RestConf

	// Internal RPC client (direct, no etcd)
	ApiproRpc zrpc.RpcClientConf

	// Redis for rate limit + chat history + (optional) shared cache
	Redis redis.RedisConf

	Auth struct {
		AccessSecret string
		AccessExpire int64
		RefreshExpire int64
	}

	// Rate limits
	RateLimitPerMinute     int `json:",default=120"`
	RateLimitAuthPerMinute int `json:",default=20"`

	// Chat
	ChatMaxMsgLen  int `json:",default=500"`
	ChatHistoryLim int `json:",default=50"`
	ChatRatePerMin int `json:",default=60"`

	// CORS
	CorsOrigin string `json:",optional"`

	// AccessExpire/RefreshExpire as durations for token issuance passthrough
	AccessTtl  time.Duration `json:",optional"`
	RefreshTtl time.Duration `json:",optional"`
}
