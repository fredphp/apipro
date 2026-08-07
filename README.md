# apipro — go-zero RPC API project (zbyy live-streaming data model)

A standalone, high-concurrency API project built with the **go-zero** microservice framework.
It exposes a JSON-over-HTTP API gateway backed by a **gRPC (zRPC) RPC service**, with every
read endpoint served from **Redis cache** and **scheduled background refresh** of hot cache keys.
Live-room chat is delivered over **WebSocket** via a self-contained static `chat.html` widget
that you can drop into any live-room page as an `<iframe>`.

> Derived from the [zbyy](https://github.com/feibowork/zbyy) front-end project. The zbyy repo is
> a pure front-end (jQuery + webpack) that talks to an encrypted protobuf backend; this project
> reproduces that backend's data model as a stand-alone, cache-first API so the zbyy front-end
> (or any client) can consume it without the original closed backend.

## Architecture

```
                 ┌──────────────────────────────────────────────────┐
   HTTP/WS ──►   │  apipro-api  (go-zero REST, :3100)                │
  (gateway)      │  - REST routes → call RPC                         │
                 │  - JWT auth (go-zero built-in)                    │
                 │  - per-IP rate limit (Redis ZSET sliding window)  │
                 │  - /ws/chat  (WebSocket chat hub)                 │
                 │  - /chat.html (static embeddable widget)          │
                 │  - /health                                         │
                 └───────────────┬──────────────────────────────────┘
                                 │ gRPC (zRPC, direct :3101)
                 ┌───────────────▼──────────────────────────────────┐
                 │  apipro-rpc  (go-zero zRPC/gRPC, :3101)          │
                 │  - all business logic                             │
                 │  - Redis cache (read-through + TTL)              │
                 │  - scheduled cache refresh (3 jobs)              │
                 │  - user store (bcrypt, JWT)                      │
                 └───────────────┬──────────────────────────────────┘
                                 │ RESP2
                 ┌───────────────▼──────────────────────────────────┐
                 │  Redis  (:6399, internal)                        │
                 │  cache / users / chat history / rate limit        │
                 └──────────────────────────────────────────────────┘
```

**RPC mode**: the core service is a real gRPC server (`apipro.rpc`), generated from
`desc/proto/apipro.proto`. The HTTP gateway is a thin adapter that calls the RPC client.
You can equally call the gRPC server directly from any gRPC client (reflection enabled in dev mode).

## Endpoints

All read endpoints are **Redis-cached** with per-family TTL and **proactively refreshed** by a
background scheduler (match every 60s, live every 15s, commentator every 120s).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/register` | – | Register `{loginName,phone,countryCode,password,smsCode}` (smsCode=`1234` in dev) |
| POST | `/api/v1/auth/login` | – | Login `{phone,countryCode,password}` |
| POST | `/api/v1/auth/guest` | – | Guest token (anonymous) |
| POST | `/api/v1/auth/refresh` | – | Refresh access token `{refreshToken}` |
| GET | `/api/v1/user/profile` | JWT | Current user profile |
| GET | `/api/v1/match/list?date=YYYYMMDD&page=&pageSize=` | – | **比赛列表** (date optional, defaults to today) |
| GET | `/api/v1/match/recommend` | – | Recommended matches |
| GET | `/api/v1/match/detail?id=` | – | Match detail |
| GET | `/api/v1/match/cates` | – | Match category names |
| GET | `/api/v1/match/cate?cateName=&page=&pageSize=` | – | Matches by category |
| GET | `/api/v1/room/detail?roomNum=` | – | **Room ID 详情** |
| GET | `/api/v1/room/schedule?roomNum=` | – | Room anchor schedule |
| GET | `/api/v1/room/rank?roomNum=` | – | Room gift rank |
| GET | `/api/v1/commentator/list?page=&pageSize=` | – | **解说员信息** list |
| GET | `/api/v1/commentator/detail?id=` | – | Commentator detail |
| GET | `/api/v1/live/list?page=&pageSize=` | – | **直播比赛** list |
| GET | `/api/v1/live/types` | – | Live types |
| GET | `/api/v1/live/hot` | – | Hot anchors |
| POST | `/api/v1/admin/cache/refresh` | JWT | Force-refresh a cache family `{family}` |
| GET | `/api/v1/admin/cache/stats` | JWT | Cache hit/miss/key stats |
| GET | `/chat.html` | – | Static embeddable chat widget |
| GET | `/ws/chat?token=<jwt>&roomNum=<n>` | – | WebSocket chat (token optional → guest) |
| GET | `/health` | – | Health check |

## Chat (WebSocket)

Chat is NOT an HTTP API — it runs over WebSocket and the UI is a static HTML page
(`public/chat.html`) that you embed into any live-room page:

```html
<iframe src="https://your-host/chat.html?roomNum=1001&token=USER_JWT"
        style="width:100%;height:100%;border:0"></iframe>
```

The widget auto-connects, loads recent history, handles reconnects, sanitizes content (XSS-safe),
supports danmu toggle, and enforces a per-user send rate. Messages are broadcast to all room
members and the last 50 are kept in Redis.

## Cache & scheduled refresh

- Every read RPC method uses `cache.GetOrLoad`: Redis hit → return; miss → compute from the
  fixture data source → write back with TTL.
- A `cache.Scheduler` runs 3 background jobs that **proactively refresh** hot keys on a ticker,
  so the cache stays warm and TTL expiry never causes a thundering herd.
- `RefreshCache` / `CacheStats` admin RPCs (JWT-protected) let you force-invalidate and observe.

TTLs and refresh intervals are configurable in `cmd/rpc/etc/apipro.yaml`.

## Data layer

The zbyy repo contains **no backend code** (only front-ends). This project therefore ships an
in-memory fixture (`pkg/fixture`) that mirrors the zbyy data model exactly (`MatchItem`,
`RoomDetail`, `Commentator`, `LiveRoom`, `UserInfo`, …). Users are persisted in Redis (bcrypt
password hash stored in a separate key, never serialized in the public record).

> **If you need a SQL database** (e.g. to back the fixtures with real match/anchor data from a
> CMS), tell me and I'll add a `model` layer (go-zero `sqlx`/`gorm`) — the RPC/cache contract
> stays identical. For v1, Redis is the only data store.

## Security

- bcrypt password hashing (cost 10)
- HS256 JWT access (15 min) + refresh (7 days) tokens, typ-claim separated
- per-IP sliding-window rate limit (Redis ZSET), separate stricter limit on auth endpoints
- XSS-escaped chat content, max message length, alphanumeric room ids
- WebSocket auth via `?token=<jwt>` (guest allowed)
- security headers: `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, CORS configurable
- zero hard-coded secrets in code (read from config); JWT secret must be ≥32 chars

## Build & run

Requirements: Go 1.24+, `goctl` v1.7+, `protoc` + plugins, Redis.

```bash
# 1. start redis
redis-server --port 6399 --daemonize yes --save "" --appendonly no

# 2. build
go build -o bin/apipro-rpc ./cmd/rpc
go build -o bin/apipro-api ./cmd/api

# 3. run rpc
./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml

# 4. run api (in another shell)
./bin/apipro-api -f cmd/api/etc/apipro.yaml
```

Or via the sandbox gateway: `https://preview-host/api/v1/match/list?XTransformPort=3100`.

## Project layout

```
apipro/
├── cmd/
│   ├── api/            # HTTP+WS gateway (go-zero REST)
│   │   ├── apipro.go
│   │   ├── etc/apipro.yaml
│   │   └── internal/{config,svc,handler,logic,types,conv}
│   └── rpc/            # gRPC service (go-zero zRPC)
│       ├── apipro.go
│       ├── apiproClient/   # generated client
│       ├── etc/apipro.yaml
│       └── internal/{config,svc,server,logic}
├── common/
│   ├── cache/          # read-through Redis cache + scheduler
│   ├── ctxdata/        # JWT uid extraction
│   ├── jwtx/           # JWT sign/verify
│   ├── ratelimit/      # sliding-window per-IP limiter
│   └── store/          # Redis user store (bcrypt)
├── pkg/
│   ├── fixture/        # zbyy data-model fixtures
│   └── wschat/         # WebSocket chat hub
├── desc/
│   ├── api/apipro.api      # go-zero REST DSL
│   └── proto/
│       ├── apipro.proto    # gRPC service contract
│       └── gen/            # generated pb code
└── public/chat.html    # static embeddable chat widget
```

## Regenerate code

```bash
# regenerate RPC from proto
goctl rpc protoc desc/proto/apipro.proto --go_out=desc/proto/gen --go-grpc_out=desc/proto/gen \
  --zrpc_out=cmd/rpc --style=goZero

# regenerate API from .api
goctl api go --api desc/api/apipro.api --dir cmd/api --style=goZero
```
