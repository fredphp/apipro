// Package wschat implements the YuYanTV (zbyy) WebSocket chat protocol.
//
// Connection: GET /ws/chat (upgrade to WebSocket).
//
// Auth is post-upgrade via the LOGIN frame (opcode 1001) carrying
// {key:<accessToken>, plat:<int>, version:<int>}.
//
// Frame format (BINARY message):
//
//      [0x00][0xA0][opcode:uint16 BE][len:uint32 BE][AES-128-ECB-PKCS7(JSON body)]
//
// Empty body (length=0) → no encryption, no JSON (used for HEART/HEART_SUCCESS).
//
// The same keys as the HTTP transport are used:
//   - RequestKey (client→server decrypt)
//   - ResponseKey (server→client encrypt)
//
// For backward-compat with the existing public/chat.html which may send JSON
// text frames, we ALSO accept TextMessage frames containing JSON. The probe
// logic: if frame[0]==0x00 (binary magic) → binary protocol; otherwise treat
// as JSON text.
package wschat

import (
        "context"
        "crypto/rand"
        "encoding/binary"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "strings"
        "sync"
        "time"

        "apipro/common/auth"

        "github.com/gorilla/websocket"
        "github.com/zeromicro/go-zero/core/logx"
        "github.com/zeromicro/go-zero/core/stores/redis"
)

// ----- Opcodes (match backend-zero internal/ws/opcode.go) -----

const (
        OpHeart          uint16 = 1000 // client → server
        OpLogin          uint16 = 1001
        OpLogout         uint16 = 1002
        OpRoomEnter      uint16 = 1003
        OpRoomLeave      uint16 = 1004
        OpComment        uint16 = 1005
        OpCommentDelete  uint16 = 1006
        OpLike           uint16 = 1007 // legacy alias
        OpGift           uint16 = 1010 // legacy alias (was OpSessionResume in backend-zero)
        OpSessionResume  uint16 = 1010

        OpHeartSuccess         uint16 = 2000 // server → client ack
        OpLoginSuccess         uint16 = 2001
        OpLogoutSuccess        uint16 = 2002
        OpRoomEnterSuccess     uint16 = 2003
        OpRoomLeaveSuccess     uint16 = 2004
        OpCommentSuccess       uint16 = 2005
        OpCommentDeleteSuccess uint16 = 2006
        OpLikeSuccess          uint16 = 2007
        OpGiftSuccess          uint16 = 2010

        OpCommentPush        uint16 = 3000 // server → client push
        OpCommentPushDelete  uint16 = 3001
        OpSysPush            uint16 = 3002
        OpScoreGrowPush      uint16 = 3003
        OpSubscribeMatchPush uint16 = 3004
        OpViewNumPush        uint16 = 3005
        OpOfflinePush        uint16 = 3006
        OpSubscribeListPush  uint16 = 3007
        OpFeedbackPush       uint16 = 3008
        OpMsgPush            uint16 = 3009

        OpError uint16 = 9999
)

// WS error codes (inside OpError body).
const (
        ErrCodeNotInChatServer = 1000
        ErrCodeSensitiveWord   = 1002
        ErrCodeUnauthorized    = 1003
        ErrCodeMuted           = 1004
        ErrCodeAccountDisabled = 1005
)

const (
        frameByte0     = 0x00
        frameByte1     = 0xA0
        frameHeaderLen = 8 // 2 magic + 2 opcode + 4 length

        defaultHeartbeatTimeout = 30 * time.Second
        defaultWriteTimeout     = 5 * time.Second
        defaultSendQueueSize    = 256
        defaultHistoryLimit     = 50

        // Redis key for chat history (LPUSH newest first, LTRIM 0 49).
        chatHistoryKeyPrefix = "yuyan:live:chat:"
)

var upgrader = websocket.Upgrader{
        ReadBufferSize:  4096,
        WriteBufferSize: 4096,
        CheckOrigin:     func(r *http.Request) bool { return true },
}

// Client is one WS connection.
type Client struct {
        conn   *websocket.Conn
        hub    *Hub
        send   chan []byte
        mu     sync.Mutex

        // session state
        uid       int64
        nickName  string
        icon      string
        isGuest   bool
        authenticated bool

        // rooms the client is currently in (set lookup)
        rooms map[string]struct{}

        // rate limiting
        sendTimes []int64
}

// Hub manages all clients + rooms.
type Hub struct {
        mu          sync.RWMutex
        rooms       map[string]map[*Client]struct{}
        rdb         *redis.Redis
        sessions    *auth.SessionStore
        maxMsgLen   int
        historyLim  int
        ratePerMin  int
        reqKey      []byte // decrypt client→server
        respKey     []byte // encrypt server→client
}

// NewHub constructs a Hub. The session store is created lazily from rdb.
func NewHub(rdb *redis.Redis, respKey, reqKey []byte, maxMsgLen, historyLim, ratePerMin int) *Hub {
        if maxMsgLen <= 0 {
                maxMsgLen = 500
        }
        if historyLim <= 0 {
                historyLim = defaultHistoryLimit
        }
        if ratePerMin <= 0 {
                ratePerMin = 60
        }
        return &Hub{
                rooms:      map[string]map[*Client]struct{}{},
                rdb:        rdb,
                sessions:   auth.NewSessionStore(rdb),
                maxMsgLen:  maxMsgLen,
                historyLim: historyLim,
                ratePerMin: ratePerMin,
                reqKey:     reqKey,
                respKey:    respKey,
        }
}

// Run starts the hub's broadcast loop. Currently a no-op (broadcasts are
// delivered directly to client.send channels).
func (h *Hub) Run(ctx context.Context) {
        <-ctx.Done()
}

// =============================================================
// HTTP upgrade
// =============================================================

// ServeWS handles GET /ws/chat
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                logx.Errorf("wschat upgrade: %v", err)
                return
        }
        conn.SetReadLimit(int64(h.maxMsgLen) + 256)
        _ = conn.SetReadDeadline(time.Now().Add(defaultHeartbeatTimeout * 2))
        conn.SetPongHandler(func(string) error {
                _ = conn.SetReadDeadline(time.Now().Add(defaultHeartbeatTimeout * 2))
                return nil
        })

        c := &Client{
                conn:  conn,
                hub:   h,
                send:  make(chan []byte, defaultSendQueueSize),
                rooms: map[string]struct{}{},
        }
        go c.writePump()
        go c.readPump()
}

// =============================================================
// Read pump (client → server)
// =============================================================

func (c *Client) readPump() {
        defer func() {
                c.hub.unregisterAll(c)
                _ = c.conn.Close()
        }()
        for {
                msgType, message, err := c.conn.ReadMessage()
                if err != nil {
                        return
                }
                // Reset read deadline on any message (heartbeat proxy).
                _ = c.conn.SetReadDeadline(time.Now().Add(defaultHeartbeatTimeout * 2))

                // Probe: binary magic 0x00 0xA0 → binary protocol; else JSON text.
                if msgType == websocket.BinaryMessage && len(message) >= 2 && message[0] == frameByte0 && message[1] == frameByte1 {
                        c.handleBinaryFrame(message)
                        continue
                }
                // Fallback: JSON text frame (backward-compat with the old chat.html).
                c.handleJSONTextFrame(message)
        }
}

// handleBinaryFrame decodes the 8-byte header + AES-encrypted JSON body, then
// dispatches by opcode.
func (c *Client) handleBinaryFrame(frame []byte) {
        if len(frame) < frameHeaderLen {
                c.sendError(ErrCodeUnauthorized, "invalid frame")
                return
        }
        opcode := binary.BigEndian.Uint16(frame[2:4])
        bodyLen := binary.BigEndian.Uint32(frame[4:8])
        if int(bodyLen) != len(frame)-frameHeaderLen {
                c.sendError(ErrCodeUnauthorized, "frame length mismatch")
                return
        }
        var bodyJSON []byte
        if bodyLen > 0 {
                plain, err := decryptFrameBody(frame[frameHeaderLen:], c.hub.reqKey)
                if err != nil {
                        c.sendError(ErrCodeUnauthorized, "decrypt failed")
                        return
                }
                bodyJSON = plain
        }
        c.dispatch(opcode, bodyJSON)
}

// handleJSONTextFrame handles the legacy JSON text protocol used by the
// previous chat.html. It accepts:
//
//      {"op":1001,"key":"<accessToken>","plat":4,"version":1}
//      {"op":1003,"roomNum":"1001","haveHistory":1}
//      {"op":1005,"roomNum":"1001","content":"hi"}
//      {"op":1000}                // heartbeat
//
// If `op` is missing, treats as a chat comment (op 1005) for backward compat.
func (c *Client) handleJSONTextFrame(message []byte) {
        var msg struct {
                Op          uint16 `json:"op"`
                Key         string `json:"key"`
                Plat        int    `json:"plat"`
                Version     int    `json:"version"`
                RoomNum     string `json:"roomNum"`
                Content     string `json:"content"`
                HaveHistory int    `json:"haveHistory"`
                MsgType     int    `json:"msgType"`
        }
        if err := json.Unmarshal(message, &msg); err != nil {
                return
        }
        op := msg.Op
        if op == 0 {
                // Legacy: bare comment.
                op = OpComment
                if msg.Content == "" {
                        return
                }
        }
        // Build body JSON for dispatch.
        bodyMap := map[string]any{}
        if msg.Key != "" {
                bodyMap["key"] = msg.Key
        }
        if msg.Plat != 0 {
                bodyMap["plat"] = msg.Plat
        }
        if msg.Version != 0 {
                bodyMap["version"] = msg.Version
        }
        if msg.RoomNum != "" {
                bodyMap["roomNum"] = msg.RoomNum
        }
        if msg.Content != "" {
                bodyMap["content"] = msg.Content
        }
        if msg.HaveHistory != 0 {
                bodyMap["haveHistory"] = msg.HaveHistory
        }
        if msg.MsgType != 0 {
                bodyMap["msgType"] = msg.MsgType
        }
        body, _ := json.Marshal(bodyMap)
        // Dispatch in binary protocol; send acks as binary frames so the new
        // zbyy client works. The legacy chat.html reads both binary and text.
        c.dispatch(op, body)
}

// dispatch is the central opcode router.
func (c *Client) dispatch(op uint16, body []byte) {
        switch op {
        case OpHeart:
                c.sendBinary(OpHeartSuccess, nil)
        case OpLogin:
                c.handleLogin(body)
        case OpLogout:
                c.handleLogout(body)
        case OpRoomEnter:
                c.handleRoomEnter(body)
        case OpRoomLeave:
                c.handleRoomLeave(body)
        case OpComment:
                c.handleComment(body)
        case OpLike:
                c.handleLike(body)
        case OpGift:
                c.handleGift(body)
        default:
                c.sendError(ErrCodeUnauthorized, fmt.Sprintf("unknown opcode %d", op))
        }
}

// ----- Per-opcode handlers -----

type loginBody struct {
        Key     string `json:"key"`
        Plat    int    `json:"plat"`
        Version int    `json:"version"`
}

func (c *Client) handleLogin(body []byte) {
        var b loginBody
        _ = json.Unmarshal(body, &b)
        if b.Key == "" {
                c.sendError(ErrCodeUnauthorized, "missing key")
                return
        }
        sess, err := c.hub.sessions.Get(context.Background(), b.Key)
        if err != nil || sess == nil {
                c.sendError(ErrCodeUnauthorized, "invalid session")
                return
        }
        c.uid = sess.UID
        c.nickName = sess.NickName
        c.icon = sess.Icon
        c.isGuest = sess.IsGuest
        c.authenticated = true
        c.sendBinary(OpLoginSuccess, []byte(`{}`))
}

func (c *Client) handleLogout(body []byte) {
        c.sendBinary(OpLogoutSuccess, []byte(`{}`))
        c.hub.unregisterAll(c)
        _ = c.conn.Close()
}

type roomEnterBody struct {
        RoomNum      string `json:"roomNum"`
        HaveHistory  int    `json:"haveHistory"`
}

func (c *Client) handleRoomEnter(body []byte) {
        if !c.authenticated {
                c.sendError(ErrCodeUnauthorized, "login required")
                return
        }
        var b roomEnterBody
        _ = json.Unmarshal(body, &b)
        if b.RoomNum == "" {
                c.sendError(ErrCodeNotInChatServer, "missing roomNum")
                return
        }
        if !isSafeRoom(b.RoomNum) {
                c.sendError(ErrCodeNotInChatServer, "invalid room")
                return
        }
        // Join the room.
        c.hub.joinRoom(b.RoomNum, c)
        // Ack
        c.sendBinary(OpRoomEnterSuccess, []byte(`{}`))
        // Broadcast updated viewer count
        c.hub.broadcastViewNum(b.RoomNum)
        // Replay history (last 50 messages)
        c.replayHistory(b.RoomNum)
}

func (c *Client) handleRoomLeave(body []byte) {
        var b struct{ RoomNum string `json:"roomNum"` }
        _ = json.Unmarshal(body, &b)
        if b.RoomNum != "" {
                c.hub.leaveRoom(b.RoomNum, c)
                c.hub.broadcastViewNum(b.RoomNum)
        }
        c.sendBinary(OpRoomLeaveSuccess, []byte(`{}`))
}

type commentBody struct {
        RoomNum  string `json:"roomNum"`
        MsgType  int    `json:"msgType"`
        Content  string `json:"content"`
        SendUID  int64  `json:"sendUid"`
}

func (c *Client) handleComment(body []byte) {
        if !c.authenticated {
                c.sendError(ErrCodeUnauthorized, "login required")
                return
        }
        var b commentBody
        _ = json.Unmarshal(body, &b)
        if b.RoomNum == "" {
                c.sendError(ErrCodeNotInChatServer, "missing roomNum")
                return
        }
        if _, ok := c.rooms[b.RoomNum]; !ok {
                c.sendError(ErrCodeNotInChatServer, "not in room")
                return
        }
        if c.isGuest {
                c.sendError(ErrCodeMuted, "guests cannot speak")
                return
        }
        content := sanitize(b.Content)
        if content == "" {
                return
        }
        if len(content) > c.hub.maxMsgLen {
                content = content[:c.hub.maxMsgLen]
        }
        // Rate limit
        if !c.allowSend() {
                c.sendError(ErrCodeMuted, "rate limit")
                return
        }
        // Sensitive word filter (simple built-in list).
        if hit, word := matchBlockword(content); hit {
                logx.Infof("wschat: blocked word %q in %q", word, content)
                c.sendError(ErrCodeSensitiveWord, "请勿发布敏感内容，多次违规将封号处理")
                return
        }
        // Build push body.
        msgType := b.MsgType
        if msgType == 0 {
                msgType = 1 // text
        }
        msgID := genMsgID()
        pushBody, _ := json.Marshal(map[string]any{
                "sendUid":  c.uid,
                "roomNum":  b.RoomNum,
                "msgType":  msgType,
                "sendTime": time.Now().UTC().UnixMilli(),
                "content":  content,
                "sendUser": map[string]any{
                        "uid":      c.uid,
                        "nickName": c.nickName,
                        "icon":     c.icon,
                },
                "msgId": msgID,
        })
        // Persist to Redis list (LPUSH + LTRIM 0 49).
        c.hub.persistHistory(b.RoomNum, pushBody)
        // Ack the sender.
        c.sendBinary(OpCommentSuccess, []byte(`{}`))
        // Broadcast to the room.
        c.hub.broadcast(b.RoomNum, OpCommentPush, pushBody)
}

func (c *Client) handleLike(body []byte) {
        if !c.authenticated {
                c.sendError(ErrCodeUnauthorized, "login required")
                return
        }
        var b struct {
                RoomNum string `json:"roomNum"`
        }
        _ = json.Unmarshal(body, &b)
        if b.RoomNum == "" {
                return
        }
        c.sendBinary(OpLikeSuccess, []byte(`{}`))
        push, _ := json.Marshal(map[string]any{
                "sendUid": c.uid,
                "roomNum": b.RoomNum,
        })
        c.hub.broadcast(b.RoomNum, OpLikeSuccess, push)
}

func (c *Client) handleGift(body []byte) {
        if !c.authenticated {
                c.sendError(ErrCodeUnauthorized, "login required")
                return
        }
        c.sendBinary(OpGiftSuccess, []byte(`{}`))
}

// =============================================================
// Write pump (server → client)
// =============================================================

func (c *Client) writePump() {
        ticker := time.NewTicker(30 * time.Second)
        defer func() {
                ticker.Stop()
                _ = c.conn.Close()
        }()
        for {
                select {
                case msg, ok := <-c.send:
                        _ = c.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout * 2))
                        if !ok {
                                _ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                                return
                        }
                        if err := c.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
                                return
                        }
                case <-ticker.C:
                        _ = c.conn.SetWriteDeadline(time.Now().Add(defaultWriteTimeout * 2))
                        if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                                return
                        }
                }
        }
}

// =============================================================
// Hub room management
// =============================================================

func (h *Hub) joinRoom(room string, c *Client) {
        h.mu.Lock()
        defer h.mu.Unlock()
        if h.rooms[room] == nil {
                h.rooms[room] = map[*Client]struct{}{}
        }
        h.rooms[room][c] = struct{}{}
        c.rooms[room] = struct{}{}
}

func (h *Hub) leaveRoom(room string, c *Client) {
        h.mu.Lock()
        defer h.mu.Unlock()
        if r, ok := h.rooms[room]; ok {
                delete(r, c)
                if len(r) == 0 {
                        delete(h.rooms, room)
                }
        }
        delete(c.rooms, room)
}

func (h *Hub) unregisterAll(c *Client) {
        h.mu.Lock()
        defer h.mu.Unlock()
        for room := range c.rooms {
                if r, ok := h.rooms[room]; ok {
                        delete(r, c)
                        if len(r) == 0 {
                                delete(h.rooms, room)
                        }
                }
        }
        c.rooms = map[string]struct{}{}
}

// broadcast sends a frame to all clients in a room.
func (h *Hub) broadcast(room string, op uint16, body []byte) {
        h.mu.RLock()
        clients := make([]*Client, 0, len(h.rooms[room]))
        for c := range h.rooms[room] {
                clients = append(clients, c)
        }
        h.mu.RUnlock()
        frame := encodeBinaryFrame(op, body, h.respKey)
        for _, c := range clients {
                select {
                case c.send <- frame:
                default:
                        // slow client: drop
                        go h.unregisterAll(c)
                }
        }
}

// broadcastViewNum pushes 3005 {roomNum, viewNum} to all room members.
func (h *Hub) broadcastViewNum(room string) {
        h.mu.RLock()
        viewNum := int64(len(h.rooms[room]))
        h.mu.RUnlock()
        body, _ := json.Marshal(map[string]any{
                "roomNum": room,
                "viewNum": viewNum,
        })
        h.broadcast(room, OpViewNumPush, body)
}

// persistHistory appends the message JSON to the room's Redis LIST.
func (h *Hub) persistHistory(room string, body []byte) {
        if h.rdb == nil {
                return
        }
        key := chatHistoryKeyPrefix + room
        _, _ = h.rdb.Lpush(key, string(body))
        _ = h.rdb.Ltrim(key, 0, int64(h.historyLim-1))
        _ = h.rdb.Expire(key, 24*3600)
}

// replayHistory sends the last N messages as 3000 frames to the client.
func (c *Client) replayHistory(room string) {
        if c.hub.rdb == nil {
                return
        }
        key := chatHistoryKeyPrefix + room
        vals, err := c.hub.rdb.Lrange(key, 0, c.hub.historyLim)
        if err != nil {
                return
        }
        // Redis LIST is LPUSH newest-first → reverse for chronological replay.
        for i := len(vals) - 1; i >= 0; i-- {
                frame := encodeBinaryFrame(OpCommentPush, []byte(vals[i]), c.hub.respKey)
                select {
                case c.send <- frame:
                default:
                        return
                }
        }
}

// =============================================================
// Client helpers
// =============================================================

// sendBinary writes a frame to the client's send queue.
func (c *Client) sendBinary(op uint16, body []byte) {
        frame := encodeBinaryFrame(op, body, c.hub.respKey)
        select {
        case c.send <- frame:
        default:
        }
}

// sendError sends an OpError (9999) frame with the given code+message.
func (c *Client) sendError(code int, msg string) {
        body, _ := json.Marshal(map[string]any{
                "errCode": code,
                "errMsg":  msg,
        })
        c.sendBinary(OpError, body)
}

// allowSend enforces per-user rate limiting (sliding window).
func (c *Client) allowSend() bool {
        now := time.Now().UnixNano()
        cut := now - int64(time.Minute)
        i := 0
        for i < len(c.sendTimes) && c.sendTimes[i] < cut {
                i++
        }
        c.sendTimes = c.sendTimes[i:]
        if len(c.sendTimes) >= c.hub.ratePerMin {
                return false
        }
        c.sendTimes = append(c.sendTimes, now)
        return true
}

// =============================================================
// Frame encoding
// =============================================================

// encodeBinaryFrame builds a binary frame:
//   [0x00][0xA0][opcode uint16 BE][len uint32 BE][AES-encrypted body]
// Empty body → length=0, no encryption.
func encodeBinaryFrame(op uint16, body, key []byte) []byte {
        var ct []byte
        if len(body) > 0 {
                ct, _ = encryptFrameBody(body, key)
        }
        out := make([]byte, frameHeaderLen+len(ct))
        out[0] = frameByte0
        out[1] = frameByte1
        binary.BigEndian.PutUint16(out[2:4], op)
        binary.BigEndian.PutUint32(out[4:8], uint32(len(ct)))
        copy(out[8:], ct)
        return out
}

// =============================================================
// Misc helpers
// =============================================================

func genMsgID() string {
        b := make([]byte, 12)
        _, _ = rand.Read(b)
        return hex.EncodeToString(b)
}

// isSafeRoom allows only alphanumeric room ids.
func isSafeRoom(r string) bool {
        if r == "" || len(r) > 32 {
                return false
        }
        for _, ch := range r {
                if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z') {
                        return false
                }
        }
        return true
}

// sanitize escapes HTML special chars and trims/limits length.
func sanitize(s string) string {
        s = strings.TrimSpace(s)
        if len(s) > 500 {
                s = s[:500]
        }
        var b strings.Builder
        for _, r := range s {
                switch r {
                case '<':
                        b.WriteString("&lt;")
                case '>':
                        b.WriteString("&gt;")
                case '&':
                        b.WriteString("&amp;")
                case '"':
                        b.WriteString("&#34;")
                case '\'':
                        b.WriteString("&#39;")
                default:
                        b.WriteRune(r)
                }
        }
        return b.String()
}

// matchBlockword returns (true, word) if any blockword is found (substring,
// case-insensitive). The list is a small built-in baseline — production
// backends load from a block_word table.
func matchBlockword(text string) (bool, string) {
        low := strings.ToLower(text)
        for _, w := range blockwords {
                if strings.Contains(low, w) {
                        return true, w
                }
        }
        return false, ""
}

// blockwords is a tiny built-in baseline list. Real deployments should load
// from a `block_word` table and refresh periodically.
var blockwords = []string{
        "fuck", "shit", "bitch", "asshole", "dick",
        "赌博", "赌场", "博彩", "色情", "黄赌毒",
        "反动", "法轮", "六四", "台独", "港独",
        "私服", "外挂", "刷屏",
}

// Suppress unused import warning.
var _ = errors.New
