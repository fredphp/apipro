package config

import (
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	CacheRedis redis.RedisConf

	// Database (MySQL for production; sqlite for dev/self-check)
	DBDriver   string `json:",default=mysql"`        // mysql | sqlite
	DataSource string `json:",optional"`             // DSN

	JwtSecret  string
	AccessTtl  time.Duration
	RefreshTtl time.Duration

	// Cache TTLs (seconds)
	CacheMatchListTtl   int `json:",default=60"`
	CacheMatchDetailTtl int `json:",default=90"`
	CacheRoomDetailTtl  int `json:",default=30"`
	CacheCommentatorTtl int `json:",default=120"`
	CacheLiveTtl        int `json:",default=15"`
	CacheUserProfileTtl int `json:",default=120"`

	// Scheduled refresh intervals (seconds)
	RefreshMatchListTtl   int `json:",default=60"`
	RefreshLiveTtl        int `json:",default=15"`
	RefreshCommentatorTtl int `json:",default=120"`

	// chat
	ChatMaxMsgLen  int `json:",default=500"`
	ChatHistoryLim int `json:",default=50"`
	ChatRatePerMin int `json:",default=60"`
}
