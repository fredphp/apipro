# apipro 接口文档

> 基于 zbyy 直播数据模型的独立 go-zero RPC API 服务。
> 所有读接口均走 Redis 缓存 + 定时刷新；聊天走 WebSocket + 静态 HTML。

---

## 目录

- [一、服务概览](#一服务概览)
- [二、鉴权说明（zbyy 兼容加密）](#二鉴权说明zbyy-兼容加密)
- [三、注册 / 登录 API](#三注册--登录-api)
- [四、比赛列表 API](#四比赛列表-api)
- [五、Room ID 详情 API](#五room-id-详情-api)
- [六、解说员信息 API](#六解说员信息-api)
- [七、直播比赛 API](#七直播比赛-api)
- [八、聊天 API（WebSocket + 静态 HTML）](#八聊天-apiwebsocket--静态-html)
- [九、用户中心 API](#九用户中心-api)
- [十、缓存管理 API（需鉴权）](#十缓存管理-api需鉴权)
- [十一、统一错误格式](#十一统一错误格式)
- [十二、限流策略](#十二限流策略)

---

## 一、服务概览

| 进程 | 端口 | 协议 | 说明 |
|------|------|------|------|
| apipro-api | 3100 | HTTP / WebSocket | 对外网关，所有 REST 接口 + WS 聊天 |
| apipro-rpc | 3101 | gRPC | 内部 RPC 服务，直接读写数据库 + 管理 Redis 缓存 |
| Redis | 6399 | Redis | 缓存 + 定时刷新 + 聊天历史 |

**Base URL**: `http://<host>:3100`

所有接口（除 `/health` 和 `/chat.html`）统一前缀 `/api/v1`。

### 通用响应头

```
Access-Control-Allow-Origin: *
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
Referrer-Policy: no-referrer
```

### 健康检查

```
GET /health
```

**响应**：
```json
{ "ok": true, "service": "apipro", "ts": 1723000000 }
```

---

## 二、鉴权说明（zbyy 兼容加密）

### 密码加密算法

apipro **完全复刻 zbyy 客户端的 md5 加密方式**，客户端无需任何改造即可对接。

zbyy 前端（`src/js/utils/common.js`）在发送密码前会先做 md5：

```javascript
// SECRET_KEY = "&%*$8@!!%"
function md5Pwd(password, pwdType) {
  if (pwdType === 2) {
    return md5(password)
  }
  return md5(md5(password.toLowerCase()) + "&%*$8@!!%")
}
```

| pwdType | 算法 | 说明 |
|---------|------|------|
| 1 | `md5( md5(password.toLowerCase()) + "&%*$8@!!%" )` | **默认**，zbyy 客户端用这个 |
| 2 | `md5(password)` | 简单 md5 |

**示例**：
- 原始密码 `123456`，pwdType=1
- `md5("123456")` = `e10adc3949ba59abbe56e057f20f883e`
- `md5("e10adc3949ba59abbe56e057f20f883e&%*$8@!!%")` = `2e36f5fa46a866a6e91b71524dd8d155`
- 客户端发送的 `password` 字段值就是 `2e36f5fa46a866a6e91b71524dd8d155`

### Token 机制

- 登录/注册成功后返回 `accessToken` + `refreshToken`（JWT HS256）
- 需要鉴权的接口在请求头携带：`Authorization: Bearer <accessToken>`
- accessToken 过期后用 refreshToken 调 `/auth/refresh` 换新的

---

## 三、注册 / 登录 API

### 3.1 用户注册

```
POST /api/v1/auth/register
Content-Type: application/json
```

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| loginName | string | 是 | 登录账号 |
| phone | string | 是 | 手机号 |
| countryCode | string | 是 | 国家码，如 `+86` |
| password | string | 是 | **客户端 md5 加密后的密码**（不是明文） |
| smsCode | string | 是 | 短信验证码（开发环境任意 4 位数字即可，如 `1234`） |
| nickName | string | 否 | 昵称，默认用 loginName |
| pwdType | int32 | 否 | 密码加密类型，默认 1 |

**请求示例**：
```bash
curl -X POST http://127.0.0.1:3100/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{
    "loginName": "testuser",
    "phone": "13900000001",
    "countryCode": "+86",
    "password": "2e36f5fa46a866a6e91b71524dd8d155",
    "smsCode": "1234",
    "nickName": "测试用户",
    "pwdType": 1
  }'
```

**响应**：
```json
{
  "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expiresAt": 1723000900
}
```

### 3.2 用户登录

```
POST /api/v1/auth/login
Content-Type: application/json
```

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| phone | string | 是 | 手机号 |
| countryCode | string | 是 | 国家码 |
| password | string | 是 | **客户端 md5 加密后的密码** |
| pwdType | int32 | 否 | 加密类型，默认 1 |

**请求示例**：
```bash
curl -X POST http://127.0.0.1:3100/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{
    "phone": "13800138000",
    "countryCode": "+86",
    "password": "2e36f5fa46a866a6e91b71524dd8d155",
    "pwdType": 1
  }'
```

**响应**：同注册。

### 3.3 游客登录

```
POST /api/v1/auth/guest
```

无请求体。服务端自动生成一个游客 uid（`G` 前缀），返回短期 token。

**响应**：同注册（但权限受限，仅可看公开内容 + 进房聊天）。

### 3.4 刷新 Token

```
POST /api/v1/auth/refresh
Content-Type: application/json
```

**请求体**：
```json
{ "refreshToken": "eyJhbGciOi..." }
```

**响应**：同注册（返回一组新的 accessToken + refreshToken）。

---

## 四、比赛列表 API

### 4.1 按日期获取比赛列表

```
GET /api/v1/match/list
```

**Query 参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|------|------|------|------|------|
| date | string | 否 | 今天 | 日期，格式 `YYYYMMDD`，如 `20250808` |
| page | int32 | 否 | 1 | 页码 |
| pageSize | int32 | 否 | 20 | 每页条数 |

**请求示例**：
```bash
curl 'http://127.0.0.1:3100/api/v1/match/list?date=20250808&page=1&pageSize=20'
```

**响应**：
```json
{
  "list": [
    {
      "scheduleId": "2025080801",
      "subCateName": "英超",
      "cateName": "足球",
      "matchTime": 1723108800000,
      "hostName": "曼联",
      "hostIcon": "https://cdn.zbyy.example/team/man.png",
      "guestName": "利物浦",
      "guestIcon": "https://cdn.zbyy.example/team/liv.png",
      "venue": "老特拉福德",
      "status": "not_started",
      "reservationStatus": 0,
      "anchors": [
        {
          "uid": "A1001",
          "nickName": "飞鱼解说",
          "icon": "https://cdn.zbyy.example/avatar/a1001.png",
          "cutOutIcon": "https://cdn.zbyy.example/avatar/a1001_cut.png",
          "intro": "前职业球员，专注英超解说10年",
          "fans": 128000,
          "follow": 98000,
          "hot": 9527,
          "anchor": {
            "uid": "A1001",
            "roomNum": "1001",
            "detail": "每晚8点英超直播",
            "notice": "禁止刷屏、禁止广告",
            "live": true
          }
        }
      ],
      "categoryId": 1,
      "categoryName": "足球",
      "categoryIcon": "",
      "matchStatusDesc": "今日直播",
      "hostScore": 0,
      "guestScore": 0
    }
  ],
  "total": 3,
  "date": "20250808"
}
```

**status 字段取值**：`not_started`（未开始）/ `living`（进行中）/ `over`（已结束）

### 4.2 推荐比赛

```
GET /api/v1/match/recommend
```

返回今天 + 未来几天的热门比赛（按热度排序）。

**响应**：
```json
{ "list": [ /* MatchItemJson[] */ ] }
```

### 4.3 比赛详情

```
GET /api/v1/match/detail
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | scheduleId，如 `2025080801` |

**请求示例**：
```bash
curl 'http://127.0.0.1:3100/api/v1/match/detail?id=2025080801'
```

**响应**：
```json
{ "match": { /* MatchItemJson */ } }
```

### 4.4 比赛分类列表

```
GET /api/v1/match/cates
```

返回所有比赛的分类名（去重）。

**响应**：
```json
{ "cateNames": ["足球", "篮球", "斯诺克"] }
```

### 4.5 按分类获取比赛

```
GET /api/v1/match/cate
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| cateName | string | 是 | 分类名，如 `足球` |
| page | int32 | 否 | 默认 1 |
| pageSize | int32 | 否 | 默认 20 |

**响应**：同 4.1 的 `MatchListResp`（但 `date` 字段为空）。

---

## 五、Room ID 详情 API

### 5.1 房间详情

```
GET /api/v1/room/detail
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| roomNum | string | 是 | 房间号，如 `1001` |

**请求示例**：
```bash
curl 'http://127.0.0.1:3100/api/v1/room/detail?roomNum=1001'
```

**响应**：
```json
{
  "room": {
    "roomNum": "1001",
    "title": "英超焦点战: 曼联 vs 利物浦",
    "cover": "https://cdn.zbyy.example/cover/1001.jpg",
    "live": true,
    "viewNum": 38211,
    "liveType": "football",
    "anchor": { /* CommentatorJson */ },
    "streamUrls": [
      "https://live.zbyy.example/1001/hd.m3u8",
      "https://live.zbyy.example/1001/sd.m3u8"
    ],
    "notice": "文明观赛，禁止刷屏",
    "tags": ["英超", "曼联", "利物浦"]
  }
}
```

### 5.2 房间赛程

返回某个房间（解说员）接下来要解说的所有比赛。

```
GET /api/v1/room/schedule
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| roomNum | string | 是 | 房间号 |

**响应**：
```json
{ "list": [ /* MatchItemJson[] */ ] }
```

### 5.3 房间排行榜

```
GET /api/v1/room/rank
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| roomNum | string | 是 | 房间号 |

**响应**：
```json
{
  "list": [
    {
      "uid": "U5001",
      "nickName": "球迷老王",
      "icon": "https://cdn.zbyy.example/u/5001.png",
      "score": 18820,
      "rank": 1
    }
  ]
}
```

---

## 六、解说员信息 API

### 6.1 解说员列表

```
GET /api/v1/commentator/list
```

**Query 参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|------|------|------|------|------|
| page | int32 | 否 | 1 | 页码 |
| pageSize | int32 | 否 | 20 | 每页条数 |

**请求示例**：
```bash
curl 'http://127.0.0.1:3100/api/v1/commentator/list?page=1&pageSize=20'
```

**响应**：
```json
{
  "list": [ /* CommentatorJson[] */ ],
  "total": 6
}
```

### 6.2 解说员详情

```
GET /api/v1/commentator/detail
```

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | string | 是 | 解说员 uid，如 `A1001` |

**响应**：
```json
{ "commentator": { /* CommentatorJson */ } }
```

**CommentatorJson 结构**：
```json
{
  "uid": "A1001",
  "nickName": "飞鱼解说",
  "icon": "https://cdn.zbyy.example/avatar/a1001.png",
  "cutOutIcon": "https://cdn.zbyy.example/avatar/a1001_cut.png",
  "intro": "前职业球员，专注英超解说10年",
  "fans": 128000,
  "follow": 98000,
  "hot": 9527,
  "anchor": {
    "uid": "A1001",
    "roomNum": "1001",
    "detail": "每晚8点英超直播",
    "notice": "禁止刷屏、禁止广告",
    "live": true
  }
}
```

---

## 七、直播比赛 API

### 7.1 直播间列表

```
GET /api/v1/live/list
```

**Query 参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|------|------|------|------|------|
| page | int32 | 否 | 1 | 页码 |
| pageSize | int32 | 否 | 20 | 每页条数 |

**响应**（zbyy 兼容分组结构 + 保留 list/total）：
```json
{
  "0": [
    {
      "roomNum": "1001",
      "title": "英超焦点战: 曼联 vs 利物浦",
      "cover": "https://cdn.zbyy.example/cover/1001.jpg",
      "anchor": { "uid": "A1001", "nickName": "飞鱼解说", "icon": "..." },
      "viewNum": 38211,
      "liveType": "football",
      "cateName": "英超",
      "viewCount": 38211,
      "cutOutCustomCoverUrl": "https://cdn.zbyy.example/cover/1001.jpg",
      "markType": 0,
      "liveStatus": 1
    }
  ],
  "1": [ /* 足球直播间 */ ],
  "2": [ /* 篮球直播间 */ ],
  "hot": [ /* 热门直播间（同 0，按热度） */ ],
  "list": [ /* 同 0，分页后的列表 */ ],
  "total": 3
}
```

> **zbyy 客户端兼容说明**：客户端通过 `res[0]`/`res[1]`/`res[2]`/`res.hot` 访问分组，`res.list`/`res.total` 为保留字段（旧调用方兼容）。

### 7.2 直播分类

```
GET /api/v1/live/types
```

**响应**：
```json
{
  "list": [
    { "code": "football", "name": "足球", "icon": "https://cdn.zbyy.example/ico/football.png" },
    { "code": "basketball", "name": "篮球", "icon": "https://cdn.zbyy.example/ico/basketball.png" },
    { "code": "snooker", "name": "斯诺克", "icon": "https://cdn.zbyy.example/ico/snooker.png" },
    { "code": "other", "name": "其它", "icon": "https://cdn.zbyy.example/ico/other.png" }
  ]
}
```

### 7.3 热门解说员

```
GET /api/v1/live/hot
```

按热度（hot 字段）降序返回解说员列表。

**响应**：
```json
{ "list": [ /* CommentatorJson[] */ ] }
```

---

## 八、聊天 API（WebSocket + 静态 HTML）

### 8.1 静态聊天页面（可 iframe 嵌入）

```
GET /chat.html
```

返回一个自包含的 HTML 聊天组件，可直接用 iframe 嵌入到任意页面：

```html
<iframe src="http://your-host:3100/chat.html?roomNum=1001&token=JWT"
        width="100%" height="600" frameborder="0"></iframe>
```

**Query 参数**：

| 参数 | 必填 | 说明 |
|------|------|------|
| roomNum | 是 | 房间号 |
| token | 是 | JWT token（登录 / 游客登录获取） |

### 8.2 WebSocket 连接

```
GET ws://<host>:3100/ws/chat?token=<JWT>&roomNum=<房间号>
```

**连接参数**（Query）：

| 参数 | 必填 | 说明 |
|------|------|------|
| token | 是 | JWT token |
| roomNum | 是 | 房间号 |

**连接示例**（wscat）：
```bash
wscat -c 'ws://127.0.0.1:3100/ws/chat?token=eyJhbGci...&roomNum=1001'
```

**连接成功后自动收到**：
1. 最近 50 条历史消息（如有）
2. 欢迎消息（系统）
3. 之后该房间的所有广播消息

### 8.3 消息格式

所有 WS 消息均为 JSON 文本，统一结构：

```json
{
  "code": 2003,
  "msgId": "abc123",
  "roomNum": "1001",
  "sendUid": "U0001",
  "nickName": "demo用户",
  "icon": "https://cdn.zbyy.example/avatar/demo.png",
  "level": 3,
  "msgType": 1,
  "content": "大家好",
  "ts": 1723000000
}
```

#### 消息码（兼容 zbyy WS_CODE）

| code | 方向 | 含义 |
|------|------|------|
| 1000 | C→S | 心跳 |
| 2000 | S→C | 心跳成功 |
| 1001 | C→S | 登录（连接时已带 token，一般不用） |
| 2001 | S→C | 登录成功 |
| 1003 | C→S | 进入直播间（连接时已带 roomNum，一般不用） |
| 2003 | S→C | 进入直播间成功 |
| 1004 | C→S | 离开直播间 |
| 2004 | S→C | 离开直播间成功 |
| 1005 | C→S | 发送聊天消息 |
| 2005 | S→C | 发送成功回执 |
| 1006 | C→S | 删除聊天消息 |
| 2006 | S→C | 删除成功回执 |
| 3000 | S→C | 推送聊天消息（他人发言广播） |
| 3001 | S→C | 推送删除消息 |
| 3002 | S→C | 系统通知 |
| 9999 | S→C | 错误 |

#### msgType 取值

| 值 | 含义 |
|----|------|
| 1 | 弹幕 |
| 2 | 礼物 |
| 3 | 普通文本 |

### 8.4 发送消息（客户端 → 服务端）

客户端发送 `code=1005` 的消息：

```json
{
  "code": 1005,
  "msgType": 1,
  "content": "这球太精彩了"
}
```

服务端校验通过后会：
1. 广播 `code=3000` 给该房间所有在线用户
2. 给发送者回 `code=2005` 成功回执
3. 写入 Redis 历史记录（LPUSH，保留最近 50 条）

### 8.5 心跳

客户端每 30 秒发送一次：
```json
{ "code": 1000 }
```
服务端回：
```json
{ "code": 2000, "ts": 1723000000 }
```
超过 60 秒无心跳，服务端主动断开连接。

### 8.6 聊天限制

| 限制项 | 默认值 | 配置项 |
|--------|--------|--------|
| 单条消息最大长度 | 500 字符 | `ChatMaxMsgLen` |
| 历史消息条数 | 50 条 | `ChatHistoryLim` |
| 每用户每分钟发言数 | 60 条 | `ChatRatePerMin` |
| XSS 防护 | 自动转义 `< > & "` | 内置 |
| 未登录 | 拒绝连接 | 必须 token |

---

## 九、用户中心 API

### 9.1 获取当前用户资料（需鉴权）

```
GET /api/v1/user/profile
Authorization: Bearer <accessToken>
```

**响应**：
```json
{
  "uid": "U0001",
  "loginName": "demo",
  "nickName": "demo用户",
  "phone": "13800138000",
  "countryCode": "+86",
  "grow": 120,
  "score": 500,
  "level": 3,
  "avatar": "https://cdn.zbyy.example/avatar/demo.png",
  "isUser": 1,
  "createdAt": 1723000000
}
```

---

## 十、缓存管理 API（需鉴权）

### 10.1 缓存统计

```
GET /api/v1/admin/cache/stats
Authorization: Bearer <accessToken>
```

**响应**：
```json
{
  "hits": 128,
  "misses": 35,
  "keys": 29,
  "families": [
    { "family": "match_list", "keys": 7, "hits": 60, "misses": 10 },
    { "family": "match_detail", "keys": 3, "hits": 20, "misses": 5 },
    { "family": "room_detail", "keys": 6, "hits": 15, "misses": 8 },
    { "family": "commentator", "keys": 7, "hits": 18, "misses": 4 },
    { "family": "live", "keys": 4, "hits": 10, "misses": 6 },
    { "family": "user_profile", "keys": 2, "hits": 5, "misses": 2 }
  ]
}
```

### 10.2 手动刷新缓存

```
POST /api/v1/admin/cache/refresh
Authorization: Bearer <accessToken>
Content-Type: application/json
```

**请求体**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| family | string | 是 | 缓存族名，如 `match_list` / `live` / `commentator` / `room_detail` / `match_detail` / `user_profile` 或 `all` |

**请求示例**：
```bash
curl -X POST http://127.0.0.1:3100/api/v1/admin/cache/refresh \
  -H 'Authorization: Bearer eyJ...' \
  -H 'Content-Type: application/json' \
  -d '{"family":"live"}'
```

**响应**：
```json
{ "ok": true, "message": "refreshed family: live" }
```

---

## 十一、统一错误格式

所有错误响应均为以下 JSON 结构：

```json
{
  "code": 401,
  "message": "invalid password"
}
```

| HTTP 状态码 | code | 含义 |
|-------------|------|------|
| 400 | 400 | 请求参数错误 |
| 401 | 401 | 未登录 / token 失效 / 密码错误 |
| 403 | 403 | 无权限 |
| 404 | 404 | 资源不存在 |
| 429 | 429 | 触发限流 |
| 500 | 500 | 服务端错误 |

**常见错误**：

| 场景 | code | message |
|------|------|---------|
| 手机号已注册 | 409 | `phone already registered` |
| 短信验证码错误 | 400 | `invalid sms code` |
| 手机号或密码错误 | 401 | `invalid phone or password` |
| token 过期 | 401 | `token expired` |
| token 无效 | 401 | `invalid token` |
| 房间不存在 | 404 | `room not found` |
| 比赛不存在 | 404 | `match not found` |
| 限流 | 429 | `rate limit exceeded, retry later` |
| 聊天超长 | 400 | `message too long` |
| 聊天限流 | 429 | `send too fast` |

---

## 十二、限流策略

| 接口类别 | 限制 | 配置项 |
|----------|------|--------|
| 全局（每 IP） | 120 次/分钟 | `RateLimitPerMinute` |
| 登录/注册 | 20 次/分钟（每 IP） | `RateLimitAuthPerMinute` |
| 聊天发言 | 60 条/分钟（每用户） | `ChatRatePerMin` |

超限返回 `429` + `rate limit exceeded, retry later`。

限流算法：Redis ZSET 滑动窗口。

---

## 附录：接口速查表

| # | 方法 | 路径 | 鉴权 | 说明 |
|---|------|------|------|------|
| 1 | GET | `/health` | 否 | 健康检查 |
| 2 | POST | `/api/v1/auth/register` | 否 | 注册 |
| 3 | POST | `/api/v1/auth/login` | 否 | 登录 |
| 4 | POST | `/api/v1/auth/guest` | 否 | 游客登录 |
| 5 | POST | `/api/v1/auth/refresh` | 否 | 刷新 token |
| 6 | GET | `/api/v1/user/profile` | 是 | 当前用户资料 |
| 7 | GET | `/api/v1/match/list` | 否 | 按日期获取比赛列表 |
| 8 | GET | `/api/v1/match/recommend` | 否 | 推荐比赛 |
| 9 | GET | `/api/v1/match/detail` | 否 | 比赛详情 |
| 10 | GET | `/api/v1/match/cates` | 否 | 比赛分类列表 |
| 11 | GET | `/api/v1/match/cate` | 否 | 按分类获取比赛 |
| 12 | GET | `/api/v1/room/detail` | 否 | 房间详情 |
| 13 | GET | `/api/v1/room/schedule` | 否 | 房间赛程 |
| 14 | GET | `/api/v1/room/rank` | 否 | 房间排行榜 |
| 15 | GET | `/api/v1/commentator/list` | 否 | 解说员列表 |
| 16 | GET | `/api/v1/commentator/detail` | 否 | 解说员详情 |
| 17 | GET | `/api/v1/live/list` | 否 | 直播间列表 |
| 18 | GET | `/api/v1/live/types` | 否 | 直播分类 |
| 19 | GET | `/api/v1/live/hot` | 否 | 热门解说员 |
| 20 | GET | `/api/v1/admin/cache/stats` | 是 | 缓存统计 |
| 21 | POST | `/api/v1/admin/cache/refresh` | 是 | 手动刷新缓存 |
| 22 | GET | `/chat.html` | 否 | 静态聊天页面 |
| 23 | WS | `/ws/chat` | 是(token) | WebSocket 聊天 |
