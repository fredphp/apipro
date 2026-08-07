package wschat

// WebSocket chat hub. Authenticates via ?token=<jwt>, joins a room via ?roomNum=<n>,
// broadcasts messages to all room members, keeps recent history in Redis (cached),
// enforces per-user send rate, max message length, and XSS-escapes content.

import (
        "context"
        "encoding/json"
        "net/http"
        "sync"
        "time"

        "apipro/common/jwtx"

        "github.com/gorilla/websocket"
        "github.com/zeromicro/go-zero/core/logx"
        "github.com/zeromicro/go-zero/core/stores/redis"
)

var upgrader = websocket.Upgrader{
        ReadBufferSize:  4096,
        WriteBufferSize: 4096,
        CheckOrigin: func(r *http.Request) bool {
                return true // CORS handled by gateway; origin validated at token level
        },
}

type Client struct {
        conn     *websocket.Conn
        uid      string
        nickName string
        level    int32
        roomNum  string
        send     chan []byte
        hub      *Hub
}

type BroadcastMsg struct {
        MsgId    string `json:"msgId"`
        RoomNum  string `json:"roomNum"`
        SendUid  string `json:"sendUid"`
        NickName string `json:"nickName"`
        Icon     string `json:"icon"`
        Level    int32  `json:"level"`
        MsgType  int32  `json:"msgType"` // 1 danmu, 2 gift, 3 text
        Content  string `json:"content"`
        Ts       int64  `json:"ts"`
}

type Hub struct {
        mu          sync.RWMutex
        rooms       map[string]map[*Client]struct{}
        register    chan *Client
        unregister  chan *Client
        broadcast   chan BroadcastMsg
        rdb         *redis.Redis
        jwtSecret   string
        maxMsgLen   int
        historyLim  int
        ratePerMin  int
        perUserSend map[string]*slidingWindow
        rateMu      sync.Mutex
}

type slidingWindow struct {
        ts    []int64
        limit int
}

func (s *slidingWindow) allow() bool {
        now := time.Now().UnixNano()
        cut := now - int64(time.Minute)
        i := 0
        for i < len(s.ts) && s.ts[i] < cut {
                i++
        }
        s.ts = s.ts[i:]
        if len(s.ts) >= s.limit {
                return false
        }
        s.ts = append(s.ts, now)
        return true
}

func NewHub(rdb *redis.Redis, jwtSecret string, maxMsgLen, historyLim, ratePerMin int) *Hub {
        return &Hub{
                rooms:       map[string]map[*Client]struct{}{},
                register:    make(chan *Client, 64),
                unregister:  make(chan *Client, 64),
                broadcast:   make(chan BroadcastMsg, 256),
                rdb:         rdb,
                jwtSecret:   jwtSecret,
                maxMsgLen:   maxMsgLen,
                historyLim:  historyLim,
                ratePerMin:  ratePerMin,
                perUserSend: map[string]*slidingWindow{},
        }
}

func (h *Hub) Run(ctx context.Context) {
        for {
                select {
                case c := <-h.register:
                        h.mu.Lock()
                        if h.rooms[c.roomNum] == nil {
                                h.rooms[c.roomNum] = map[*Client]struct{}{}
                        }
                        h.rooms[c.roomNum][c] = struct{}{}
                        h.mu.Unlock()
                        logx.Infof("wschat: client joined room=%s uid=%s", c.roomNum, c.uid)
                case c := <-h.unregister:
                        h.mu.Lock()
                        if room, ok := h.rooms[c.roomNum]; ok {
                                if _, ok := room[c]; ok {
                                        delete(room, c)
                                        close(c.send)
                                        if len(room) == 0 {
                                                delete(h.rooms, c.roomNum)
                                        }
                                }
                        }
                        h.mu.Unlock()
                case msg := <-h.broadcast:
                        h.mu.RLock()
                        room := h.rooms[msg.RoomNum]
                        clients := make([]*Client, 0, len(room))
                        for c := range room {
                                clients = append(clients, c)
                        }
                        h.mu.RUnlock()
                        data, _ := json.Marshal(msg)
                        for _, c := range clients {
                                select {
                                case c.send <- data:
                                default:
                                        // slow client: drop
                                        h.unregister <- c
                                }
                        }
                        h.persistHistory(msg)
                case <-ctx.Done():
                        return
                }
        }
}

func (h *Hub) persistHistory(msg BroadcastMsg) {
        if h.rdb == nil {
                return
        }
        key := "apipro:chat:history:" + msg.RoomNum
        b, _ := json.Marshal(msg)
        // LPUSH + LTRIM to keep last N
        _, _ = h.rdb.Lpush(key, string(b))
        _ = h.rdb.Ltrim(key, 0, int64(h.historyLim-1))
        _ = h.rdb.Expire(key, 24*3600)
}

// LoadHistory returns the last N messages for a room.
func (h *Hub) LoadHistory(roomNum string) []BroadcastMsg {
        if h.rdb == nil {
                return nil
        }
        key := "apipro:chat:history:" + roomNum
        vals, err := h.rdb.Lrange(key, 0, h.historyLim)
        if err != nil {
                return nil
        }
        out := make([]BroadcastMsg, 0, len(vals))
        for _, v := range vals {
                var m BroadcastMsg
                if json.Unmarshal([]byte(v), &m) == nil {
                        out = append(out, m)
                }
        }
        return out
}

// RoomViewerCount returns current connected clients for a room.
func (h *Hub) RoomViewerCount(roomNum string) int {
        h.mu.RLock()
        defer h.mu.RUnlock()
        return len(h.rooms[roomNum])
}

// ServeWS handles /ws/chat?token=<jwt>&roomNum=<n>
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
        tokenStr := r.URL.Query().Get("token")
        roomNum := r.URL.Query().Get("roomNum")
        if roomNum == "" {
                roomNum = "1001"
        }
        // validate token
        uid := "guest"
        nickName := "游客"
        level := int32(1)
        isUser := int32(0)
        if tokenStr != "" {
                c, err := jwtx.Verify(h.jwtSecret, tokenStr)
                if err == nil && c.Typ == jwtx.TypAccess {
                        uid = c.Uid
                        nickName = c.NickName
                        isUser = c.IsUser
                        if isUser == 1 {
                                level = 5
                        }
                }
        }
        // sanitize roomNum (alphanumeric only)
        if !isSafeRoom(roomNum) {
                http.Error(w, "invalid room", http.StatusBadRequest)
                return
        }
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
                logx.Errorf("wschat upgrade: %v", err)
                return
        }
        conn.SetReadLimit(int64(h.maxMsgLen) + 256)
        conn.SetReadDeadline(time.Now().Add(70 * time.Second))
        conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(70 * time.Second)); return nil })

        c := &Client{
                conn: conn, uid: uid, nickName: nickName, level: level,
                roomNum: roomNum, send: make(chan []byte, 64), hub: h,
        }
        h.register <- c

        // send history
        go func() {
                hist := h.LoadHistory(roomNum)
                for _, m := range hist {
                        b, _ := json.Marshal(m)
                        select {
                        case c.send <- b:
                        default:
                                return
                        }
                }
                // welcome
                welcome := BroadcastMsg{
                        MsgId: genMsgId(), RoomNum: roomNum, SendUid: "system",
                        NickName: "系统", MsgType: 3, Content: "欢迎 " + nickName + " 进入直播间 " + roomNum, Ts: time.Now().Unix(),
                }
                b, _ := json.Marshal(welcome)
                select {
                case c.send <- b:
                default:
                }
        }()

        go c.writePump()
        go c.readPump()
}

func (c *Client) readPump() {
        defer func() {
                c.hub.unregister <- c
                _ = c.conn.Close()
        }()
        for {
                _, message, err := c.conn.ReadMessage()
                if err != nil {
                        return
                }
                if len(message) > c.hub.maxMsgLen {
                        c.send <- []byte(`{"error":"message too long"}`)
                        continue
                }
                // parse incoming: {"content":"...","msgType":3}
                var in struct {
                        Content string `json:"content"`
                        MsgType int32  `json:"msgType"`
                }
                if json.Unmarshal(message, &in) != nil {
                        continue
                }
                in.Content = sanitize(in.Content)
                if in.Content == "" {
                        continue
                }
                if in.MsgType != 1 && in.MsgType != 3 {
                        in.MsgType = 3
                }
                // rate limit
                c.hub.rateMu.Lock()
                sw, ok := c.hub.perUserSend[c.uid]
                if !ok {
                        sw = &slidingWindow{ts: nil, limit: c.hub.ratePerMin}
                        c.hub.perUserSend[c.uid] = sw
                }
                c.hub.rateMu.Unlock()
                if !sw.allow() {
                        c.send <- []byte(`{"error":"rate limit"}`)
                        continue
                }
                msg := BroadcastMsg{
                        MsgId: genMsgId(), RoomNum: c.roomNum, SendUid: c.uid,
                        NickName: c.nickName, Level: c.level, MsgType: in.MsgType,
                        Content: in.Content, Ts: time.Now().Unix(),
                }
                c.hub.broadcast <- msg
        }
}

func (c *Client) writePump() {
        ticker := time.NewTicker(30 * time.Second)
        defer func() {
                ticker.Stop()
                _ = c.conn.Close()
        }()
        for {
                select {
                case msg, ok := <-c.send:
                        _ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
                        if !ok {
                                _ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
                                return
                        }
                        if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
                                return
                        }
                case <-ticker.C:
                        _ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
                        if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
                                return
                        }
                }
        }
}
