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
        DBDriver   string `json:",default=mysql"` // mysql | sqlite
        DataSource string `json:",optional"`      // DSN

        // SchemaPrefix is the MySQL schema-name prefix used by all qualified
        // table references (e.g. "zb_" → zb_user.user, zb_live.live_room).
        // Mirrors backend-zero's haima_* layout but with a customizable prefix.
        // Only meaningful for MySQL; SQLite ignores this (single-file DB).
        SchemaPrefix string `json:",default=zb_"`

        // Transport encryption keys (16-byte ASCII). Loaded from env in production.
        ApiKeyReq  string `json:",optional"`
        ApiKeyResp string `json:",optional"`

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
