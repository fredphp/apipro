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
        DBDriver   string `json:",default=mysql"` // mysql | sqlite
        DataSource string `json:",optional"`      // shared DSN (empty path for cross-schema)

        // Databases provides per-schema-group DSNs for explicit multi-database
        // configuration. Keys are schema short-names: "user", "live", "chat",
        // "gift", "admin", "sys", "basketball", "football", "eim_user",
        // "eim_friend", "eim_group", "eim_admin" (or canonical "eim:user" etc.).
        // When a schema is listed here, it gets its own connection pool.
        // When a schema is NOT listed, it uses the shared DataSource.
        Databases map[string]string `json:",optional"`

        // SchemaPrefix is the MySQL schema-name prefix used by all qualified
        // table references for the main app schemas (e.g. "zb_" → zb_user.user,
        // zb_live.live_room). Mirrors backend-zero's haima_* layout but with
        // a customizable prefix. Only meaningful for MySQL.
        SchemaPrefix string `json:",default=zb_"`

        // EimSchemaPrefix is the MySQL schema-name prefix for IM-related
        // schemas (e.g. "eim_" → eim_user, eim_friend, eim_group, eim_admin).
        // Only meaningful for MySQL.
        EimSchemaPrefix string `json:",default=eim_"`

        // Transport encryption keys (16-byte ASCII). Loaded from env in production.
        // Per docs/password-login-register.txt:
        //   Web (plat=3): ApiKeyReq for request, ApiKeyResp for response
        //   WAP (plat=4): ApiKeyReq for request, ApiKeyRespWap for response
        //                 (WAP uses the SAME key for req+resp)
        ApiKeyReq     string `json:",optional"`
        ApiKeyResp    string `json:",optional"`
        ApiKeyRespWap string `json:",optional"` // WAP response key; defaults to ApiKeyReq when empty

        // App environment: "dev" | "prod" (controls SMS dev bypass).
        // (Mode is inherited from RpcServerConf — use that for go-zero's service mode.)
        AppMode string `json:",default=dev"`

        // SMS dev bypass code (only honored when AppMode=dev).
        SmsDevBypassCode string `json:",optional"`

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

        // JSONP snapshot directory (writes matches.json, all_live_rooms.json, etc.)
        JsonpSnapshotDir string `json:",optional"`

        // File base URL for asset path prefixing
        FileBaseURL string `json:",optional"`

        // Legacy (kept for backwards compat; not used by the new auth flow).
        JwtSecret  string         `json:",optional"`
        AccessTtl  time.Duration  `json:",optional"`
        RefreshTtl time.Duration  `json:",optional"`
}
