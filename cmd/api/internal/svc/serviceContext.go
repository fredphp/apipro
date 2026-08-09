package svc

import (
        "strings"

        "apipro/cmd/api/internal/config"
        "apipro/common/db"
        "apipro/common/model"
        "apipro/common/ratelimit"
        "apipro/pkg/codec"
        "apipro/pkg/wschat"

        "github.com/zeromicro/go-zero/core/logx"
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

        // Configure schema-name qualification BEFORE constructing models:
        //   MySQL  → qualified <prefix>schema.table (e.g. zb_user.user)
        //   SQLite → bare table names (single-file DB)
        switch strings.ToLower(strings.TrimSpace(c.DBDriver)) {
        case "sqlite", "sqlite3":
                model.SetNoSchemaPrefix()
        default:
                if p := strings.TrimSpace(c.SchemaPrefix); p != "" {
                        model.SetSchemaPrefix(p)
                }
                if p := strings.TrimSpace(c.EimSchemaPrefix); p != "" {
                        model.SetEimSchemaPrefix(p)
                }
        }

        // Codec keys — fall back to production defaults if unset.
        reqKey := []byte(c.ApiKeyReq)
        if len(reqKey) == 0 {
                reqKey = []byte(codec.DefaultRequestKey)
        }
        respKey := []byte(c.ApiKeyResp)
        if len(respKey) == 0 {
                respKey = []byte(codec.DefaultResponseKey)
        }
        // WAP response key — defaults to the request key (per spec, WAP uses
        // the SAME key for both request and response).
        wapRespKey := []byte(c.ApiKeyRespWap)
        if len(wapRespKey) == 0 {
                wapRespKey = reqKey
        }

        // AUDIT-010/011/012: when a DB DataSource is configured, pass the
        // chat-message, live-room, and user models into the WS hub so chat is
        // persisted to MySQL and room/user-status checks are enforced. When
        // unset, the hub degrades gracefully (Redis-only).
        //
        // Multi-database: use MultiDB so per-schema pools are honored when
        // the Databases map is configured. Models that do cross-schema JOINs
        // (live_room) use Shared(); single-schema models use ForSchema().
        var (
                chatMessages *model.ChatRoomMessageModel
                roomsModel   *model.LiveRoomModel
                usersModel   *model.UserModel
        )
        if c.DataSource != "" || len(c.Databases) > 0 {
                mdb, err := db.NewMultiDB(c.DBDriver, c.DataSource, c.Databases)
                if err != nil {
                        logx.Errorf("svc: db init failed (WS hub will run in Redis-only mode): %v", err)
                } else {
                        chatMessages = model.NewChatRoomMessageModel(mdb.ForSchema("chat"))
                        roomsModel = model.NewLiveRoomModel(mdb.Shared()) // cross-schema JOINs
                        usersModel = model.NewUserModel(mdb.ForSchema("user"))
                }
        }

        hub := wschat.NewHub(rdb, respKey, reqKey, c.ChatMaxMsgLen, c.ChatHistoryLim, c.ChatRatePerMin,
                chatMessages, roomsModel, usersModel)

        return &ServiceContext{
                Config:      c,
                ApiproRpc:   zrpc.MustNewClient(c.ApiproRpc),
                Redis:       rdb,
                RateLimiter: ratelimit.New(rdb, c.RateLimitPerMinute, "api"),
                AuthLimiter: ratelimit.New(rdb, c.RateLimitAuthPerMinute, "auth"),
                ChatHub:     hub,
                Transport: codec.TransportConfig{
                        RequestKey:     reqKey,
                        ResponseKey:    respKey,
                        WapResponseKey: wapRespKey,
                },
        }
}
