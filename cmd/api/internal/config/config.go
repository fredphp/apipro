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
        //
        // MULTI-DATABASE MODE (MySQL):
        //   Production uses a multi-database layout (zb_user / zb_live / zb_chat
        //   / zb_gift / zb_admin / zb_sys / zb_basketball / zb_football, plus
        //   eim_user / eim_friend / eim_group / eim_admin for IM). Two ways to
        //   configure:
        //
        //   1. SHARED-POOL MODE (default, recommended):
        //        DataSource = "root:pass@tcp(host:port)/?charset=..."  (empty path!)
        //        Databases  = {}  (or omitted)
        //      A single connection pool with NO pinned database. All schemas
        //      are accessed via fully-qualified names (zb_user.user, etc.)
        //      and cross-schema JOINs work transparently.
        //
        //   2. PER-SCHEMA-POOL MODE (optional):
        //        DataSource = "root:pass@tcp(host:port)/?charset=..."  (shared, for JOINs)
        //        Databases:
        //          user:       "root:pass@tcp(host:port)/zb_user?charset=..."
        //          live:       "root:pass@tcp(host:port)/zb_live?charset=..."
        //          chat:       "root:pass@tcp(host:port)/zb_chat?charset=..."
        //          eim_user:   "root:pass@tcp(host:port)/eim_user?charset=..."
        //          ...
        //      Each schema group gets its own pool pinned to that database.
        //      Models that do cross-schema JOINs still use the shared pool
        //      (DataSource); single-schema models use their per-schema pool.
        //
        //   When DataSource is empty AND Databases is non-empty, the first
        //   Databases entry is used as the shared pool (cross-schema JOINs
        //   will fail in this case — only use when you don't need JOINs).
        DBDriver   string `json:",default=mysql"`
        DataSource string `json:",optional"`

        // Databases provides per-schema-group DSNs for explicit multi-database
        // configuration. Keys are schema short-names: "user", "live", "chat",
        // "gift", "admin", "sys", "basketball", "football", "eim_user",
        // "eim_friend", "eim_group", "eim_admin" (or canonical "eim:user" etc.).
        // When a schema is listed here, it gets its own connection pool.
        // When a schema is NOT listed, it uses the shared DataSource.
        // See the DBDriver/DataSource comment above for the full explanation.
        Databases map[string]string `json:",optional"`

        // SchemaPrefix is the MySQL schema-name prefix used by all qualified
        // table references for the main app schemas (e.g. "zb_" → zb_user.user,
        // zb_live.live_room). Only meaningful for MySQL; SQLite ignores this.
        SchemaPrefix string `json:",default=zb_"`

        // EimSchemaPrefix is the MySQL schema-name prefix for IM-related
        // schemas (e.g. "eim_" → eim_user, eim_friend, eim_group, eim_admin).
        // Only meaningful for MySQL; SQLite ignores this.
        EimSchemaPrefix string `json:",default=eim_"`

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
