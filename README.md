# apipro — go-zero RPC API project (zbyy live-streaming data model)

A standalone, high-concurrency API project built with the **go-zero** microservice framework.
It exposes a JSON-over-HTTP API gateway backed by a **gRPC (zRPC) RPC service**, with every
read endpoint served from **Redis cache** and **scheduled background refresh** of hot cache keys.
All data is read from a **MySQL database** (SQLite for dev). Authentication uses the **exact same
password encryption as the zbyy client** (`md5(md5(password) + SECRET_KEY)`) so the existing
zbyy front-end can register/login without modification.

Live-room chat is delivered over **WebSocket** via a self-contained static `chat.html` widget
that you can drop into any live-room page as an `<iframe>`.

> Derived from the [zbyy](https://github.com/feibowork/zbyy) front-end project.

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
                 │  - user store (zbyy md5 password, JWT)           │
                 │  - data models → MySQL / SQLite                  │
                 └───────────────┬──────────────────────────────────┘
                                 │
                ┌────────────────┼────────────────┐
                │                │                │
    ┌───────────▼──────────┐  ┌──▼───────────┐  ┌─▼──────────────┐
    │  MySQL / SQLite       │  │  Redis       │  │  (future:      │
    │  (data source)        │  │  (:6399)     │  │   more caches)  │
    │  users, matches,      │  │  cache /     │  └────────────────┘
    │  rooms, anchors,      │  │  chat hist / │
    │  live_types, ranks    │  │  rate limit   │
    └───────────────────────┘  └──────────────┘
```

## Authentication (zbyy-compatible)

The zbyy front-end encrypts the password **before** sending it to the server
(`src/js/utils/common.js → md5Pwd`):

```
md5Pwd(password, pwdType):
  pwdType == 2  →  md5(password)
  pwdType == 1  →  md5( md5(password.toLowerCase()) + SECRET_KEY )
                    where SECRET_KEY = "&%*$8@!!%"
```

The server **stores the client-encrypted string as-is** and compares it directly on login.
This means the zbyy client can register/login against this API **without any modification**.

- `common/auth/pwd.go` — replicates the exact algorithm (for server-side convenience / admin tools)
- `common/store/user.go` — Register stores the client-sent hash; Login compares directly
- `common/model/user_model.go` — MySQL/SQLite CRUD for the `users` table

Demo user (seed): phone=`13800138000`, countryCode=`+86`, password=`123456`
→ stored hash = `md5(md5("123456") + "&%*$8@!!%")` = `2e36f5fa46a866a6e91b71524dd8d155`

## Endpoints

All read endpoints are **Redis-cached** with per-family TTL and **proactively refreshed** by a
background scheduler (match every 60s, live every 15s, commentator every 120s).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/api/v1/auth/register` | – | Register `{loginName,phone,countryCode,password,smsCode,nickName,pwdType}` (smsCode=`1234` in dev) |
| POST | `/api/v1/auth/login` | – | Login `{phone,countryCode,password,pwdType}` |
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

## Database (data source)

All API data comes from a **MySQL database** (production) or **SQLite** (dev/self-check).
The schema is derived from the zbyy data model:

| Table | Description |
|-------|-------------|
| `users` | Registered users + guests (password = client md5 hash) |
| `anchors` | Commentators / anchors (解说员) |
| `rooms` | Live rooms (直播间) |
| `matches` | Match schedule (赛程) |
| `match_anchors` | Match ↔ anchor many-to-many |
| `live_types` | Live type categories (足球/篮球/斯诺克/其它) |
| `room_ranks` | Room gift leaderboards (排行榜) |

### Schema files
- `deploy/schema.mysql.sql` — MySQL schema + seed data
- `deploy/schema.sqlite.sql` — SQLite schema (dev)

### Configuration

`cmd/rpc/etc/apipro.yaml`:
```yaml
# Database (数据来源)
DBDriver: sqlite                          # mysql | sqlite
DataSource: ./data/apipro.db              # MySQL: user:pass@tcp(host:3306)/apipro?charset=utf8mb4&parseTime=true&loc=Local

# Redis (缓存 + 定时刷新)
CacheRedis:
  Host: 127.0.0.1:6399
  Type: node
```

### Seed the database

```bash
# Build the seed tool
go build -o bin/apipro-seed ./cmd/seed

# Seed (auto-creates SQLite tables; for MySQL run schema.mysql.sql first)
./bin/apipro-seed -config cmd/rpc/etc/apipro.yaml
```

## Cache & scheduled refresh

- Every read RPC method uses `cache.GetOrLoad`: Redis hit → return; miss → query DB → write back with TTL.
- A `cache.Scheduler` runs 3 background jobs that **proactively refresh** hot keys on a ticker,
  so the cache stays warm and TTL expiry never causes a thundering herd.
- `RefreshCache` / `CacheStats` admin RPCs (JWT-protected) let you force-invalidate and observe.

TTLs and refresh intervals are configurable in `cmd/rpc/etc/apipro.yaml`.

## Security

- **zbyy-compatible password encryption**: server stores the client-sent `md5(md5(pw)+secret)` hash
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
go build -o bin/apipro-seed ./cmd/seed

# 3. seed the database (creates data/apipro.db for SQLite)
./bin/apipro-seed -config cmd/rpc/etc/apipro.yaml

# 4. run rpc
./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml

# 5. run api (in another shell)
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
│   ├── rpc/            # gRPC service (go-zero zRPC)
│   │   ├── apipro.go
│   │   ├── apiproClient/   # generated client
│   │   ├── etc/apipro.yaml
│   │   └── internal/{config,svc,server,logic}
│   └── seed/           # DB seed tool
├── common/
│   ├── auth/           # zbyy-compatible md5 password (pwd.go)
│   ├── cache/          # read-through Redis cache + scheduler
│   ├── ctxdata/        # JWT uid extraction
│   ├── db/             # database/sql connection (MySQL + SQLite)
│   ├── jwtx/           # JWT sign/verify
│   ├── model/          # DB models (user, anchor, room, match)
│   ├── ratelimit/      # sliding-window per-IP limiter
│   └── store/          # user store (MySQL-backed, zbyy md5)
├── deploy/
│   ├── schema.mysql.sql    # MySQL schema + seed data
│   └── schema.sqlite.sql   # SQLite schema (dev)
├── pkg/
│   ├── fixture/        # zbyy data-model structs + helpers
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
protoc --proto_path=desc/proto --go_out=desc/proto/gen/apipro --go_opt=paths=source_relative \
  --go-grpc_out=desc/proto/gen/apipro --go-grpc_opt=paths=source_relative desc/proto/apipro.proto

# regenerate API from .api (then remove duplicate stub logic files)
goctl api go --api desc/api/apipro.api --dir cmd/api --style=goZero
```
