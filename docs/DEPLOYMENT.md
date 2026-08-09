# apipro 部署及运行步骤

> 适用版本：HEAD `f173611`（fredphp/apipro）
> 适用平台：Linux x86_64（Ubuntu 20.04+ / CentOS 7+ / Debian 11+）
> 端口约定：**API 3100 / RPC 3101 / Redis 6399**

---

## 目录

1. [架构与组件](#1-架构与组件)
2. [环境前置条件](#2-环境前置条件)
3. [获取源码](#3-获取源码)
4. [配置文件说明](#4-配置文件说明)
5. [数据库准备](#5-数据库准备)
6. [Redis 准备](#6-redis-准备)
7. [编译构建](#7-编译构建)
8. [启动运行](#8-启动运行)
9. [冒烟测试](#9-冒烟测试)
10. [生产环境部署](#10-生产环境部署)
11. [健康检查与日志](#11-健康检查与日志)
12. [常见问题排查](#12-常见问题排查)

---

## 1. 架构与组件

```
HTTP/WS ──►  apipro-api   (go-zero REST,  :3100)   ── gRPC ──►  apipro-rpc  (go-zero zRPC, :3101)
             ├─ REST 路由 → 调用 RPC                                ├─ 全部业务逻辑
             ├─ JWT 鉴权                                            ├─ Redis 读穿缓存 + TTL
             ├─ 滑动窗口限流 (Redis ZSET)                           ├─ 后台定时刷新热缓存
             ├─ /ws/chat  WebSocket 聊天                            ├─ 用户体系 (md5 兼容)
             └─ /chat.html 嵌入式聊天小部件                          └─ MySQL / SQLite 数据访问
                                                                    │
                                                  ┌─────────────────┼─────────────────┐
                                                  ▼                 ▼                 ▼
                                              MySQL            Redis (:6399)      data/jsonp/
                                          (多 schema 跨库)      缓存/会话/历史       静态快照
```

| 组件 | 二进制 | 端口 | 配置文件 |
|---|---|---|---|
| API 网关 | `bin/apipro-api` | 3100 (HTTP/WS) | `cmd/api/etc/apipro.yaml` |
| RPC 服务 | `bin/apipro-rpc` | 3101 (gRPC) | `cmd/rpc/etc/apipro.yaml` |
| Redis | `redis-server` | 6399 | `--port 6399` |
| MySQL | `mysqld` | 3306 | DSN 写在 yaml |

> ⚠️ 两个 yaml 的 `DBDriver` / `DataSource` / `SchemaPrefix` / `ApiKey*` / `SmsDevBypassCode` / Redis 配置**必须完全一致**，否则 API 与 RPC 加解密或表名前缀会失配。

---

## 2. 环境前置条件

### 2.1 必需

| 软件 | 最低版本 | 验证命令 |
|---|---|---|
| Go | 1.24+ | `go version` |
| Redis | 5.0+ | `redis-server --version` |
| MySQL（生产） | 5.7+ / 8.0 | `mysql --version` |
| gcc / glibc | 系统自带 | `gcc --version` |

### 2.2 可选（仅当代码生成时需要）

| 软件 | 版本 | 用途 |
|---|---|---|
| `goctl` | v1.7+ | 从 `.api` 文件生成 handler/logic/types |
| `protoc` | v25+ | 从 `.proto` 生成 gRPC stub |
| `protoc-gen-go` / `protoc-gen-go-grpc` | latest | protoc Go 插件 |

> 部署运行**不需要** goctl / protoc —— 仓库已提交所有生成代码。仅当你修改 `desc/api/apipro.api` 或 `desc/proto/apipro.proto` 时才需重新生成。

### 2.3 沙箱环境路径（已预装）

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH"
```

---

## 3. 获取源码

```bash
# 方式一：从 GitHub 克隆
git clone https://github.com/fredphp/apipro.git
cd apipro

# 方式二：当前沙箱已部署在
cd /home/z/my-project/apipro
```

目录结构关键说明：

```
apipro/
├── cmd/
│   ├── api/                # HTTP+WS 网关 (go-zero REST)
│   │   ├── apipro.go       # main 入口
│   │   └── etc/apipro.yaml # API 配置
│   ├── rpc/                # gRPC 服务 (go-zero zRPC)
│   │   ├── apipro.go       # main 入口
│   │   └── etc/apipro.yaml # RPC 配置
│   └── smoketest/          # 端到端冒烟测试工具
├── common/                 # auth / cache / db / model / ratelimit / store
├── deploy/
│   ├── schema.mysql.sql    # MySQL 多库 schema + 种子数据
│   └── schema.sqlite.sql   # SQLite 单文件 schema (dev)
├── desc/
│   ├── api/apipro.api      # REST DSL
│   └── proto/              # protobuf 契约 + 生成代码
├── pkg/
│   ├── codec/              # AES-ECB 加解密
│   ├── wschat/             # WebSocket hub
│   └── fixture/            # 数据模型结构体
├── public/chat.html        # 静态嵌入聊天小部件
├── start.sh                # 一键启动（前台）
└── daemon.sh               # 守护进程启动（后台）
```

---

## 4. 配置文件说明

### 4.1 API 配置 `cmd/api/etc/apipro.yaml`

```yaml
Name: apipro
Host: 0.0.0.0           # 生产环境建议改为内网 IP
Port: 3100
AppMode: dev            # dev = SMS 旁路码生效；prod = 严格校验

# RPC 客户端 → apipro-rpc
ApiproRpc:
  Endpoints:
    - 127.0.0.1:3101
  NonBlock: true
  Timeout: 2000000000   # 2 秒（纳秒）

# Redis（会话 / 限流 / 聊天历史 / 共享缓存）
Redis:
  Host: 127.0.0.1:6399
  Type: node

# ─── 多数据库配置 ───────────────────────────────────────────
# Mode 1 (默认推荐)：共享连接池，DSN 不绑定具体库，支持跨库 JOIN
DBDriver: mysql
DataSource: root:123456@tcp(127.0.0.1:3306)/?charset=utf8mb4&parseTime=true&loc=Local&allowNativePasswords=true
#                                                     ^ 注意：路径为空，跨库 JOIN 必须为空

# Mode 2 (可选)：按 schema 分池（每个 schema 单独 DSN）
# Databases:
#   user:     "root:123456@tcp(127.0.0.1:3306)/zb_user?charset=utf8mb4&parseTime=true&loc=Local"
#   live:     "root:123456@tcp(127.0.0.1:3306)/zb_live?charset=utf8mb4&parseTime=true&loc=Local"
#   ...

SchemaPrefix: "zb_"          # → zb_user.user, zb_live.live_room ...
EimSchemaPrefix: "eim_"      # → eim_user.eim_user ...

# SQLite 单文件模式 (dev)
# DBDriver: sqlite
# DataSource: ./data/apipro.db

# ─── 传输加密 (AES-128-ECB/PKCS7) ─────────────────────────
# Web (plat=3): req=PHp1st5vEg5Ca8FH, resp=qlCJekfRKwWkQxl7
# WAP (plat=4): req=PHp1st5vEg5Ca8FH, resp=PHp1st5vEg5Ca8FH
ApiKeyReq: "PHp1st5vEg5Ca8FH"
ApiKeyResp: "qlCJekfRKwWkQxl7"
ApiKeyRespWap: "PHp1st5vEg5Ca8FH"

# SMS 开发旁路码（dev 模式下 smsCode=8888 即可绕过验证）
SmsDevBypassCode: "8888"

# 图形验证码 / JSONP 快照 / 文件 URL 前缀 / 限流 / CORS
KaptchaCodeLen: 5
KaptchaTTL: 300
JsonpSnapshotDir: ./data/jsonp
FileBaseURL: "https://sta.ncctrials.com/file"
RateLimitPerMinute: 120
RateLimitAuthPerMinute: 20
ChatMaxMsgLen: 500
ChatHistoryLim: 50
ChatRatePerMin: 60
CorsOrigin: "*"
```

### 4.2 RPC 配置 `cmd/rpc/etc/apipro.yaml`

包含 API 配置的全部数据库 / 加密 / Redis 项，**外加**：

```yaml
Name: apipro.rpc
ListenOn: 127.0.0.1:3101
AppMode: dev

# 缓存 TTL (秒)
CacheMatchListTtl: 60
CacheMatchDetailTtl: 90
CacheRoomDetailTtl: 30
CacheCommentatorTtl: 120
CacheLiveTtl: 15
CacheUserProfileTtl: 120

# 后台定时刷新间隔 (秒)
RefreshMatchListTtl: 60
RefreshLiveTtl: 15
RefreshCommentatorTtl: 120
```

### 4.3 关键配置规则

| 项 | 规则 |
|---|---|
| `DataSource` 路径 | **必须为空**（`/` 后无库名），否则跨库 JOIN 失败 |
| `SchemaPrefix` | API 与 RPC 必须一致；改 `haima_` 即可对齐 backend-zero 上游 |
| `ApiKey*` | API 与 RPC 必须一致；与 zbyy 客户端约定值不可改 |
| `AppMode` | `dev` 时 `SmsDevBypassCode` 生效；`prod` 必须改为严格校验 |
| `Host` | 生产环境建议从 `0.0.0.0` 改为内网 IP，由 Nginx 反代 |

---

## 5. 数据库准备

### 5.1 MySQL（生产推荐）

```bash
# 1) 登录 MySQL
mysql -u root -p

# 2) 创建库与表 + 写入种子数据（一条命令完成）
mysql> source /path/to/apipro/deploy/schema.mysql.sql;

# 3) 验证库已创建
mysql> SHOW DATABASES LIKE 'zb_%';
+--------------------+
| Database (zb_%)    |
+--------------------+
| zb_admin           |
| zb_basketball      |
| zb_chat            |
| zb_football        |
| zb_gift            |
| zb_live            |
| zb_sys             |
| zb_user            |
+--------------------+

# 4) 验证种子数据
mysql> SELECT COUNT(*) FROM zb_user.user;
mysql> SELECT COUNT(*) FROM zb_live.live_room;
```

`schema.mysql.sql` 会自动创建 8 个 `zb_*` 库（user/live/chat/gift/admin/sys/basketball/football）以及对应的表与种子数据。IM 相关的 `eim_*` 库需要单独脚本（如有）。

> ⚠️ **生产环境务必改 MySQL 密码**：yaml 中默认 `root:123456` 仅为示例。生产环境应：
> ```sql
> CREATE USER 'apipro'@'%' IDENTIFIED BY '<strong-password>';
> GRANT ALL PRIVILEGES ON zb_*.* TO 'apipro'@'%';
> GRANT ALL PRIVILEGES ON eim_*.* TO 'apipro'@'%';
> FLUSH PRIVILEGES;
> ```
> 然后把 yaml 的 DSN 改为 `apipro:<strong-password>@tcp(...)`。

### 5.2 SQLite（开发/自测）

无需手动建表 —— 首次启动 RPC 时，model 层会自动 `CREATE TABLE IF NOT EXISTS`：

```yaml
# 同时修改 cmd/api/etc/apipro.yaml 和 cmd/rpc/etc/apipro.yaml：
DBDriver: sqlite
DataSource: ./data/apipro.db
# SchemaPrefix 在 SQLite 模式下被自动忽略（SQLite 无跨库命名空间）
```

数据库文件会生成在 `./data/apipro.db`。如需重置：删除该文件后重启即可。

### 5.3 数据库连接校验

启动前可用一行命令验证 DSN：

```bash
# MySQL
mysql -u root -p123456 -h 127.0.0.1 -P 3306 -e "SELECT 1"

# SQLite
sqlite3 /home/z/my-project/apipro/data/apipro.db ".tables"
```

---

## 6. Redis 准备

### 6.1 启动 Redis（专用端口 6399）

```bash
# 检查是否已在运行
redis-cli -p 6399 ping
# → PONG  表示已运行

# 若未运行，启动专用实例（无持久化，沙箱/开发推荐）
redis-server --port 6399 --daemonize yes --save "" --appendonly no

# 验证
redis-cli -p 6399 ping
# → PONG
```

### 6.2 生产环境 Redis 建议

| 项 | 推荐值 |
|---|---|
| 持久化 | `--save 60 1000 --appendonly yes`（聊天历史需要持久化时） |
| 密码 | `--requirepass <password>`，并在 yaml 中 `Host: 127.0.0.1:6399?password=<password>` |
| 内存 | `--maxmemory 512mb --maxmemory-policy allkeys-lru` |
| 部署 | 独立实例，与业务 MySQL 分离 |

---

## 7. 编译构建

### 7.1 一键构建

```bash
cd /home/z/my-project/apipro
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH"

# 构建三个二进制（输出到 bin/）
go build -o bin/apipro-rpc     ./cmd/rpc
go build -o bin/apipro-api     ./cmd/api
go build -o bin/smoketest      ./cmd/smoketest

# 验证产物
ls -la bin/
# apipro-api  apipro-rpc  smoketest
```

### 7.2 构建前自检

```bash
# 静态检查
go vet ./...

# 单元测试（可选，~5s）
go test ./...
```

### 7.3 交叉编译（生产环境）

如部署机器与构建机架构不同：

```bash
# Linux x86_64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/apipro-rpc ./cmd/rpc
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/apipro-api ./cmd/api

# Linux ARM64（如鲲鹏 / 飞腾）
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/apipro-rpc ./cmd/rpc
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/apipro-api ./cmd/api
```

> `-ldflags="-s -w"` 去掉调试信息，二进制体积可减约 30%。
> SQLite 模式需要 CGO，因此 `CGO_ENABLED=1` 且需要目标平台的 C 工具链。MySQL 模式可纯静态。

### 7.4 重新生成代码（仅修改 .api / .proto 时）

```bash
# 1. 重新生成 gRPC stub
protoc --proto_path=desc/proto \
  --go_out=desc/proto/gen/apipro --go_opt=paths=source_relative \
  --go-grpc_out=desc/proto/gen/apipro --go-grpc_opt=paths=source_relative \
  desc/proto/apipro.proto

# 2. 重新生成 REST handler/logic/types（注意：会覆盖 logic 文件！务必先 git stash）
goctl api go --api desc/api/apipro.api --dir cmd/api --style=goZero
# 恢复自定义 logic：git checkout cmd/api/internal/logic/
```

---

## 8. 启动运行

### 8.1 启动顺序（重要）

```
Redis  →  apipro-rpc  →  apipro-api
```

API 启动时会通过 zRPC 连接 RPC，RPC 没起来会导致 API 报错。

### 8.2 方式一：start.sh（前台，开发推荐）

```bash
cd /home/z/my-project/apipro
./start.sh
```

`start.sh` 会依次：
1. 检查 Redis :6399，未运行则启动
2. `go build` 重新编译 apipro-rpc 和 apipro-api
3. `pkill` 旧的 RPC / API 进程
4. 后台启动 RPC（日志写 `/tmp/apipro-rpc.log`）
5. 前台启动 API（日志直接输出到终端）

按 `Ctrl+C` 停止 API；RPC 仍在后台，需要 `pkill -f apipro-rpc` 单独停止。

### 8.3 方式二：daemon.sh（后台守护，常驻推荐）

```bash
cd /home/z/my-project/apipro
nohup ./daemon.sh > /tmp/apipro-daemon.log 2>&1 &
disown
```

`daemon.sh` 会：
1. 杀掉旧 RPC / API 进程
2. 后台启动 RPC 与 API（日志分别写 `/tmp/apipro-rpc.log` 和 `/tmp/apipro-api.log`）
3. 写 PID 文件到 `/tmp/apipro-rpc.pid` 与 `/tmp/apipro-api.pid`
4. 进入 while 循环，两个进程都退出时脚本才退出

```bash
# 查看进程
cat /tmp/apipro-rpc.pid /tmp/apipro-api.pid
ps -ef | grep -E "apipro-(rpc|api)" | grep -v grep

# 查看日志
tail -f /tmp/apipro-rpc.log
tail -f /tmp/apipro-api.log

# 停止
kill $(cat /tmp/apipro-api.pid) $(cat /tmp/apipro-rpc.pid)
# 或简单粗暴：
pkill -f apipro-rpc; pkill -f apipro-api
```

### 8.4 方式三：手动分步启动（最透明）

```bash
# 终端 1 — Redis
redis-server --port 6399 --daemonize yes --save "" --appendonly no

# 终端 2 — RPC
cd /home/z/my-project/apipro
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml

# 终端 3 — API
cd /home/z/my-project/apipro
export PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"
./bin/apipro-api -f cmd/api/etc/apipro.yaml
```

### 8.5 启动验证

API 启动后日志应输出：

```
Starting apipro-api server at 0.0.0.0:3100...
multidb: shared-pool mode (cross-schema queries enabled)
```

RPC 启动后日志应输出：

```
Starting rpc server at 127.0.0.1:3101...
multidb: shared-pool mode (cross-schema queries enabled)
```

看到 `multidb: shared-pool mode` 表示数据库连接已就绪。

---

## 9. 冒烟测试

### 9.1 健康检查

```bash
curl http://127.0.0.1:3100/health
# → ok
```

### 9.2 端到端冒烟（protobuf 信封 + 加解密）

```bash
cd /home/z/my-project/apipro
./bin/smoketest http://127.0.0.1:3100
```

该脚本依次验证 5 个场景：

| # | 场景 | 期望 |
|---|---|---|
| 1 | `GET /api/kaptcha?t=<ts>&mobile=testuser` | 200，返回 SVG，解析出 5 位验证码 |
| 2 | `POST /login/reg`（accountType=2，plat=3 Web） | `err_code=200`（首次）或 `4104 账号已被注册`（已注册） |
| 3 | `POST /login/login`（Web plat=3） | `err_code=200`，`new_session_id == result.sessionId` |
| 4 | `POST /login/login`（WAP plat=4） | `err_code=200`，WAP 响应密钥解密成功 |
| 5 | `POST /login/login`（legacy JSON 兼容） | `code=200`，向后兼容保留 |

全部通过表示注册 / 登录 / AES-ECB 加解密 / WAP 密钥切换 / 历史兼容均正常。

### 9.3 通过 Caddy 网关访问

沙箱只暴露 80/443 端口，必须通过 `XTransformPort` 参数转发：

```bash
# 健康检查
curl "https://<preview-host>/health?XTransformPort=3100"

# 比赛列表
curl "https://<preview-host>/api/v1/match/list?XTransformPort=3100"
```

---

## 10. 生产环境部署

### 10.1 systemd 服务（推荐）

**`/etc/systemd/system/apipro-rpc.service`**

```ini
[Unit]
Description=apipro RPC service (go-zero zRPC)
After=network.target redis@6399.service mysqld.service
Requires=redis@6399.service

[Service]
Type=simple
User=apipro
Group=apipro
WorkingDirectory=/opt/apipro
ExecStart=/opt/apipro/bin/apipro-rpc -f /opt/apipro/cmd/rpc/etc/apipro.yaml
Restart=always
RestartSec=3
LimitNOFILE=65536
StandardOutput=append:/var/log/apipro/rpc.log
StandardError=append:/var/log/apipro/rpc.err.log

[Install]
WantedBy=multi-user.target
```

**`/etc/systemd/system/apipro-api.service`**

```ini
[Unit]
Description=apipro API gateway (go-zero REST)
After=network.target apipro-rpc.service
Requires=apipro-rpc.service

[Service]
Type=simple
User=apipro
Group=apipro
WorkingDirectory=/opt/apipro
ExecStart=/opt/apipro/bin/apipro-api -f /opt/apipro/cmd/api/etc/apipro.yaml
Restart=always
RestartSec=3
LimitNOFILE=65536
StandardOutput=append:/var/log/apipro/api.log
StandardError=append:/var/log/apipro/api.err.log

[Install]
WantedBy=multi-user.target
```

启用：

```bash
sudo useradd -r -s /usr/sbin/nologin apipro
sudo mkdir -p /opt/apipro /var/log/apipro
sudo chown -R apipro:apipro /opt/apipro /var/log/apipro

# 拷贝二进制和配置
sudo cp -r bin cmd deploy /opt/apipro/
sudo chown -R apipro:apipro /opt/apipro

sudo systemctl daemon-reload
sudo systemctl enable --now apipro-rpc
sudo systemctl enable --now apipro-api

# 查看状态
sudo systemctl status apipro-rpc apipro-api
```

### 10.2 Nginx 反向代理

```nginx
upstream apipro_api {
    server 127.0.0.1:3100;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    ssl_certificate     /etc/ssl/api.example.com.crt;
    ssl_certificate_key /etc/ssl/api.example.com.key;

    # REST API
    location /api/ {
        proxy_pass http://apipro_api;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 30s;
    }

    # WebSocket 聊天
    location /ws/chat {
        proxy_pass http://apipro_api;
        proxy_http_version 1.1;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;   # 长连接保活
    }

    # 静态聊天小部件
    location /chat.html {
        proxy_pass http://apipro_api;
    }

    # 健康检查
    location /health {
        proxy_pass http://apipro_api;
        access_log off;
    }
}
```

### 10.3 部署目录建议

```
/opt/apipro/
├── bin/
│   ├── apipro-api
│   └── apipro-rpc
├── cmd/
│   ├── api/etc/apipro.yaml      # 生产配置（密码已改）
│   └── rpc/etc/apipro.yaml
├── data/
│   ├── jsonp/                   # JSONP 快照（自动生成）
│   └── apipro.db                # 仅 SQLite 模式
├── public/chat.html
└── deploy/schema.mysql.sql

/var/log/apipro/
├── api.log
├── api.err.log
├── rpc.log
└── rpc.err.log
```

### 10.4 生产配置 Checklist

- [ ] `AppMode: prod`（关闭 SMS 旁路码）
- [ ] `Host` 改为内网 IP（如 `127.0.0.1` 或 `10.0.0.x`），由 Nginx 暴露
- [ ] MySQL DSN 用户名密码改为专用账号（非 root）
- [ ] Redis 设置密码并在 yaml 的 `Host` 加 `?password=...`
- [ ] `CorsOrigin` 从 `*` 改为具体前端域名
- [ ] `FileBaseURL` 改为生产 CDN
- [ ] 防火墙：仅放行 80/443，3100/3101/6399/3306 仅本机访问
- [ ] 日志切割：logrotate 配置 `/var/log/apipro/*.log`
- [ ] 监控：Prometheus + Grafana（go-zero 自带 `/metrics`）

### 10.5 性能调优建议

| 维度 | 建议 |
|---|---|
| 文件句柄 | `ulimit -n 65536`，systemd 已设 `LimitNOFILE` |
| MySQL 连接池 | 共享池默认 `SetMaxOpenConns(0)`（无上限），生产建议在 `common/db/multidb.go` 设为 50~100 |
| Redis 连接池 | go-zero 默认 10 个连接，高并发可在 yaml `Redis:` 下加 `Node:` 子项调优 |
| goroutine | go-zero REST 默认 `MaxConns: 10000`，按机器配置调整 |
| GOMAXPROCS | 默认等于 CPU 核数，容器化部署建议显式设置 |

---

## 11. 健康检查与日志

### 11.1 健康检查

```bash
# API 活性
curl -fsS http://127.0.0.1:3100/health
# → ok

# RPC 活性（gRPC，需要 grpcurl）
grpcurl -plaintext 127.0.0.1:3101 list

# Redis
redis-cli -p 6399 ping

# MySQL
mysqladmin -h 127.0.0.1 -P 3306 -u root -p ping
```

### 11.2 日志位置

| 部署方式 | 日志位置 |
|---|---|
| `start.sh` | RPC：`/tmp/apipro-rpc.log`；API：终端 stdout |
| `daemon.sh` | RPC：`/tmp/apipro-rpc.log`；API：`/tmp/apipro-api.log` |
| systemd | `/var/log/apipro/{api,rpc}.{log,err.log}` |
| Nginx | `/var/log/nginx/access.log`、`/var/log/nginx/error.log` |

### 11.3 日志关键字

启动成功的标志：

```
Starting apipro-api server at 0.0.0.0:3100
Starting rpc server at 127.0.0.1:3101
multidb: shared-pool mode (cross-schema queries enabled)
```

启动失败常见关键字：

```
dial tcp 127.0.0.1:6399: connect: connection refused   # Redis 没起
dial tcp 127.0.0.1:3306: connect: connection refused   # MySQL 没起
Error 1045: Access denied for user 'root'              # MySQL 密码错
context deadline exceeded                              # RPC 没起或端口不对
```

---

## 12. 常见问题排查

### 12.1 API 启动后立刻退出

**现象**：`./bin/apipro-api` 启动后秒退，日志只有 `Starting apipro-api server...`

**排查**：

```bash
# 1. RPC 是否在跑
ps -ef | grep apipro-rpc | grep -v grep
# 没有则先启动 RPC

# 2. 端口是否被占用
ss -lntp | grep -E '3100|3101'

# 3. 配置文件路径是否正确
./bin/apipro-api -f cmd/api/etc/apipro.yaml
# 必须用 -f 指定，否则 go-zero 找不到默认配置
```

### 12.2 跨库 JOIN 报错 `Table 'xxx.yyy' doesn't exist`

**原因**：`DataSource` DSN 中绑定了具体库名。

**修复**：把 DSN 路径改为空：

```yaml
# ❌ 错误：绑定了 /apipro
DataSource: root:123456@tcp(127.0.0.1:3306)/apipro?charset=utf8mb4...

# ✓ 正确：路径为空
DataSource: root:123456@tcp(127.0.0.1:3306)/?charset=utf8mb4...
```

### 12.3 注册接口报 `账号已被注册`（SQLite 残留）

**原因**：`./data/apipro.db` 上次测试已写入同名账号。

**修复**：

```bash
rm /home/z/my-project/apipro/data/apipro.db
# 重启 RPC，表会自动重建
```

或换一个 `loginName` 测试。

### 12.4 登录返回 `err_code != 200`

| err_code | 含义 | 处理 |
|---|---|---|
| 4001 | 账号不存在 | 先调注册接口 |
| 4002 | 密码错误 | 检查客户端 md5 算法：`md5(client_md5 + base64_salt)` |
| 4003 | 账号被禁用 | 检查 `user.status` 字段 |
| 4101 | 验证码错误 | dev 模式下应使用 `8888` 旁路码 |
| 4104 | 账号已注册（注册接口） | 换 loginName 或清库 |

### 12.5 WAP 端解密失败

**原因**：WAP（plat=4）使用与请求相同的响应密钥，与 Web（plat=3）不同。

**检查**：
- `cmd/api/etc/apipro.yaml` 中 `ApiKeyRespWap: "PHp1st5vEg5Ca8FH"` 必须与 `ApiKeyReq` 相同
- 客户端请求头 `plat` 字段：Web=3，WAP=4

### 12.6 WebSocket 聊天连不上

**排查链路**：

```bash
# 1. API 进程是否在跑
curl http://127.0.0.1:3100/health

# 2. 直接连 WebSocket（绕过 Nginx）
wscat -c "ws://127.0.0.1:3100/ws/chat?roomNum=1001"
# 不带 token = 游客身份，应该能连

# 3. 通过 Nginx 连（检查 Upgrade 头）
wscat -c "wss://api.example.com/ws/chat?roomNum=1001&token=<jwt>"
# 失败则检查 Nginx 的 /ws/chat location 配置

# 4. Redis 聊天历史
redis-cli -p 6399 KEYS "chat:room:1001:*"
```

### 12.7 限流被误触发

**现象**：返回 `429 Too Many Requests`。

**处理**：

```bash
# 查看当前 IP 的限流计数
redis-cli -p 6399 ZRANGE "ratelimit:127.0.0.1" 0 -1 WITHSCORES

# 清除限流
redis-cli -p 6399 DEL "ratelimit:127.0.0.1"

# 或在 yaml 调大阈值
RateLimitPerMinute: 120       # → 600
RateLimitAuthPerMinute: 20    # → 100
```

### 12.8 缓存不刷新

**现象**：数据库已更新但 API 仍返回旧数据。

**处理**：

```bash
# 1. 强制刷新指定缓存族
curl -X POST http://127.0.0.1:3100/api/v1/admin/cache/refresh \
  -H "Authorization: Bearer <admin-jwt>" \
  -H "Content-Type: application/json" \
  -d '{"family":"match_list"}'

# 2. 查看缓存统计
curl http://127.0.0.1:3100/api/v1/admin/cache/stats \
  -H "Authorization: Bearer <admin-jwt>"

# 3. 全清（慎用）
redis-cli -p 6399 FLUSHDB
```

支持的缓存族：`match_list` / `match_detail` / `room_detail` / `commentator` / `live` / `user_profile`。

---

## 附录：快速启动 Cheatsheet

```bash
# === 一次性命令（沙箱环境）===
cd /home/z/my-project/apipro
export PATH="$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH"

# 1. 起 Redis（如未运行）
redis-cli -p 6399 ping || redis-server --port 6399 --daemonize yes --save "" --appendonly no

# 2. 构建
go build -o bin/apipro-rpc ./cmd/rpc
go build -o bin/apipro-api ./cmd/api
go build -o bin/smoketest  ./cmd/smoketest

# 3. 起 RPC（后台）
pkill -f apipro-rpc 2>/dev/null; sleep 1
nohup ./bin/apipro-rpc -f cmd/rpc/etc/apipro.yaml > /tmp/apipro-rpc.log 2>&1 &
sleep 2

# 4. 起 API（后台）
pkill -f apipro-api 2>/dev/null; sleep 1
nohup ./bin/apipro-api -f cmd/api/etc/apipro.yaml > /tmp/apipro-api.log 2>&1 &
sleep 2

# 5. 验证
curl http://127.0.0.1:3100/health          # → ok
./bin/smoketest http://127.0.0.1:3100      # 5/5 通过

# === 停止 ===
pkill -f apipro-api; pkill -f apipro-rpc
```
