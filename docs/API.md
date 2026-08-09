# apipro 接口文档

> 基于 zbyy (YuYanTV) 数据模型的 go-zero API 网关。
> 所有加密接口采用 AES-128-ECB/PKCS7 + Protobuf/JSON 双信封；只读接口走 JSONP 明文；
> 聊天走 WebSocket 二进制帧（同样使用 AES-128-ECB 加密）。
>
> **本文档与代码 100% 对齐**：路由来自 `cmd/api/internal/handler/routes.go`，加解密中间件
> 来自 `pkg/codec/middleware.go`，AES/帧编解码来自 `pkg/codec/codec.go`，Protobuf 信封
> 定义见 `desc/proto/fy.proto`，密码算法见 `common/auth/pwd.go`，会话存储见
> `common/auth/session.go`，业务逻辑分发与错误码见 `cmd/rpc/internal/logic/call.go`，
> 响应结构体见 `cmd/rpc/internal/svc/builders.go`。

---

## 目录

- [一、服务概览](#一服务概览)
- [二、传输加密协议](#二传输加密协议)
- [三、鉴权机制](#三鉴权机制)
- [四、图形验证码 API](#四图形验证码-api)
- [五、注册 / 登录 API](#五注册--登录-api)
- [六、比赛 API](#六比赛-api)
- [七、直播 API](#七直播-api)
- [八、房间 API](#八房间-api)
- [九、用户 API](#九用户-api)
- [十、聊天 API（WebSocket）](#十聊天-apiwebsocket)
- [十一、错误码](#十一错误码)
- [十二、限流策略](#十二限流策略)
- [十三、Caddy 网关访问](#十三caddy-网关访问)
- [十四、附录：接口速查表](#十四附录接口速查表)

---

## 一、服务概览

### 1.1 进程拓扑

| 进程 | 端口 | 协议 | 说明 |
|------|------|------|------|
| apipro-api | 3100 | HTTP / WebSocket | 对外网关，承载所有 REST 接口、JSONP 接口、健康检查、图形验证码、静态聊天页、WebSocket |
| apipro-rpc | 3101 | gRPC（go-zero） | 内部 RPC 服务，所有业务逻辑都在 `Call` 方法里按 `method` 字段分发 |
| Redis | 6399 | RESP | 会话存储、限流、聊天历史、共享缓存、JSONP 快照 |

### 1.2 Base URL

```
http://<host>:3100
```

> **重要**：所有路由均挂在根路径下，**没有 `/api/v1/` 前缀**。
> 旧的 `/api/v1/auth/register`、`/api/v1/match/list` 等路径在当前实现中**全部 404**。
> 唯一带 `/api` 前缀的路径是 `GET /api/kaptcha`（图形验证码）。

### 1.3 通用响应头

由 go-zero 中间件统一注入：

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Headers: Content-Type, X-Session, X-Plat, X-Seq
Access-Control-Allow-Methods: GET, POST, OPTIONS
X-Content-Type-Options: nosniff
X-Frame-Options: SAMEORIGIN
Referrer-Policy: no-referrer
```

### 1.4 健康检查

```
GET /health
```

明文 JSON，**不加密、不需要鉴权**：

```json
{ "status": "ok", "ts": 1723000000 }
```

`ts` 为 `time.Now().Unix()` 秒级时间戳。

### 1.5 静态聊天页

```
GET /chat.html
```

返回 `text/html; charset=utf-8` 的静态文件 `public/chat.html`（494 行，含登录/进房/发消息的最小可用页面），`Cache-Control: public, max-age=60`。

---

## 二、传输加密协议

所有 `POST /login/*`、`POST /live/*`、`POST /match/*`、`POST /user/*`、`POST /sys/*`
接口都挂在 `codec.Transport` 中间件后，必须使用 **AES-128-ECB/PKCS7 + 帧**
封装。`GET /health`、`GET /api/kaptcha`、`GET /chat.html`、`GET /ws/chat` 以及
所有 `*.json` JSONP 接口**不走加密中间件**。

### 2.1 密钥（生产环境，源自 yaml）

| 配置键 | 值 | 用途 |
|--------|------|------|
| `ApiKeyReq` | `PHp1st5vEg5Ca8FH` | 解密客户端→服务端（Web/WAP 通用） |
| `ApiKeyResp` | `qlCJekfRKwWkQxl7` | 加密服务端→客户端（**Web，plat=3**） |
| `ApiKeyRespWap` | `PHp1st5vEg5Ca8FH` | 加密服务端→客户端（**WAP，plat=4**，与请求密钥相同） |

密钥均为 **16 字节 ASCII 字符串**（即 128-bit）。AES 模式 ECB，**无 IV**；填充为
**PKCS7**。`pkg/codec/codec.go` 顶部记录了一个验证用测试向量：

```
key        = "PHp1st5vEg5Ca8FH"  (16 字节)
plaintext  = `{"a":1}`           (7 字节)
ciphertext = hex f6d9666260c649f41050f460aa0a2cbe
```

### 2.2 HTTP 帧格式

帧由 **6 字节头部 + 密文** 组成：

```
偏移  长度  字段
0     2     帧魔数 = 0x00 0xA0  （注意：不是 0xFE 0xFF）
2     4     uint32 BE  密文长度（不含头部）
6     N     密文（AES-128-ECB/PKCS7 后的字节）
```

> **重要**：中间件同时兼容**带帧头**和**裸 AES**两种输入。即客户端可以直接发送
> 裸密文（无 6 字节头部），中间件会先尝试 `DecodeFrame`，失败再回退 `Decrypt`。
> 服务端响应**总是**带帧头。

### 2.3 三种请求明文形态

中间件解密后的明文有三种可能形态，由首字节判定：

| 形态 | 判定条件 | 处理 |
|------|---------|------|
| **Protobuf `FY_CLIENT`** | `plain[0] != '{'` 且能成功 `proto.Unmarshal` 为 `FY_CLIENT` 且 `common_req != nil` | 首选路径，与 zbyy 真实客户端完全一致。从 `CLIENT_INFO` 取 `session_id`、`seq`、`plat`；`common_req.param` 即业务 JSON |
| **JSON 信封（legacy）** | `plain[0] == '{'` 且解析后含 `param` 字段 | 从 `sessionId`、`seq`、`plat`（字符串）取值，`param` 即业务 JSON |
| **裸业务 JSON（debug）** | `plain[0] == '{'` 但不含 `param` 字段 | 直接当业务 JSON；`session_id` 取自 `X-Session` 请求头，`plat` 取自 `X-Plat` 头，`seq` 取自 `X-Seq` 头 |

#### 2.3.1 Protobuf `FY_CLIENT` 结构（`desc/proto/fy.proto`）

```protobuf
message FY_CLIENT {
  COMMON_REQ  common_req  = 1; // 请求
  COMMON_RESP common_resp = 2; // 响应
}

message CLIENT_INFO {
  string session_id   = 1; // access token，或游客生成的负数字符串
  int32  seq          = 2; // 序列号
  int32  app_ver      = 3; // 应用版本号（恒为 0；plat==0 时作为 plat 的回退）
  int32  package_code = 4; // 包 id
  int32  plat         = 5; // 平台：3=Web，4=WAP
  int32  language     = 6; // 语言：1=zh
}

message COMMON_REQ {
  CLIENT_INFO client_info = 1;
  bytes       param       = 2; // 业务 JSON
}

message COMMON_RESULT {
  int32  err_code       = 1; // 200 = 成功
  string err_msg        = 2; // 错误描述
  int32  seq            = 3; // 回显 seq
  string new_session_id = 4; // 鉴权类接口此处携带新的 access token
}

message COMMON_RESP {
  COMMON_RESULT common_result = 1;
  bytes         result        = 2; // 业务 JSON
}
```

> **关于 `plat` 字段位置的兼容性**：`fy.proto` 注释提到原 spec 文档把 `app_ver`
> 标为 field=3 且 `plat` 也标为 field=3（proto3 不允许重复）。本实现把 `plat`
> 移到 field=5，但**也读 field=3 作为回退**——即客户端如果把 plat 编码在 field=3，
> 中间件仍能识别。

#### 2.3.2 JSON 信封（legacy）

```json
{
  "sessionId": "<accessToken 或空>",
  "seq": 1,
  "plat": "3",
  "param": { /* 业务 JSON */ }
}
```

`plat` 字段接受 `"3"`/`"4"` 或 `"Web"`/`"WAP"`（大小写不敏感）。

#### 2.3.3 裸业务 JSON（debug）

直接发送业务 JSON（如 `{"loginName":"...","password":"..."}`），同时用以下 HTTP
请求头传会话/平台/序号：

```
X-Session: <accessToken>
X-Plat: 3
X-Seq: 1
Content-Type: application/octet-stream
```

> 裸 JSON 路径主要用于 smoketest 与开发期调试，**生产客户端不应使用**。

### 2.4 响应明文形态

#### 2.4.1 请求为 Protobuf 时 → 响应也是 Protobuf `FY_CLIENT`

中间件把业务 handler 写出的 `{code, meg, seq, newSessionId, result}` JSON 信封
**映射到** `FY_CLIENT.common_resp`：

| JSON 字段 | Protobuf 字段 |
|-----------|--------------|
| `code` | `common_result.err_code` |
| `meg` | `common_result.err_msg` |
| `seq` | `common_result.seq` |
| `newSessionId` | `common_result.new_session_id` |
| `result` | `common_resp.result`（bytes） |

如果 handler 没写出信封（直接写出业务 JSON），中间件会自动包成 `err_code=200`、
`result = <原 JSON>`。

#### 2.4.2 请求为 JSON 信封或裸 JSON 时 → 响应也是 JSON 信封

```json
{
  "code": 200,
  "meg": "",
  "seq": 1,
  "newSessionId": "<accessToken，仅在鉴权类接口返回>",
  "result": { /* 业务 JSON */ }
}
```

### 2.5 ⚠️ 故意保留的字段拼写错误

为与 zbyy 真实客户端完全兼容，以下字段名**故意保留错误拼写**，**不要修正**：

| 字段 | 错误拼写 | 正确拼写 | 出现位置 |
|------|---------|---------|---------|
| 错误描述 | `meg` | `msg` | JSON 信封 `{"code":200,"meg":"..."}`、Protobuf `err_msg` 仍为正确拼写 |
| 下一档成长值 | `nextMinGrom` | `nextMinGrow` | `growDto.nextMinGrom`（`UserInfoResult.GrowDTO`） |
| 错误码 | `err_code`（snake_case） | — | Protobuf 字段 |
| 新会话 id | `new_session_id`（Protobuf） / `newSessionId`（JSON） | — | 鉴权类接口携带新 access token |

### 2.6 plat=3 / plat=4 密钥选择

中间件在加密响应时按 `CLIENT_INFO.plat` 选择响应密钥：

```go
respKey := cfg.ResponseKey                       // 默认 Web 密钥
if plat == 4 && len(cfg.WapResponseKey) > 0 {
    respKey = cfg.WapResponseKey                 // WAP 密钥
}
```

**后果**：

- Web 客户端（plat=3）发的请求用 `qlCJekfRKwWkQxl7` 解响应；
- WAP 客户端（plat=4）发的请求用 `PHp1st5vEg5Ca8FH`（与请求密钥相同）解响应；
- 如果客户端没传 `plat`，`plat=0`，中间件默认用 Web 响应密钥。

### 2.7 加密接口的 curl 示例（无法直接用 curl）

由于 curl 无法做 AES-128-ECB/PKCS7 + 6 字节帧头封装，加密接口**不能直接用 curl
调通**。请使用以下任一方式：

1. **smoketest 程序**（推荐）——`cmd/smoketest/main.go` 已封装好 Protobuf 信封
   + AES 帧编解码，调用 `apipro-api` 跑通注册/登录/WAP/legacy JSON 五条路径：

   ```bash
   go run ./cmd/smoketest http://127.0.0.1:3100
   ```

2. **Postman + Pre-request Script** —— 用 `CryptoJS.AES.encrypt(plain, key,
   {mode:ECB, padding:Pkcs7})` 生成密文，拼 6 字节头部后转 base64，再用
   `application/octet-stream` POST 出去；响应侧反向操作。

3. **自写脚本** —— Go / Node / Python 任一，调用 `pkg/codec` 或对应语言的
   AES-128-ECB 实现。Go 示例：

   ```go
   import (
       "apipro/desc/proto/gen/fy"
       "apipro/pkg/codec"
       "google.golang.org/protobuf/proto"
   )

   func callAPI(url string, businessJSON []byte, plat int) (*fy.COMMON_RESP, error) {
       fcReq := &fy.FY_CLIENT{
           CommonReq: &fy.COMMON_REQ{
               ClientInfo: &fy.CLIENT_INFO{
                   SessionId: "-guest123",   // 游客或留空
                   Seq:       1,
                   Plat:      int32(plat),   // 3=Web, 4=WAP
                   Language:  1,
               },
               Param: businessJSON,
           },
       }
       plain, _ := proto.Marshal(fcReq)
       wire, _ := codec.EncodeFrame(plain, []byte("PHp1st5vEg5Ca8FH")) // 请求密钥
       resp, _ := http.Post(url, "application/octet-stream", bytes.NewReader(wire))
       body, _ := io.ReadAll(resp.Body)
       respKey := []byte("qlCJekfRKwWkQxl7")                             // Web 响应密钥
       if plat == 4 {
           respKey = []byte("PHp1st5vEg5Ca8FH")                          // WAP 响应密钥
       }
       plain2, _ := codec.DecodeFrame(body, respKey)
       var fcResp fy.FY_CLIENT
       proto.Unmarshal(plain2, &fcResp)
       return fcResp.CommonResp, nil
   }
   ```

---

## 三、鉴权机制

### 3.1 密码算法（仅支持 `pwd_type=2`）

`pwd_type=1` 未实现，调用即返回 `pwd_type 1 is unsupported; use pwd_type=2`。

**`pwd_type=2` 算法**（见 `common/auth/pwd.go`）：

```
client_md5  = lowercase_hex_md5(plain_password)              // 客户端发送的 `password`
db_password = lowercase_hex_md5(client_md5 + salt)           // 数据库存储的 `password`
salt        = base64.StdEncoding(32 random bytes) = 44 ASCII // 数据库存储的 `salt`
```

**注意**：`salt` 直接字符串拼接在 `client_md5` 后面，**不做任何分隔**。

#### 测试向量（与 backend-zero 对齐）

```
MD5Hex("qwe123")
  = "200820e3227815ed1756a6b531e7e0d2"

DBPassword("200820e3227815ed1756a6b531e7e0d2", "7Whd1U2T1pjeDP4HcSVDxwBMF5Vf6NWx")
  = "8ec733b6de4825a437faee2c01ddd309"
```

### 3.2 Token 格式（非 JWT，不透明随机 hex）

| Token 类型 | 生成方式 | 长度 | TTL |
|-----------|---------|------|-----|
| AccessToken | `hex(rand 24 bytes)` | 48 hex 字符 | 普通用户 30 分钟；游客 24 小时 |
| RefreshToken | `hex(rand 32 bytes)` | 64 hex 字符 | 普通用户 30 天；游客无 |

**`refresh` 接口做 token 轮换**：旧 access + refresh 全部删除，签发新的；refresh
窗口**不延长**（即首次登录后 30 天内必须重新登录）。

### 3.3 Redis 键

| Redis 键 | 值 | TTL | 用途 |
|----------|----|-----|------|
| `yuyan:sess:v2:<accessToken>` | JSON 序列化的 `Session` 结构体 | 用户：30 天（与 refresh 同步）；游客：24 小时 | 会话存储 |
| `yuyan:refresh:v2:<refreshToken>` | `<accessToken>` 字符串 | 同上 | refresh 反查 access，用于轮换 |
| `yuyan:login:lock:acct:<loginName>` | `"1"` | 30 分钟 | 登录失败 10 次后账号锁定 |
| `yuyan:login:fail:acct:<loginName>` | 计数 | 15 分钟 | 登录失败计数器 |
| `yuyan:login:lock:<cc>:<phone>` | `"1"` | 30 分钟 | 手机号登录锁定 |
| `yuyan:login:fail:<cc>:<phone>` | 计数 | 15 分钟 | 手机号登录失败计数器 |
| `yuyan:kaptcha:<mobile>` | 5 字符验证码 | 300 秒 | 图形验证码（一次性） |
| `yuyan:ratelimit:login` (ZSET) | 时间戳 | 滑窗 | 登录限流（10/min per loginName or phone） |
| `yuyan:ratelimit:cooldown:sms:<cc>:<phone>` | `"1"` | 60 秒 | SMS 发送冷却 |
| `yuyan:ratelimit:sms:<cc>:<phone>:hour` | 计数 | 1 小时 | SMS 每小时上限（10/hour） |
| `yuyan:chat:message_id` | 计数 | — | 聊天消息自增 id |
| `yuyan:live:chat:<roomNum>` | LIST | 24 小时 | 聊天历史（LPUSH + LTRIM 0 49） |

### 3.4 Token 在请求中如何携带

**加密接口**（POST `/login/*`、`/live/*` 等）—— 三选一：

| 请求形态 | Token 位置 |
|---------|-----------|
| Protobuf `FY_CLIENT` | `CLIENT_INFO.session_id` 字段 |
| JSON 信封 | `sessionId` 字段 |
| 裸业务 JSON | HTTP 请求头 `X-Session: <accessToken>` |

**WebSocket** —— 通过 LOGIN 帧（opcode 1001）的 `key` 字段携带，详见
[§十 聊天 API](#十聊天-apiwebsocket)。

> **重要**：本 API **不使用 `Authorization: Bearer` 头**。任何带 Bearer 头的
> 请求都不会被识别。

### 3.5 Session 结构（Redis 中存储）

```go
type Session struct {
    AccessToken     string `json:"accessToken"`
    RefreshToken    string `json:"refreshToken,omitempty"`
    UID             int64  `json:"uid"`
    NickName        string `json:"nickName"`
    Icon            string `json:"icon"`
    UserType        int    `json:"userType"`        // 1=audience, 2=anchor, 3=admin
    DeviceType      string `json:"deviceType"`      // "android"|"ios"|"pc"|"h5"
    Plat            int    `json:"plat"`            // 1=android 2=ios 3=pc 4=h5
    IsGuest         bool   `json:"isGuest"`
    AccessExpireAt  int64  `json:"accessExpireAt"`  // unix seconds
    RefreshExpireAt int64  `json:"refreshExpireAt,omitempty"` // 0 for guest
}
```

---

## 四、图形验证码 API

### 4.1 `GET /api/kaptcha`

返回 SVG 图片验证码（明文，**不加密**）。

**Query 参数**：

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `mobile` | string | 是 | 手机号或登录名，用于服务端按 `yuyan:kaptcha:<mobile>` 索引验证码 |
| `t` | int | 否 | 时间戳，仅作为客户端缓存破坏参数，服务端忽略 |

**响应**：

- `200 OK`，`Content-Type: image/svg+xml; charset=utf-8`，`Cache-Control: no-store, no-cache, must-revalidate`
- Body 为一段 SVG，形如：

  ```xml
  <svg xmlns="http://www.w3.org/2000/svg" width="120" height="40">
    <rect width="120" height="40" fill="#f0f0f0"/>
    <text x="12" y="28" font-family="monospace" font-size="22" fill="#333">VXH4U</text>
  </svg>
  ```

- `mobile` 缺失时返回 `400 Bad Request`，body 为 `missing mobile`。

**字符集**：`ABCDEFGHJKLMNPQRSTUVWXYZ23456789`（32 字符，剔除易混淆的 `0`/`O`/`I`/`1`）。

**长度**：5 字符（由 yaml `KaptchaCodeLen` 控制，默认 5）。

**TTL**：300 秒（由 yaml `KaptchaTTL` 控制）。

**一次性**：注册/找回流程校验后**立即删除** Redis 键，第二次使用必失败。

### 4.2 curl 示例

```bash
# 拉取验证码（保存为 SVG 文件，浏览器打开看明文）
curl -sS "http://127.0.0.1:3100/api/kaptcha?t=$(date +%s%3N)&mobile=xiaocainiao001" \
  -o /tmp/kaptcha.svg

# 在 /tmp/kaptcha.svg 中读出 5 字符验证码（例如 VXH4U），随后用于注册请求
```

> smoketest 程序 (`cmd/smoketest/main.go`) 直接解析 SVG `<text>` 标签提取验证码。

---

## 五、注册 / 登录 API

### 5.1 `POST /login/reg` — 注册

支持两种注册方式：

- `accountType=2` **账号注册**：仅需 `loginName` + `password` + `nickName` + `kaptcha`，**不发短信**；
- `accountType=1` **手机号注册**：需 `phone` + `countryCode` + `smsCode` + `kaptcha` + `password` + `nickName`。

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `accountType` | int | 是 | `2`=账号注册，`1`=手机号注册 |
| `loginName` | string | accountType=2 必填 | 账号名（≥1 字符） |
| `phone` | string | accountType=1 必填 | 手机号 |
| `countryCode` | string | accountType=1 必填 | 国家码，如 `+86` 或 `86` |
| `password` | string | 是 | 客户端 md5（不是明文密码），如 `200820e3227815ed1756a6b531e7e0d2` |
| `nickName` | string | 是 | 昵称（≥ 2 个 Unicode 字符） |
| `pwdType` | int | 是 | **必须为 `2`**；`1` 不支持 |
| `kaptcha` | string | 是 | 图形验证码，5 字符，从 `GET /api/kaptcha?mobile=<loginName 或 phone>` 获取 |
| `smsCode` | string | accountType=1 必填 | 短信验证码；dev 模式下 `8888` 通用 |
| `smsType` | int | accountType=1 可选 | 短信类型（注册场景恒为 1） |
| `icon` | string | 否 | 头像 URL |
| `plat` | int | 否 | 平台：`3`=Web，`4`=WAP；同时也会从 `CLIENT_INFO.plat` 取 |

#### 请求示例（业务 JSON，需先 AES 帧封装）

```json
{
  "accountType": 2,
  "loginName": "xiaocainiao001",
  "password": "200820e3227815ed1756a6b531e7e0d2",
  "nickName": "小菜鸟",
  "pwdType": 2,
  "kaptcha": "VXH4U"
}
```

#### 响应 `result` JSON —— `AuthResponse` 结构

```json
{
  "accessToken": "9f1dc3637a5360da6dd54024c0a0be6a257d8af612724f7f",
  "sessionId": "9f1dc3637a5360da6dd54024c0a0be6a257d8af612724f7f",
  "refreshToken": "<64 hex chars>",
  "userInfo": {
    "uid": 100001,
    "nickName": "小菜鸟",
    "icon": "",
    "cutOutIcon": "",
    "userType": 1,
    "score": 0,
    "grow": 0,
    "growDto": { "id": 1, "name": "新手", "nextMinGrom": 100 },
    "gender": 0,
    "birthday": 0,
    "loginName": "xiaocainiao001",
    "phone": "",
    "countryCode": ""
  },
  "urls": {},
  "phone": "",
  "countryCode": 0,
  "loginName": "xiaocainiao001"
}
```

> **`accessToken` 与 `sessionId` 永远相等**——两者都指向同一个 access token。客户端
> 用哪个都行。

#### ⚠️ SPEC CHECK —— `new_session_id` 必须等于 `result.sessionId`

鉴权类接口（`login` / `register` / `guestLogin` / `refresh`）的 RPC 层会从
`result.sessionId` 提取新 token，**覆盖** `common_result.new_session_id`：

```go
// cmd/rpc/internal/logic/call.go L122-139
if code == CodeOK && isAuthMethod(method) {
    if newSID := extractResultSessionID(result); newSID != "" {
        resp.NewSessionId = newSID
    }
}
```

即 **Protobuf 响应的 `common_result.new_session_id` 与 `common_resp.result.sessionId`
必然相等**，都携带服务器签发的新 access token。smoketest 在 `[2]` 与 `[3]` 步
显式校验此不变量，输出 `SPEC CHECK: new_session_id == result.sessionId ✓`。

#### 错误码

| code | 触发场景 |
|------|---------|
| 200 | 注册成功 |
| 400 | 参数缺失 / 昵称 < 2 字符 / uid 冲突 / 数据库写入失败 |
| 4104 | `accountType=2`：loginName 已被注册；`accountType=1`：phone 已被注册 |
| 4114 | `accountType=1`：手机号格式错误 |
| 4106 | `accountType=1`：短信验证码错误 |
| 4120 | 图形验证码错误或已过期 |
| 4113 | 操作过于频繁（限流） |

### 5.2 `POST /login/login` — 登录

支持账号登录与手机号登录。

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `accountType` | int | 是 | `2`=账号登录，`1`=手机号登录 |
| `loginName` | string | accountType=2 必填 | 账号名 |
| `phone` | string | accountType=1 必填 | 手机号 |
| `countryCode` | string | accountType=1 必填 | 国家码 |
| `password` | string | 是 | 客户端 md5 |
| `pwdType` | int | 是 | **必须为 `2`** |
| `loginMode` | int | 是 | **必须为 `1`** |
| `loginType` | int | 是 | **必须为 `1`** |
| `plat` | int | 否 | 平台，从 `CLIENT_INFO.plat` 优先取 |

> `loginMode != 1` 或 `loginType != 1` 时直接返回 `400 "登录失败"`。

#### 请求示例

```json
{
  "accountType": 2,
  "loginName": "xiaocainiao001",
  "password": "200820e3227815ed1756a6b531e7e0d2",
  "pwdType": 2,
  "loginMode": 1,
  "loginType": 1
}
```

#### 响应

与 `/login/reg` 相同的 `AuthResponse` 结构（§5.1）。

#### 登录失败锁定策略

按 `loginName`（账号登录）或 `<cc>:<phone>`（手机号登录）计数：

- 失败 1 次：写入 `yuyan:login:fail:acct:<loginName>`，TTL 15 分钟；
- 累计 ≥ 10 次：写 `yuyan:login:lock:acct:<loginName>` = `"1"` TTL 30 分钟，并删除计数器；
- 锁定期内再次登录直接返回 `4105 "账号已锁定，请30分钟后再试"`。

#### 错误码

| code | 触发场景 |
|------|---------|
| 200 | 登录成功 |
| 400 | 参数缺失 / `loginMode`/`loginType` 不对 / `pwdType != 2` |
| 4101 | 账号未注册 |
| 4102 | 密码错误（含 pwd_type 不匹配） |
| 4103 | 账号已封禁 |
| 4105 | 账号已锁定（30 分钟） |
| 4113 | 限流（10 次/min per loginName/phone） |
| 4114 | 手机号格式错误 |

### 5.3 `POST /login/guestLogin` — 游客登录

#### 请求 `param` JSON

```json
{ "plat": 3 }
```

`plat` 也可省略，由 `CLIENT_INFO.plat` 决定。

#### 响应

`AuthResponse` 结构，但：

- `refreshToken` 为空；
- `accessToken` TTL = 24 小时（非 30 分钟）；
- `userInfo.uid = 0`、`userInfo.userType = 1`（audience，但 IsGuest=true）；
- `userInfo.gender = 3`（other）；
- `userInfo.nickName` = 服务端随机生成的 6 位大写 hex，如 `A1F4C9`。

#### SPEC CHECK

`common_result.new_session_id` 仍等于 `result.sessionId`（游客 token）。

### 5.4 `POST /login/refresh` — 刷新 Token

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `refreshToken` | string | 是 | 64 hex 字符的 refresh token |

#### 响应

新的 `AuthResponse`（**access + refresh 全部轮换**，旧的立即失效）。Refresh 窗口
**不延长**——即从首次登录算起 30 天后必须重新登录。

#### 错误码

| code | 触发场景 |
|------|---------|
| 200 | 刷新成功 |
| 400 | refresh denied（游客、refresh 不存在或已过期） |
| 4103 | 用户已封禁（refresh 时回查用户状态） |

### 5.5 `POST /login/logout` — 登出

#### 请求 `param` JSON

```json
{}
```

`session_id` 从加密信封的 `CLIENT_INFO.session_id` 取（不需要在 `param` 里再传）。

#### 响应

```json
{}
```

`code=200`，`newSessionId` 与传入的 `session_id` 相同（非鉴权类方法不做 token
轮换，仅 echo）。

#### 行为

删除 Redis 中的 `yuyan:sess:v2:<accessToken>` 与 `yuyan:refresh:v2:<refreshToken>`
（如果有）。session 不存在也不报错。

### 5.6 `POST /sys/getSmsCode` — 发送短信验证码

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `countryCode` | string | 是 | 国家码，如 `+86` |
| `phone` | string | 是 | 手机号 |
| `type` | int | 是 | `1`=注册，`2`=忘记密码，`3`=修改，`4`=绑定 |

#### 请求示例

```json
{
  "countryCode": "+86",
  "phone": "13800138000",
  "type": 1
}
```

#### 响应

```json
{}
```

`code=200`。

#### Dev 模式特殊行为

`AppMode: dev`（yaml）下，**不发真实短信**，校验时直接接受 `SmsDevBypassCode`
（yaml 配置，默认 `"8888"`）作为通过码。生产环境 `AppMode: prod` 下必须接入真实
短信网关（当前实现里 `smsStore.Issue` 仅落 Redis，无真实短信发送，需自行扩展）。

#### 限流

| 限制 | 配置 |
|------|------|
| 同一手机号 60 秒冷却 | Redis `SETNXEX yuyan:ratelimit:cooldown:sms:<cc>:<phone>` |
| 同一手机号每小时 10 条 | Redis `INCR yuyan:ratelimit:sms:<cc>:<phone>:hour` |

#### 错误码

| code | 触发场景 |
|------|---------|
| 200 | 发送成功（或 dev 模式直接返回） |
| 4104 | `type=1` 且手机号已注册 |
| 4113 | 60 秒内重复发送 / 1 小时超 10 条 |
| 4114 | 手机号格式错误 |

---

## 六、比赛 API

### 6.1 `POST /match/recommend` — 推荐赛事（加密）

返回今日 + 未来热门赛事列表。

#### 请求 `param` JSON

```json
{}
```

#### 响应 `result` JSON

```json
{
  "count": 3,
  "pageNum": 1,
  "matches": [
    {
      "scheduleId": 2025080801,
      "matchTime": 1723108800000,
      "hostName": "曼联",
      "guestName": "利物浦",
      "hostIcon": "https://sta.ncctrials.com/file/team/man.png",
      "guestIcon": "https://sta.ncctrials.com/file/team/liv.png",
      "subCateName": "英超",
      "categoryId": 1,
      "categoryName": "足球",
      "categoryIcon": "",
      "hostScore": 0,
      "guestScore": 0,
      "matchStatus": 0,
      "status": 1,
      "matchStatusDesc": "",
      "anchors": [
        {
          "uid": 1001,
          "nickName": "飞鱼解说",
          "icon": "https://...",
          "cutOutIcon": "https://...",
          "anchor": {
            "roomNum": "1001",
            "detail": "英超焦点战解说",
            "notice": "欢迎来到直播间"
          }
        }
      ]
    }
  ]
}
```

#### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `count` | int | 返回的赛事总数 |
| `pageNum` | int | 当前页码，恒为 `1` |
| `matches` | array | `MatchCatalogItem` 数组，最多 8 条 |
| `matchTime` | int64 | 比赛开始时间，**毫秒级** UTC 时间戳 |
| `categoryId` | int64 | 比赛大类 ID（= live_type 父类 ID）：`1`=足球，`2`=篮球，`5`=斯诺克 |
| `status` | int | 恒为 `1`（数据有效） |
| `matchStatusDesc` | string | 恒为 `""`（不发明文案） |
| `anchors` | array | 解说员列表（同一场比赛可能有多位解说） |

#### 缓存

Redis `cache:match:recommend`，TTL 60 秒。

### 6.2 `POST /match/cateList` — 比赛分类列表（加密）

**实现上是 `/match/recommend` 的别名**（`call.go` 里 `match_cateList` 直接调用
`handleMatchRecommend`），返回结构完全相同。

#### 请求 `param` JSON

```json
{}
```

#### 响应

同 §6.1。

### 6.3 `POST /match/detail` — 比赛详情（加密）

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `scheduleId` | int64 | 是 | 赛程 ID |
| `roomNum` | string | 否 | 指定直播间号（用于上下文过滤，当前实现未严格使用） |

```json
{ "scheduleId": 2025080801, "roomNum": "1001" }
```

#### 响应 `result` JSON

```json
{
  "match": {
    "scheduleId": 2025080801,
    "matchTime": 1723108800000,
    "hostName": "曼联",
    "guestName": "利物浦",
    "hostIcon": "...",
    "guestIcon": "...",
    "subCateName": "英超",
    "categoryIcon": ""
  },
  "rooms": [
    {
      "roomNum": "1001",
      "title": "英超焦点战",
      "cover": "...",
      "cutOutCustomCoverUrl": "",
      "markType": 0,
      "liveStatus": 1,
      "hd": 1,
      "viewCount": 38211,
      "focusCount": 98000,
      "anchor": {
        "uid": 1001,
        "nickName": "飞鱼解说",
        "icon": "...",
        "cutOutIcon": "...",
        "roomNum": "1001",
        "notice": "..."
      }
    }
  ]
}
```

> 注意：`match` 字段使用 `MatchItem` 结构（8 字段，比 `MatchCatalogItem` 简化），
> 与 backend-zero 行为对齐。`rooms` 字段是此赛程关联的直播间列表（最多 50 条）。

#### 缓存

Redis `cache:match:detail:<scheduleId>`，TTL 60 秒。

#### 错误码

| code | 触发场景 |
|------|---------|
| 200 | 成功 |
| 400 | `scheduleId` 缺失 / 未找到该赛程 |

### 6.4 `GET /matches.json` 和 `GET /match_all.json` — 全部赛事分组（JSONP）

两个路径都调用 `matches_jsonp` RPC，**完全等价**。返回按比赛大类分组的赛事列表。

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `callback` | string | 否 | JSONP 回调函数名；不传则使用默认 `matches`；传空值 `?callback=` 则返回裸 JSON |

#### 响应（默认 JSONP 包裹）

```
matches({"data":{
  "0":  [ /* 合并所有，按 scheduleId 去重，按 matchTime 升序，最多 30 条 */ ],
  "1":  [ /* 足球，最多 30 条 */ ],
  "2":  [ /* 篮球，最多 30 条 */ ],
  "5":  [ /* 斯诺克，最多 30 条 */ ],
  "hot":[ /* 推荐，最多 8 条 */ ]
}})
```

`?callback=` 时返回裸 JSON `{"data":{...}}`，`?callback=foo` 时返回
`foo({"data":{...}})`。

> `"0"` 是合并所有大类的"全部"分组，不是大类 0。每个 `MatchCatalogItem` 与
> §6.1 相同结构。

#### 缓存 + 快照

- Redis `cache:match:catalog`，TTL 60 秒；
- 同时落盘到 `<JsonpSnapshotDir>/matches.json`（默认 `./data/jsonp/matches.json`），
  内容为 `matches(<JSON>)` 形式，供静态文件服务直接吐出。

#### curl 示例

```bash
# 裸 JSON
curl -sS "http://127.0.0.1:3100/matches.json?callback=" | jq .

# 自定义回调
curl -sS "http://127.0.0.1:3100/matches.json?callback=myCb"
```

### 6.5 `GET /match_recommend.json` — 推荐赛事（JSONP）

JSONP 版本的 `/match/recommend`，调用同一 RPC。

#### Query 参数

同 §6.4，默认回调 `match_recommend`。

#### 响应

```
match_recommend({"data":{
  "count": 3,
  "pageNum": 1,
  "matches": [ /* 同 §6.1 */ ]
}})
```

### 6.6 `GET /match/matches_<YYYYMMDD>.json` — 按日期查比赛（JSONP，平铺数组）

**仅匹配 `matches_<YYYYMMDD>.json` 文件名模式**，否则 404。`YYYYMMDD` 必须 8 位
数字。

#### 路径示例

```
GET /match/matches_20250808.json
GET /match/matches_20250101.json?callback=foo
```

#### 响应

**平铺数组**（不像其他 JSONP 接口那样包 `{"data":...}`）：

```
matches_20250808([
  { "scheduleId":..., "matchTime":..., "hostName":..., ... },
  { ... }
])
```

`?callback=` 时返回裸数组 `[...]`，`?callback=foo` 时返回 `foo([...])`。

#### 默认回调名

`matches_<YYYYMMDD>`（即文件名去掉 `.json`）。

### 6.7 比赛 API 错误码

| code | 触发场景 |
|------|---------|
| 200 | 成功 |
| 400 | 参数缺失 / `scheduleId` 不存在 / 日期格式错误 |

---

## 七、直播 API

### 7.1 `POST /live/hot` — 热门直播间（加密）

#### 请求 `param` JSON

```json
{}
```

#### 响应 `result` JSON

```json
{
  "hot": [
    {
      "roomNum": "1001",
      "title": "英超焦点战",
      "cover": "https://...",
      "cutOutCustomCoverUrl": "",
      "markType": 0,
      "liveStatus": 1,
      "hd": 1,
      "viewCount": 38211,
      "focusCount": 98000,
      "anchor": {
        "uid": 1001,
        "nickName": "飞鱼解说",
        "icon": "https://...",
        "cutOutIcon": "https://...",
        "roomNum": "1001",
        "notice": "..."
      }
    }
  ]
}
```

`viewCount = DB.VisitCount + DB.FictitiousVisitCount`（真实访问 + 虚拟访问）；
`focusCount` 同理。最多返回 50 条。

### 7.2 `POST /live/cateList` — 按直播分类查直播间（加密）

> ⚠️ **不是直播分类目录**。直播分类目录在 `/live_types.json`（§7.5）。
> 本接口按 `liveTypeId` 过滤直播间。

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `liveTypeId` | int64 | 是 | 直播分类 ID（来自 `/live_types.json` 的 `liveTypeId` 字段） |

```json
{ "liveTypeId": 1 }
```

#### 响应 `result` JSON

```json
{
  "rooms": [
    { /* RoomResult，同 §7.1 的 hot 数组元素 */ }
  ]
}
```

最多 50 条。`liveTypeId=0` 或缺失时返回 `400 "missing liveTypeId"`。

### 7.3 `POST /live/detail` — 直播间详情（加密）

#### 请求 `param` JSON

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `roomNum` | string | 是 | 直播间号 |
| `anchorId` | int64 | 否 | 主播 uid（当前实现未严格使用） |

```json
{ "roomNum": "1001", "anchorId": 1001 }
```

#### 响应 `result` JSON

返回 `{"room": RoomDetailResult, "stream": PlayStreamURLs}`：

```json
{
  "room": {
    "roomNum": "1001",
    "title": "英超焦点战",
    "contact": "微信:xxx",
    "hd": 1,
    "cover": "...",
    "notice": "欢迎来到直播间",
    "detail": "英超焦点战解说",
    "liveFlv": "https://.../live.flv",
    "liveM3u8": "https://.../live.m3u8",
    "liveStatus": 1,
    "viewCount": 38211,
    "focusCount": 98000,
    "anchor": { /* AnchorResult */ }
  },
  "stream": {
    "flv":   "https://.../live.flv",
    "hdFlv": "https://.../live.flv",
    "m3u8":   "https://.../live.m3u8",
    "hdM3u8": "https://.../live.m3u8"
  }
}
```

> 注意：`stream.hdFlv` 与 `flv` 当前实现**指向同一 URL**（hd 字段在 DB 中未单独
> 维护），`stream.hdM3u8` 与 `m3u8` 同理。

`roomNum` 缺失或未找到时返回 `400 "missing roomNum"` / `400 "room not found"`。

### 7.4 `GET /all_live_rooms.json` — 全部直播间分组（JSONP）

调用 `live_all_rooms` RPC。返回按直播分类分组的所有可见直播间。

#### Query 参数

同 §6.4，默认回调 `all_live_rooms`。

#### 响应

```
all_live_rooms({"data":{
  "0":   [ /* 全部可见，最多 200 条 */ ],
  "hot": [ /* 热门，最多 50 条 */ ],
  "1":   [ /* liveTypeId=1 的直播间，最多 50 条 */ ],
  "2":   [ /* liveTypeId=2 的直播间，最多 50 条 */ ]
  /* ... 每个顶层 liveType 都有一个键 ... */
}})
```

> **键是动态的**：除固定的 `"0"`（全部）与 `"hot"`（热门）外，每个顶层直播分类
> 都按其 `liveTypeId` 生成一个键（如 `"1"`、`"2"`、`"3"`...）。具体哪些键存在
> 取决于数据库 `live_type` 表里的顶层分类。

#### curl 示例

```bash
curl -sS "http://127.0.0.1:3100/all_live_rooms.json?callback=" | jq .
```

### 7.5 `GET /live_types.json` — 直播分类目录（JSONP）

调用 `live_types` RPC，返回顶层直播分类列表。

#### Query 参数

同 §6.4，默认回调 `live_types`。

#### 响应

```
live_types({"data":[
  { "liveTypeId": 1, "typeName": "足球", "parentId": 0, "icon": "https://..." },
  { "liveTypeId": 2, "typeName": "篮球", "parentId": 0, "icon": "https://..." }
]})
```

注意是**数组**（不是 `{list:[...]}`），字段为 `liveTypeId` / `typeName` /
`parentId` / `icon`。仅返回 `parentId = 0` 的顶层分类。

#### 缓存

Redis `cache:live:types`，TTL 15 秒。

### 7.6 `GET /hot_anchor.json` — 热门主播（JSONP，shape-converted）

调用 `live_hot` RPC，但在 handler 层**做了字段重映射**以匹配 zbyy 客户端期望的
`anchors` 数组结构。

#### Query 参数

同 §6.4，默认回调 `hot_anchor`。

#### 响应

```
hot_anchor({"data":{
  "anchors": [
    {
      "nickName": "飞鱼解说",
      "icon": "https://...",
      "anchor": { "roomNum": "1001" }
    }
  ]
}})
```

**注意**：handler 内部用匿名 struct 解构了 `live_hot` 的 `{hot:[{title, anchor:{nickName, icon, roomNum}}]}`，
然后只取 `anchor` 子对象的 `nickName`、`icon`、`roomNum` 重组成上面的形状。源代码
见 `cmd/api/internal/handler/jsonp.go::HotAnchorJSONPHandler`。

> ⚠️ 当前实现有一个细微问题：源 `src.Hot[].NickName` 字段映射到 JSON `"title"`
> 字段（看似错位），但实际仅 `anchor.nickName` 被输出，因此最终响应里的
> `nickName` 来自 `live_hot` 返回的 `anchor.nickName`，**与 `/live/hot` POST
> 返回的 `anchor.nickName` 一致**。客户端正常使用没有影响。

---

## 八、房间 API

房间 API 都是 **JSONP GET** 接口，路径参数为 `roomNum`。

### 8.1 `GET /room/:roomNum/detail.json` — 房间详情（JSONP）

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `callback` | string | 否 | JSONP 回调，默认 `detail` |

#### 响应

```
detail({"data":{
  "room":   { /* RoomDetailResult，同 §7.3 的 room 字段 */ },
  "stream": { /* PlayStreamURLs，同 §7.3 的 stream 字段 */ }
}})
```

> 注意：与 §7.3 `/live/detail` POST 一样，响应是 `{room, stream}` 两层结构，
> **不是直接平铺的 RoomDetailResult**。

#### 缓存

Redis `cache:room:detail:<roomNum>`，TTL 30 秒。

### 8.2 `GET /room/:roomNum/schedule.json` — 房间赛程（JSONP）

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `callback` | string | 否 | JSONP 回调，默认 `schedule_<roomNum>`（如 `schedule_1001`） |

#### 响应

```
schedule_1001({"data":{
  "matches": [
    {
      "scheduleId": 2025080801,
      "matchTime": 1723108800000,
      "hostName": "曼联",
      "guestName": "利物浦",
      "hostIcon": "...",
      "guestIcon": "...",
      "subCateName": "英超",
      "categoryIcon": ""
    }
  ]
}})
```

`MatchItem` 是 `MatchCatalogItem` 的简化版，仅 8 个字段。最多 50 条，按
`scheduleId` 去重。

#### 缓存

Redis `cache:room:schedule:<roomNum>`，TTL 60 秒。

### 8.3 `GET /room/:roomNum/gift_rank.json` — 礼物榜（JSONP）

#### Query 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `callback` | string | 否 | JSONP 回调，默认 `gift_rank` |

#### 响应

```
gift_rank({"data":[
  {
    "user": {
      "uid": 100001,
      "nickName": "球迷老王",
      "icon": "https://...",
      "cutOutIcon": "",
      "userType": 1,
      "score": 0,
      "grow": 0,
      "growDto": { "id": 0, "name": "", "nextMinGrom": 0 },
      "gender": 0,
      "birthday": 0,
      "loginName": "",
      "phone": "",
      "countryCode": ""
    },
    "contribution": 18820
  }
]})
```

> 注意：响应 `data` 是**裸数组**，每个元素是 `{user: UserInfoResult, contribution: int64}`，
> 不是 `{list:[...]}` 也不是平铺的 `{uid, nickName, score, rank}`。仅前 10 名。
> `user` 字段是完整的 `UserInfoResult`（与 §5.1 中的 `userInfo` 同结构），但
> `gift_rank` 实现里只填了 `uid`/`nickName`/`icon` 三个字段，其它为零值。

#### 缓存

Redis `cache:room:gift_rank:<roomNum>`，TTL 30 秒。

---

## 九、用户 API

### 9.1 `POST /user/detail` — 当前用户详情（加密）

#### 请求 `param` JSON

```json
{}
```

`session_id` 从加密信封的 `CLIENT_INFO.session_id` 取。

#### 鉴权要求

- `session_id` 不能为空，否则返回 `100 "login required"`；
- session 不能是游客，否则返回 `100 "login required"`；
- 用户状态 `Status != 1` 时返回 `4103 "账号已封禁"`。

#### 响应 `result` JSON

```json
{
  "user": {
    "uid": 100001,
    "nickName": "小菜鸟",
    "icon": "",
    "cutOutIcon": "",
    "userType": 1,
    "score": 0,
    "grow": 0,
    "growDto": { "id": 1, "name": "新手", "nextMinGrom": 100 },
    "gender": 0,
    "birthday": 0,
    "loginName": "xiaocainiao001",
    "phone": "",
    "countryCode": ""
  }
}
```

> 注意：响应是 `{user: UserInfoResult}`，**不是平铺的 UserInfoResult**。与 §5.1
> 中 `AuthResponse.userInfo` 字段同结构。

#### 错误码

| code | 触发场景 |
|------|---------|
| 100 | 未登录 / 游客 session |
| 200 | 成功 |
| 400 | 用户记录不存在 |
| 4103 | 用户已封禁 |

---

## 十、聊天 API（WebSocket）

### 10.1 连接

```
GET /ws/chat
```

WebSocket 升级，**不需要 query 参数**。鉴权在升级后通过 LOGIN 帧（opcode 1001）
完成。`CheckOrigin` 永远返回 `true`（允许跨域）。

#### 连接限制

- 读缓冲 4 KB，写缓冲 4 KB；
- 单条消息上限 `maxMsgLen + 256` 字节（默认 756 字节）；
- 心跳超时 60 秒（`defaultHeartbeatTimeout * 2`）；
- 写超时 10 秒；
- 发送队列 256 条（满了直接踢掉慢客户端）。

### 10.2 二进制帧格式

WebSocket BinaryMessage 帧格式（**8 字节头部 + 密文**，比 HTTP 帧多了 2 字节 opcode）：

```
偏移  长度  字段
0     2     帧魔数 = 0x00 0xA0
2     2     uint16 BE  opcode（如 1001=LOGIN，1005=COMMENT，详见 §10.4）
4     4     uint32 BE  body 长度（密文长度，可为 0）
8     N     密文（AES-128-ECB/PKCS7 加密的 JSON body；body 长度为 0 时无密文）
```

加密密钥与 HTTP 接口相同（`reqKey` 解客户端→服务端，`respKey` 加服务端→客户端，
WAP 客户端用 `ApiKeyRespWap`）。

### 10.3 兼容 JSON 文本帧

服务器**同时接受** TextMessage 帧，格式为 JSON：

```json
{ "op": 1001, "key": "<accessToken>", "plat": 4, "version": 1 }
{ "op": 1003, "roomNum": "1001", "haveHistory": 1 }
{ "op": 1005, "roomNum": "1001", "content": "hi" }
{ "op": 1000 }
```

`op` 缺失时按 `1005`（COMMENT）处理。这种模式主要用于兼容旧版 `public/chat.html`
页面。ACK 帧仍以二进制形式发送。

### 10.4 Opcode 表

#### 客户端 → 服务端

| Opcode | 名称 | Body | 说明 |
|--------|------|------|------|
| 1000 | `HEART` | 空 | 心跳 |
| 1001 | `LOGIN` | `{"key":"<accessToken>","plat":4,"version":1}` | 鉴权；`key` 缺失或 session 无效返回 `9999/1003 "invalid session"` |
| 1002 | `LOGOUT` | 空 | 登出并断开 |
| 1003 | `ROOM_ENTER` | `{"roomNum":"1001","haveHistory":1}` | 进房；`haveHistory=1` 时回放最近 50 条消息 |
| 1004 | `ROOM_LEAVE` | `{"roomNum":"1001"}` | 离房 |
| 1005 | `COMMENT` | `{"roomNum":"1001","content":"hi","msgType":1}` | 发消息；`msgType` 默认 1（文本） |
| 1006 | `COMMENT_DELETE` | `{"roomNum":"...","msgId":...}` | 撤回（当前实现未完整接入） |
| 1007 | `LIKE` | `{"roomNum":"1001"}` | 点赞（legacy 别名） |
| 1010 | `SESSION_RESUME` | `{"key":"<newAccessToken>"}` | 切换 token 但不离开房间（M-AUTH-007） |

#### 服务端 → 客户端

| Opcode | 名称 | Body | 说明 |
|--------|------|------|------|
| 2000 | `HEART_SUCCESS` | 空 | 心跳 ACK |
| 2001 | `LOGIN_SUCCESS` | `{}` | 登录成功 |
| 2002 | `LOGOUT_SUCCESS` | `{}` | 登出 ACK |
| 2003 | `ROOM_ENTER_SUCCESS` | `{}` | 进房 ACK |
| 2004 | `ROOM_LEAVE_SUCCESS` | `{}` | 离房 ACK |
| 2005 | `COMMENT_SUCCESS` | `{}` | 发消息 ACK（仅发送者收到） |
| 2006 | `COMMENT_DELETE_SUCCESS` | — | 撤回 ACK |
| 2007 | `LIKE_SUCCESS` | `{"sendUid":...,"roomNum":"..."}` | 点赞广播（房间内所有人收到） |
| 2010 | `SESSION_RESUME_SUCCESS` | `{}` | token 切换 ACK |
| 3000 | `COMMENT_PUSH` | 见 §10.5 | 聊天消息推送（房间内所有人） |
| 3001 | `COMMENT_PUSH_DELETE` | — | 撤回推送 |
| 3002 | `SYS_PUSH` | — | 系统推送 |
| 3003 | `SCOREGrow_PUSH` | — | 比分变化推送 |
| 3004 | `SUBSCRIBE_MATCH_PUSH` | — | 订阅赛事推送 |
| 3005 | `VIEW_NUM_PUSH` | `{"roomNum":"1001","viewNum":...}` | 在线人数推送 |
| 3006 | `OFFLINE_PUSH` | — | 离线推送 |
| 3007 | `SUBSCRIBE_LIST_PUSH` | — | 订阅列表推送 |
| 3008 | `FEEDBACK_PUSH` | — | 反馈推送 |
| 3009 | `MSG_PUSH` | — | 普通消息推送 |
| 9999 | `ERROR` | `{"code":1003,"msg":"..."}` | 错误帧 |

### 10.5 聊天消息推送 body（opcode 3000）

```json
{
  "sendUid": 100001,
  "roomNum": "1001",
  "msgType": 1,
  "sendTime": 1723108800000,
  "content": "hi",
  "sendUser": {
    "uid": 100001,
    "nickName": "球迷老王",
    "icon": "https://..."
  },
  "msgId": 1234567
}
```

`msgId` 来自 Redis INCR `yuyan:chat:message_id`，全局自增。`sendTime` 为毫秒级 UTC。

### 10.6 系统通知 body（opcode 3000，msgType=3）

进房时如果房间未开播，会推送：

```json
{
  "roomNum": "1001",
  "msgType": 3,
  "noticeType": 1,
  "content": "未开播直播间不可以发言",
  "sendTime": 1723108800000
}
```

进房时如果是非游客首次进入已开播房间，会广播给房间内所有人：

```json
{
  "roomNum": "1001",
  "msgType": 3,
  "noticeType": 2,
  "content": "<nickName>进入直播间",
  "sendTime": 1723108800000
}
```

### 10.7 WS 错误码（在 opcode 9999 body 内）

| code | 含义 |
|------|------|
| 1000 | `ErrCodeNotInChatServer` 不在聊天服务器 / 房间号无效 |
| 1002 | `ErrCodeSensitiveWord` 敏感词 |
| 1003 | `ErrCodeUnauthorized` 未登录 / 鉴权失败 |
| 1004 | `ErrCodeMuted` 被禁言（房间未开播、IP 黑名单、限流） |
| 1005 | `ErrCodeAccountDisabled` 账号已封禁 |

### 10.8 历史消息回放

进房时如果 `haveHistory=1`，按顺序回放最近 50 条消息：

1. 先从 Redis LIST `yuyan:live:chat:<roomNum>` 取（LPUSH newest first，所以
   `LRANGE 0 49` 即最近 50 条倒序）；
2. Redis 为空时回退到 MySQL `chat_room_message` 表查最近 50 条；
3. 回放以 3000 帧逐条发送给该客户端（不广播给房间其他人）。

### 10.9 聊天限流

- 单客户端 60 条/分钟（`ChatRatePerMin`，yaml 可配）；超过返回 `9999/1004 "rate limit"`；
- 单条消息最长 500 字符（`ChatMaxMsgLen`），超出截断；
- 历史保留 50 条/房间（`ChatHistoryLim`）。

### 10.10 静态聊天页面

`GET /chat.html` 返回 494 行的演示页面，已内置登录/进房/发消息/心跳逻辑，使用
JSON 文本帧协议（§10.3）。可用浏览器直接打开 `http://127.0.0.1:3100/chat.html`
体验。

---

## 十一、错误码

### 11.1 业务错误码（出现在加密信封的 `code` / `err_code` 字段）

| code | 名称 | 中文消息 | HTTP 含义 |
|------|------|---------|----------|
| 100 | `CodeLoginRequired` | `login required` | 未登录 |
| 101 | `CodeGuestReauth` | （中间件内部用） | 加密请求体无效 |
| 200 | `CodeOK` | （空） | 成功 |
| 400 | `CodeBusinessError` | 各类业务错误描述 | 参数错误 / 通用业务失败 |
| 1002 | `CodeSensitiveWord` | 敏感词 | 聊天命中敏感词 |
| 4101 | `CodeAccountNotFound` | `账号未注册` | 登录时账号不存在 |
| 4102 | `CodePasswordWrong` | `密码错误` | 密码不匹配 |
| 4103 | `CodeUserBanned` | `账号已封禁` | 用户 `Status != 1` |
| 4104 | `CodePhoneAlreadyReg` | `账号已被注册` / `手机号码已被注册` | 注册时账号/手机号已存在 |
| 4105 | `CodeLoginLocked` | `账号已锁定，请30分钟后再试` | 登录失败 ≥ 10 次 |
| 4106 | `CodeSmsCheckFailed` | `验证码错误` | 短信验证码错误 |
| 4113 | `CodeRateLimited` | `操作过于频繁，请稍后再试` | 限流 |
| 4114 | `CodePhoneInvalid` | `手机号码格式错误` | 手机号格式校验失败 |
| 4120 | `CodeKaptchaInvalid` | `图形验证码错误` | 图形验证码错误或已过期 |
| 4131 | `CodeNickNameBanned` | （昵称违规） | 昵称命中黑名单 |

### 11.2 中间件层错误（HTTP 状态码 200，加密信封内）

| code | 触发场景 |
|------|---------|
| 101 | 加密请求体无法解密 / 不是有效的 protobuf 或 JSON |

中间件返回的错误**总是用 Web 响应密钥加密**（因为此时 plat 还未解析出来）。

### 11.3 HTTP 状态码

| HTTP | 出现场景 |
|------|---------|
| 200 | 几乎所有响应，包括业务错误（错误信息在加密信封的 `code`/`meg` 里） |
| 400 | `/api/kaptcha` 缺 `mobile` 参数 |
| 404 | 路由不存在 / JSONP 路径不匹配 `matches_<YYYYMMDD>.json` 模式 / `:roomNum` 缺失 |
| 500 | RPC 不可用 / 加密失败（极少见） |

> **限流不返回 HTTP 429**。被限流时仍返回 HTTP 200，加密信封内 `code=4113`、
> `meg="操作过于频繁，请稍后再试"`。

---

## 十二、限流策略

### 12.1 全局限流

| 限制 | 配置键 | 默认值 | 算法 |
|------|--------|--------|------|
| 单 IP 全局请求 | `RateLimitPerMinute` | 120 | Redis ZSET 滑窗 |
| 单 IP 登录/注册 | `RateLimitAuthPerMinute` | 20 | Redis ZSET 滑窗 |
| 单 loginName 登录 | （硬编码） | 10 | Redis ZSET 滑窗，key `yuyan:ratelimit:login` |
| 单手机号登录 | （硬编码） | 10 | Redis ZSET 滑窗，key `yuyan:ratelimit:login` |

### 12.2 短信限流

| 限制 | 配置 | 默认值 |
|------|------|--------|
| 单手机号冷却 | Redis `SETNXEX yuyan:ratelimit:cooldown:sms:<cc>:<phone>` | 60 秒 |
| 单手机号小时上限 | Redis `INCR yuyan:ratelimit:sms:<cc>:<phone>:hour` | 10/小时 |

### 12.3 聊天限流

| 限制 | 配置键 | 默认值 |
|------|--------|--------|
| 单客户端发消息 | `ChatRatePerMin` | 60 条/分钟 |
| 单条消息长度 | `ChatMaxMsgLen` | 500 字符 |
| 房间历史保留 | `ChatHistoryLim` | 50 条 |

### 12.4 限流触发行为

- **HTTP 接口**：返回 HTTP 200 + 加密信封 `code=4113, meg="操作过于频繁，请稍后再试"`；
- **WebSocket 接口**：发送 `9999` 错误帧，body `{"code":1004,"msg":"rate limit"}`，**不断开连接**。

---

## 十三、Caddy 网关访问

沙箱环境里 apipro 跑在 Caddy 网关后。Caddy 默认监听 80/443，但 apipro-api 监听
**3100**，apipro-rpc 监听 **3101**。要从 preview host 访问 apipro，需在 URL 上
附加 `?XTransformPort=3100` query 参数，Caddy 会据此把请求转发到 3100 端口。

### 13.1 健康检查（明文）

```bash
curl -sS "https://<preview-host>/health?XTransformPort=3100"
# {"status":"ok","ts":1723000000}
```

### 13.2 加密接口（POST）

```bash
# 业务 JSON body 需先用 AES-128-ECB/PKCS7 + 6 字节帧头封装为二进制
curl -sS \
  -X POST \
  -H "Content-Type: application/octet-stream" \
  --data-binary @encrypted.bin \
  "https://<preview-host>/login/reg?XTransformPort=3100" \
  -o response.bin
```

### 13.3 JSONP 接口

```bash
curl -sS "https://<preview-host>/matches.json?callback=&XTransformPort=3100" | jq .
```

### 13.4 WebSocket

```
wss://<preview-host>/ws/chat?XTransformPort=3100
```

### 13.5 图形验证码

```
https://<preview-host>/api/kaptcha?t=<ts>&mobile=<loginName>&XTransformPort=3100
```

---

## 十四、附录：接口速查表

### 14.1 全部路由

| # | Method | Path | 加密 | RPC method | 默认回调 | 说明 |
|---|--------|------|------|-----------|---------|------|
| 1 | GET | `/health` | ❌ | — | — | 健康检查 |
| 2 | GET | `/api/kaptcha` | ❌ | — | — | 图形验证码 SVG |
| 3 | GET | `/chat.html` | ❌ | — | — | 静态聊天页 |
| 4 | GET | `/ws/chat` | WS | — | — | WebSocket 聊天 |
| 5 | GET | `/matches.json` | ❌ | `matches_jsonp` | `matches` | 全部赛事分组 |
| 6 | GET | `/match_all.json` | ❌ | `matches_jsonp` | `matches` | 同 #5（别名） |
| 7 | GET | `/all_live_rooms.json` | ❌ | `live_all_rooms` | `all_live_rooms` | 全部直播间分组 |
| 8 | GET | `/live_types.json` | ❌ | `live_types` | `live_types` | 直播分类目录 |
| 9 | GET | `/hot_anchor.json` | ❌ | `live_hot` | `hot_anchor` | 热门主播（shape-converted） |
| 10 | GET | `/match_recommend.json` | ❌ | `match_recommend` | `match_recommend` | 推荐赛事 |
| 11 | GET | `/room/:roomNum/detail.json` | ❌ | `room_detail` | `detail` | 房间详情 |
| 12 | GET | `/room/:roomNum/schedule.json` | ❌ | `room_schedule` | `schedule_<roomNum>` | 房间赛程 |
| 13 | GET | `/room/:roomNum/gift_rank.json` | ❌ | `room_gift_rank` | `gift_rank` | 礼物榜 |
| 14 | GET | `/match/:name` | ❌ | `match_byDate` | `matches_<YYYYMMDD>` | 按日期查比赛（仅匹配 `matches_*.json`） |
| 15 | POST | `/login/login` | ✅ | `login` | — | 登录 |
| 16 | POST | `/login/reg` | ✅ | `register` | — | 注册 |
| 17 | POST | `/login/guestLogin` | ✅ | `guestLogin` | — | 游客登录 |
| 18 | POST | `/login/refresh` | ✅ | `refresh` | — | 刷新 token |
| 19 | POST | `/login/logout` | ✅ | `logout` | — | 登出 |
| 20 | POST | `/live/hot` | ✅ | `live_hot` | — | 热门直播间 |
| 21 | POST | `/live/cateList` | ✅ | `live_cateList` | — | 按分类查直播间 |
| 22 | POST | `/live/detail` | ✅ | `live_detail` | — | 直播间详情 |
| 23 | POST | `/match/recommend` | ✅ | `match_recommend` | — | 推荐赛事 |
| 24 | POST | `/match/cateList` | ✅ | `match_cateList` | — | 推荐赛事（别名） |
| 25 | POST | `/match/detail` | ✅ | `match_detail` | — | 比赛详情 |
| 26 | POST | `/user/detail` | ✅ | `user_detail` | — | 当前用户详情 |
| 27 | POST | `/sys/getSmsCode` | ✅ | `sms_getCode` | — | 发送短信验证码 |

> RPC 内部还存在 `sms_checkCode` 方法但未挂路由（仅供其他 RPC 内部调用）。

### 14.2 加密接口请求/响应速查

| 接口 | 请求 param 关键字段 | 响应 result 形状 |
|------|---------------------|------------------|
| `/login/reg` | `accountType, loginName/phone, password, pwdType, nickName, kaptcha` | `AuthResponse`（accessToken+sessionId+refreshToken+userInfo+...） |
| `/login/login` | `accountType, loginName/phone, password, pwdType, loginMode=1, loginType=1` | 同上 |
| `/login/guestLogin` | `{}` | `AuthResponse`（无 refreshToken，24h TTL） |
| `/login/refresh` | `{refreshToken}` | `AuthResponse`（轮换） |
| `/login/logout` | `{}` | `{}` |
| `/user/detail` | `{}` | `{user: UserInfoResult}` |
| `/sys/getSmsCode` | `{countryCode, phone, type}` | `{}` |
| `/match/recommend` | `{}` | `{count, pageNum, matches: [MatchCatalogItem]}` |
| `/match/cateList` | `{}` | 同上 |
| `/match/detail` | `{scheduleId, roomNum?}` | `{match: MatchItem, rooms: [RoomResult]}` |
| `/live/hot` | `{}` | `{hot: [RoomResult]}` |
| `/live/cateList` | `{liveTypeId}` | `{rooms: [RoomResult]}` |
| `/live/detail` | `{roomNum, anchorId?}` | `{room: RoomDetailResult, stream: PlayStreamURLs}` |

### 14.3 JSONP 接口响应速查

| 接口 | 默认回调 | `data` 字段形状 |
|------|---------|----------------|
| `/matches.json` | `matches` | `{0:[], 1:[], 2:[], 5:[], hot:[]}` |
| `/match_all.json` | `matches` | 同上 |
| `/all_live_rooms.json` | `all_live_rooms` | `{0:[], hot:[], <liveTypeId>:[] ...}` |
| `/live_types.json` | `live_types` | `[{liveTypeId, typeName, parentId, icon}]` |
| `/hot_anchor.json` | `hot_anchor` | `{anchors:[{nickName, icon, anchor:{roomNum}}]}` |
| `/match_recommend.json` | `match_recommend` | `{count, pageNum, matches:[MatchCatalogItem]}` |
| `/room/:roomNum/detail.json` | `detail` | `{room: RoomDetailResult, stream: PlayStreamURLs}` |
| `/room/:roomNum/schedule.json` | `schedule_<roomNum>` | `{matches: [MatchItem]}` |
| `/room/:roomNum/gift_rank.json` | `gift_rank` | `[{user: UserInfoResult, contribution: int}]`（裸数组） |
| `/match/matches_<YYYYMMDD>.json` | `matches_<YYYYMMDD>` | `[MatchCatalogItem]`（裸数组，**不包 data**） |

### 14.4 配置项速查（`apipro.yaml`）

| 配置键 | 默认值 | 说明 |
|--------|--------|------|
| `Host` | `0.0.0.0` | API 监听地址 |
| `Port` | `3100` | API 监听端口 |
| `AppMode` | `dev` | `dev`=SMS 旁路生效；`prod`=严格 |
| `ApiKeyReq` | `PHp1st5vEg5Ca8FH` | 客户端→服务端 AES 密钥 |
| `ApiKeyResp` | `qlCJekfRKwWkQxl7` | 服务端→客户端 AES 密钥（Web plat=3） |
| `ApiKeyRespWap` | `PHp1st5vEg5Ca8FH` | 服务端→客户端 AES 密钥（WAP plat=4） |
| `SmsDevBypassCode` | `8888` | Dev 模式 SMS 通用码 |
| `KaptchaCodeLen` | `5` | 图形验证码长度 |
| `KaptchaTTL` | `300` | 图形验证码 TTL（秒） |
| `JsonpSnapshotDir` | `./data/jsonp` | JSONP 快照落盘目录 |
| `FileBaseURL` | `https://sta.ncctrials.com/file` | 文件资产 URL 前缀 |
| `RateLimitPerMinute` | `120` | 全局单 IP 限流 |
| `RateLimitAuthPerMinute` | `20` | 单 IP 鉴权类限流 |
| `ChatMaxMsgLen` | `500` | 聊天单条消息上限 |
| `ChatHistoryLim` | `50` | 聊天历史保留条数 |
| `ChatRatePerMin` | `60` | 单客户端聊天限流 |
| `CorsOrigin` | `*` | CORS 允许源 |

### 14.5 相关文件索引

| 文件 | 作用 |
|------|------|
| `cmd/api/internal/handler/routes.go` | 所有路由注册 |
| `cmd/api/internal/handler/encrypted.go` | 加密接口 handler + kaptcha |
| `cmd/api/internal/handler/jsonp.go` | JSONP handler + health + 静态文件 |
| `pkg/codec/middleware.go` | AES 传输中间件（解密/重加密/信封映射） |
| `pkg/codec/codec.go` | AES-128-ECB/PKCS7 + 6 字节帧编解码 |
| `desc/proto/fy.proto` | Protobuf `FY_CLIENT` 信封 schema |
| `common/auth/pwd.go` | 密码算法（md5(client_md5 + salt)） |
| `common/auth/session.go` | 不透明 token 会话存储 |
| `cmd/rpc/internal/logic/call.go` | RPC `Call` 方法分发 + 全部业务逻辑 + 错误码常量 |
| `cmd/rpc/internal/svc/builders.go` | 响应结构体（AuthResponse、UserInfoResult、RoomResult、MatchCatalogItem 等） |
| `pkg/wschat/hub.go` | WebSocket 聊天 hub（opcode、帧、限流、历史） |
| `cmd/smoketest/main.go` | 端到端冒烟测试（注册/登录/WAP/legacy JSON） |
| `cmd/api/etc/apipro.yaml` | API 进程配置 |
| `cmd/rpc/etc/apipro.yaml` | RPC 进程配置 |

---

**文档版本**：v2.0（完全重写，与代码 100% 对齐）
**最后更新**：基于 `cmd/api/internal/handler/routes.go`、`pkg/codec/middleware.go`、
`cmd/rpc/internal/logic/call.go`、`cmd/rpc/internal/svc/builders.go` 等源码核对生成。
