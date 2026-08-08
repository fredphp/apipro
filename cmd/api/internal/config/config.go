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

        // Redis for rate limit + chat history + session store
        Redis redis.RedisConf

        // Database for WS chat persistence + room/user lookups (AUDIT-010/011/012).
        // Optional — when empty, the WS hub degrades to Redis-only mode.
        DBDriver   string `json:",default=mysql"`
        DataSource string `json:",optional"`

        // Transport encryption keys — same as RPC.
        ApiKeyReq  string `json:",optional"`
        ApiKeyResp string `json:",optional"`

        // App mode: "dev" | "prod" (controls SMS dev bypass).
        // (Mode is inherited from RestConf — use that for go-zero's service mode.)
        AppMode string `json:",default=dev"`

        // SMS dev bypass code
        SmsDevBypassCode string `json:",optional"`

        // JSONP snapshot directory
        JsonpSnapshotDir string `json:",optional"`

        // File base URL for asset path prefixing
        FileBaseURL string `json:",optional"`

        // Rate limits
        RateLimitPerMinute     int `json:",default=120"`
        RateLimitAuthPerMinute int `json:",default=20"`

        // Chat
        ChatMaxMsgLen  int `json:",default=500"`
        ChatHistoryLim int `json:",default=50"`
        ChatRatePerMin int `json:",default=60"`

        // CORS
        CorsOrigin string `json:",optional"`

        // Legacy (kept for backwards compat; not used by the new auth flow).
        AccessTtl  time.Duration `json:",optional"`
        RefreshTtl time.Duration `json:",optional"`
        Auth       struct {
                AccessSecret  string
                AccessExpire  int64
                RefreshExpire int64
        } `json:",optional"`
}
