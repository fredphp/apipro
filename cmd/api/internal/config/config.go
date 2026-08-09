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

        // SchemaPrefix is the MySQL schema-name prefix used by all qualified
        // table references (e.g. "zb_" → zb_user.user, zb_live.live_room).
        // Only meaningful for MySQL; SQLite ignores this (single-file DB).
        SchemaPrefix string `json:",default=zb_"`

        // Transport encryption keys — same as RPC.
        // Per docs/password-login-register.txt:
        //   Web (plat=3): ApiKeyReq for request, ApiKeyResp for response
        //   WAP (plat=4): ApiKeyReq for request, ApiKeyRespWap for response
        //                 (WAP uses the SAME key for req+resp)
        ApiKeyReq    string `json:",optional"`
        ApiKeyResp   string `json:",optional"`
        ApiKeyRespWap string `json:",optional"` // WAP response key; defaults to ApiKeyReq when empty

        // App mode: "dev" | "prod" (controls SMS dev bypass).
        // (Mode is inherited from RestConf — use that for go-zero's service mode.)
        AppMode string `json:",default=dev"`

        // SMS dev bypass code
        SmsDevBypassCode string `json:",optional"`

        // Kaptcha (image captcha) config — /api/kaptcha
        KaptchaCodeLen int `json:",default=5"`  // number of chars in the code
        KaptchaTTL     int `json:",default=300"` // Redis TTL in seconds

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
