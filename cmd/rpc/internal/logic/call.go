package logic

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "strconv"
        "strings"
        "time"

        "apipro/cmd/rpc/internal/svc"
        "apipro/common/auth"
        "apipro/common/model"
        "apipro/desc/proto/gen/apipro"

        "github.com/zeromicro/go-zero/core/logx"
)

// Auth/business error codes — match backend-zero's auth_codes.go.
const (
        CodeOK                = 200
        CodeLoginRequired     = 100
        CodeGuestReauth       = 101
        CodeBusinessError     = 400
        CodeAccountNotFound   = 4101
        CodePasswordWrong     = 4102
        CodeUserBanned        = 4103
        CodePhoneAlreadyReg   = 4104
        CodeLoginLocked       = 4105
        CodeSmsCheckFailed    = 4106
        CodeRateLimited       = 4113
        CodePhoneInvalid      = 4114
        CodeNickNameBanned    = 4131
        CodeKaptchaInvalid    = 4120
        CodeSensitiveWord     = 1002
)

type CallLogic struct {
        ctx    context.Context
        svcCtx *svc.ServiceContext
}

func NewCallLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CallLogic {
        return &CallLogic{ctx: ctx, svcCtx: svcCtx}
}

// Call is the single RPC method. It dispatches by `in.Method`.
func (l *CallLogic) Call(in *apipro.CallReq) (*apipro.CallResp, error) {
        resp := &apipro.CallResp{Seq: in.Seq, NewSessionId: in.SessionId}
        method := strings.TrimSpace(in.Method)
        logx.Infof("rpc.Call method=%s sessionLen=%d seq=%d", method, len(in.SessionId), in.Seq)

        var result json.RawMessage
        var code int = CodeOK
        var meg string

        switch method {
        // ---- auth ----
        case "login":
                result, code, meg = l.handleLogin(in)
        case "register":
                result, code, meg = l.handleRegister(in)
        case "guestLogin":
                result, code, meg = l.handleGuestLogin(in)
        case "refresh":
                result, code, meg = l.handleRefresh(in)
        case "logout":
                result, code, meg = l.handleLogout(in)
        case "user_detail":
                result, code, meg = l.handleUserDetail(in)
        case "sms_getCode":
                result, code, meg = l.handleSmsGetCode(in)
        case "sms_checkCode":
                result, code, meg = l.handleSmsCheckCode(in)

        // ---- match ----
        case "matches_jsonp": // matches.json payload
                result, code, meg = l.handleMatchesJSONP()
        case "match_recommend": // encrypted /match/recommend
                result, code, meg = l.handleMatchRecommend()
        case "match_cateList": // alias of recommend
                result, code, meg = l.handleMatchRecommend()
        case "match_detail":
                result, code, meg = l.handleMatchDetail(in)
        case "match_byDate":
                result, code, meg = l.handleMatchByDate(in)

        // ---- live ----
        case "live_all_rooms": // all_live_rooms.json payload
                result, code, meg = l.handleAllLiveRooms()
        case "live_types": // live_types.json payload
                result, code, meg = l.handleLiveTypes()
        case "live_hot": // encrypted /live/hot
                result, code, meg = l.handleLiveHot()
        case "live_detail":
                result, code, meg = l.handleLiveDetail(in)

        // ---- room ----
        case "room_detail": // /room/<n>/detail.json payload
                result, code, meg = l.handleRoomDetail(in)
        case "room_schedule": // /room/<n>/schedule.json payload
                result, code, meg = l.handleRoomSchedule(in)
        case "room_gift_rank": // /room/<n>/gift_rank.json payload
                result, code, meg = l.handleRoomGiftRank(in)

        default:
                code = CodeBusinessError
                meg = "unknown method: " + method
        }

        resp.Code = int32(code)
        resp.Meg = meg
        resp.Result = result
        if code == CodeOK && in.SessionId != "" {
                resp.NewSessionId = in.SessionId
        }
        return resp, nil
}

// =============================================================
// Auth handlers
// =============================================================

type loginReq struct {
        AccountType int    `json:"accountType"`
        CountryCode string `json:"countryCode"`
        Phone       string `json:"phone"`
        LoginMode   int    `json:"loginMode"`
        LoginType   int    `json:"loginType"`
        Password    string `json:"password"` // client md5 hex (case-sensitive)
        PwdType     int    `json:"pwdType"`
        Plat        int    `json:"plat"`
}

type registerReq struct {
        AccountType int    `json:"accountType"`
        CountryCode string `json:"countryCode"`
        Phone       string `json:"phone"`
        SmsType     int    `json:"smsType"`
        SmsCode     string `json:"smsCode"`
        Password    string `json:"password"` // client md5
        NickName    string `json:"nickName"`
        Icon        string `json:"icon"`
        PwdType     int    `json:"pwdType"`
        Kaptcha     string `json:"kaptcha"`
        Plat        int    `json:"plat"`
}

type refreshReq struct {
        RefreshToken string `json:"refreshToken"`
}

type smsReq struct {
        CountryCode string `json:"countryCode"`
        Phone       string `json:"phone"`
        Type        int    `json:"type"` // 1=register, 2=forget, 3=update, 4=bind
}

type smsCheckReq struct {
        CountryCode string `json:"countryCode"`
        Phone       string `json:"phone"`
        Type        int    `json:"type"`
        SmsCode     string `json:"smsCode"`
}

type matchDetailReq struct {
        ScheduleID int64  `json:"scheduleId"`
        RoomNum    string `json:"roomNum"`
}

type liveDetailReq struct {
        RoomNum  string `json:"roomNum"`
        AnchorID int64  `json:"anchorId"`
}

type matchByDateReq struct {
        Date string `json:"date"` // YYYYMMDD
}

func (l *CallLogic) handleLogin(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req loginReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        // Validation gate (matches backend-zero auth.go:213)
        if req.AccountType == 0 || req.LoginMode != 1 || req.LoginType != 1 || req.Phone == "" {
                return nil, CodeBusinessError, "登录失败"
        }
        if req.PwdType != auth.PwdTypeMD5 {
                return nil, CodeBusinessError, "pwd_type 1 is unsupported; use pwd_type=2"
        }
        // Normalize country code (strip +)
        cc := normalizeCC(req.CountryCode)
        u, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone)
        if err != nil {
                if errors.Is(err, model.ErrNotFound) {
                        return nil, CodeAccountNotFound, "账号未注册"
                }
                return nil, CodeBusinessError, err.Error()
        }
        // Verify password
        if u.PwdType != auth.PwdTypeMD5 {
                return nil, CodePasswordWrong, "密码错误"
        }
        if !auth.VerifyPassword(req.Password, u.Password, u.Salt) {
                return nil, CodePasswordWrong, "密码错误"
        }
        // Check status
        if u.Status != 1 {
                return nil, CodeUserBanned, "账号已封禁"
        }
        // Issue session
        sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), req.Plat)
        if err != nil {
                return nil, CodeBusinessError, "issue session: " + err.Error()
        }
        resp := l.buildAuthResponse(sess, u)
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleRegister(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req registerReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        if req.AccountType != 1 || req.Phone == "" || req.NickName == "" || req.PwdType != auth.PwdTypeMD5 {
                return nil, CodeBusinessError, "注册失败"
        }
        cc := normalizeCC(req.CountryCode)
        // Check SMS code (type=1 for register)
        if !l.svcCtx.SmsStore.Verify(l.ctx, cc, req.Phone, 1, req.SmsCode) {
                return nil, CodeSmsCheckFailed, "验证码错误"
        }
        // Check phone not already registered
        if existing, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone); err == nil && existing != nil {
                return nil, CodePhoneAlreadyReg, "手机号码已被注册"
        }
        // Allocate UID
        uid, err := l.svcCtx.Models.Users.NextUID(l.ctx)
        if err != nil {
                return nil, CodeBusinessError, "uid alloc: " + err.Error()
        }
        // Hash password: salt = base64(32 random), stored = md5(client_md5 + salt)
        stored, salt, err := auth.HashPassword(req.Password)
        if err != nil {
                return nil, CodeBusinessError, "hash: " + err.Error()
        }
        u := &model.User{
                UID:         uid,
                LoginName:   req.Phone, // loginName = phone by convention
                NickName:    req.NickName,
                Phone:       req.Phone,
                CountryCode: cc,
                Password:    stored,
                Salt:        salt,
                PwdType:     auth.PwdTypeMD5,
                UserType:    1, // audience
                Status:      1,
                Icon:        req.Icon,
                Gender:      0,
                Plat:        int32(req.Plat),
        }
        if err := l.svcCtx.Models.Users.Insert(l.ctx, u); err != nil {
                if errors.Is(err, model.ErrDuplicate) {
                        return nil, CodePhoneAlreadyReg, "手机号码已被注册"
                }
                return nil, CodeBusinessError, "insert: " + err.Error()
        }
        sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), req.Plat)
        if err != nil {
                return nil, CodeBusinessError, "issue session: " + err.Error()
        }
        resp := l.buildAuthResponse(sess, u)
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleGuestLogin(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                Plat int `json:"plat"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.Plat == 0 {
                req.Plat = 4 // default H5
        }
        sess, err := l.svcCtx.Sessions.IssueGuest(l.ctx, req.Plat)
        if err != nil {
                return nil, CodeBusinessError, "issue guest: " + err.Error()
        }
        resp := svc.AuthResponse{
                AccessToken: sess.AccessToken,
                SessionID:   sess.AccessToken,
                UserInfo: svc.UserInfoResult{
                        UID:      0,
                        NickName: sess.NickName,
                        Gender:   3, // "other" per backend-zero
                },
        }
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleRefresh(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req refreshReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        statusFn := func(uid int64) (int, error) {
                u, err := l.svcCtx.Models.Users.FindByUid(l.ctx, uid)
                if err != nil {
                        return 0, err
                }
                return int(u.Status), nil
        }
        sess, err := l.svcCtx.Sessions.Refresh(l.ctx, req.RefreshToken, statusFn)
        if err != nil {
                if errors.Is(err, auth.ErrUserBanned) {
                        return nil, CodeUserBanned, "账号已封禁"
                }
                return nil, CodeBusinessError, "refresh denied"
        }
        u, err := l.svcCtx.Models.Users.FindByUid(l.ctx, sess.UID)
        if err != nil {
                return nil, CodeBusinessError, "user not found"
        }
        resp := l.buildAuthResponse(sess, u)
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleLogout(in *apipro.CallReq) (json.RawMessage, int, string) {
        if in.SessionId == "" {
                return jsonBytes(map[string]any{}), CodeOK, ""
        }
        _ = l.svcCtx.Sessions.Revoke(l.ctx, in.SessionId)
        return jsonBytes(map[string]any{}), CodeOK, ""
}

func (l *CallLogic) handleUserDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
        if in.SessionId == "" {
                return nil, CodeLoginRequired, "login required"
        }
        sess, err := l.svcCtx.Sessions.Get(l.ctx, in.SessionId)
        if err != nil || sess == nil || sess.IsGuest {
                return nil, CodeLoginRequired, "login required"
        }
        u, err := l.svcCtx.Models.Users.FindByUid(l.ctx, sess.UID)
        if err != nil {
                return nil, CodeBusinessError, "user not found"
        }
        if u.Status != 1 {
                return nil, CodeUserBanned, "账号已封禁"
        }
        resp := map[string]any{
                "user": svc.BuildUserInfo(l.ctx, l.svcCtx.Models, u),
        }
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleSmsGetCode(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req smsReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        if req.Phone == "" {
                return nil, CodePhoneInvalid, "手机号码格式错误"
        }
        cc := normalizeCC(req.CountryCode)
        if req.Type == 0 {
                req.Type = 1
        }
        _, err := l.svcCtx.SmsStore.Issue(l.ctx, cc, req.Phone, req.Type)
        if err != nil {
                return nil, CodeBusinessError, "sms issue: " + err.Error()
        }
        // In dev mode, the dev bypass code is accepted without an actual SMS send.
        // Production should integrate a real SMS gateway here.
        return jsonBytes(map[string]any{}), CodeOK, ""
}

func (l *CallLogic) handleSmsCheckCode(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req smsCheckReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        cc := normalizeCC(req.CountryCode)
        if !l.svcCtx.SmsStore.Verify(l.ctx, cc, req.Phone, req.Type, req.SmsCode) {
                return nil, CodeSmsCheckFailed, "验证码错误"
        }
        return jsonBytes(map[string]any{}), CodeOK, ""
}

// =============================================================
// Match handlers
// =============================================================

func (l *CallLogic) handleMatchesJSONP() (json.RawMessage, int, string) {
        catalog := l.loadMatchCatalog()
        return jsonBytes(catalog), CodeOK, ""
}

func (l *CallLogic) handleMatchRecommend() (json.RawMessage, int, string) {
        items := l.loadRecommend()
        resp := map[string]any{
                "count":   len(items),
                "pageNum": 1,
                "matches": items,
        }
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleMatchDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req matchDetailReq
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.ScheduleID == 0 {
                return nil, CodeBusinessError, "missing scheduleId"
        }
        // Fetch the catalog row + anchors
        rows, err := l.svcCtx.Models.Matches.ListCatalog(l.ctx, []int64{1, 2, 5}, 200)
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        var found *svc.MatchCatalogItem
        items := groupCatalogRowsPublic(rows)
        for i := range items {
                if items[i].ScheduleID == req.ScheduleID {
                        found = &items[i]
                        break
                }
        }
        if found == nil {
                return nil, CodeBusinessError, "match not found"
        }
        // Fetch linked rooms
        rooms, _ := l.svcCtx.Models.Rooms.ListAllVisible(l.ctx, 200)
        roomResults := roomsToResultsPublic(rooms)
        resp := map[string]any{
                "match": *found,
                "rooms": roomResults,
        }
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleMatchByDate(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req matchByDateReq
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.Date == "" {
                req.Date = todayKey()
        }
        // Validate YYYYMMDD
        if len(req.Date) != 8 {
                return nil, CodeBusinessError, "date must be YYYYMMDD"
        }
        items := l.loadMatchByDate(req.Date)
        return jsonBytes(items), CodeOK, ""
}

// =============================================================
// Live handlers
// =============================================================

func (l *CallLogic) handleAllLiveRooms() (json.RawMessage, int, string) {
        out := l.loadAllLiveRooms()
        return jsonBytes(out), CodeOK, ""
}

func (l *CallLogic) handleLiveTypes() (json.RawMessage, int, string) {
        lts, err := l.svcCtx.Models.LiveTypes.ListTopLevel(l.ctx)
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        out := svc.BuildLiveTypesJSON(lts)
        return jsonBytes(out), CodeOK, ""
}

func (l *CallLogic) handleLiveHot() (json.RawMessage, int, string) {
        rooms, err := l.svcCtx.Models.Rooms.ListHot(l.ctx, 50)
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        resp := map[string]any{
                "hot": svc.RoomsToResults(rooms),
        }
        return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleLiveDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req liveDetailReq
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.RoomNum == "" {
                return nil, CodeBusinessError, "missing roomNum"
        }
        detail := svc.BuildRoomDetail(l.ctx, l.svcCtx.Models, req.RoomNum)
        if detail == nil {
                return nil, CodeBusinessError, "room not found"
        }
        return jsonBytes(detail), CodeOK, ""
}

// =============================================================
// Room handlers
// =============================================================

func (l *CallLogic) handleRoomDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                RoomNum string `json:"roomNum"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.RoomNum == "" {
                return nil, CodeBusinessError, "missing roomNum"
        }
        detail := svc.BuildRoomDetail(l.ctx, l.svcCtx.Models, req.RoomNum)
        if detail == nil {
                return nil, CodeBusinessError, "room not found"
        }
        return jsonBytes(detail), CodeOK, ""
}

func (l *CallLogic) handleRoomSchedule(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                RoomNum string `json:"roomNum"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.RoomNum == "" {
                return nil, CodeBusinessError, "missing roomNum"
        }
        out := svc.BuildRoomSchedule(l.ctx, l.svcCtx.Models, req.RoomNum)
        return jsonBytes(out), CodeOK, ""
}

func (l *CallLogic) handleRoomGiftRank(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                RoomNum string `json:"roomNum"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.RoomNum == "" {
                return nil, CodeBusinessError, "missing roomNum"
        }
        out := svc.BuildGiftRank(l.ctx, l.svcCtx.Models, req.RoomNum)
        return jsonBytes(out), CodeOK, ""
}

// =============================================================
// Helpers
// =============================================================

func (l *CallLogic) buildAuthResponse(sess *auth.Session, u *model.User) svc.AuthResponse {
        ccInt, _ := strconv.Atoi(strings.TrimPrefix(u.CountryCode, "+"))
        return svc.AuthResponse{
                AccessToken:  sess.AccessToken,
                SessionID:    sess.AccessToken,
                RefreshToken: sess.RefreshToken,
                UserInfo:     svc.BuildUserInfo(l.ctx, l.svcCtx.Models, u),
                URLs:         map[string]any{},
                Phone:        u.Phone,
                CountryCode:  ccInt,
                LoginName:    u.LoginName,
        }
}

func (l *CallLogic) loadMatchCatalog() map[string][]svc.MatchCatalogItem {
        // Try cache first
        if cached, err := l.svcCtx.Cache.Rdb().Get("apipro:match:catalog"); err == nil && cached != "" {
                var out map[string][]svc.MatchCatalogItem
                if json.Unmarshal([]byte(cached), &out) == nil {
                        return out
                }
        }
        out := svc.BuildMatchCatalog(l.ctx, l.svcCtx.Models)
        b, _ := json.Marshal(out)
        _ = l.svcCtx.Cache.Rdb().Setex("apipro:match:catalog", string(b), 60)
        return out
}

func (l *CallLogic) loadRecommend() []svc.MatchCatalogItem {
        if cached, err := l.svcCtx.Cache.Rdb().Get("apipro:match:recommend"); err == nil && cached != "" {
                var out []svc.MatchCatalogItem
                if json.Unmarshal([]byte(cached), &out) == nil {
                        return out
                }
        }
        out := svc.BuildRecommend(l.ctx, l.svcCtx.Models)
        b, _ := json.Marshal(out)
        _ = l.svcCtx.Cache.Rdb().Setex("apipro:match:recommend", string(b), 60)
        return out
}

func (l *CallLogic) loadMatchByDate(date string) []svc.MatchCatalogItem {
        key := "apipro:match:date:" + date
        if cached, err := l.svcCtx.Cache.Rdb().Get(key); err == nil && cached != "" {
                var out []svc.MatchCatalogItem
                if json.Unmarshal([]byte(cached), &out) == nil {
                        return out
                }
        }
        out := svc.BuildMatchByDate(l.ctx, l.svcCtx.Models, date)
        b, _ := json.Marshal(out)
        _ = l.svcCtx.Cache.Rdb().Setex(key, string(b), 60)
        return out
}

func (l *CallLogic) loadAllLiveRooms() map[string]any {
        if cached, err := l.svcCtx.Cache.Rdb().Get("apipro:live:all"); err == nil && cached != "" {
                var out map[string]any
                if json.Unmarshal([]byte(cached), &out) == nil {
                        return out
                }
        }
        out := svc.BuildAllLiveRooms(l.ctx, l.svcCtx.Models)
        b, _ := json.Marshal(out)
        _ = l.svcCtx.Cache.Rdb().Setex("apipro:live:all", string(b), 15)
        return out
}

// Public wrappers (used by logic that imports svc builders directly)
func groupCatalogRowsPublic(rows []model.MatchCatalogRow) []svc.MatchCatalogItem {
        return svc.GroupCatalogRows(rows)
}

func roomsToResultsPublic(rooms []model.LiveRoom) []svc.RoomResult {
        return svc.RoomsToResults(rooms)
}

// normalizeCC strips leading + and whitespace from country code.
func normalizeCC(cc string) string {
        cc = strings.TrimSpace(cc)
        cc = strings.TrimPrefix(cc, "+")
        return cc
}

// jsonBytes marshals v, returning nil on error (never panics).
func jsonBytes(v any) json.RawMessage {
        b, err := json.Marshal(v)
        if err != nil {
                return nil
        }
        return b
}

// todayKey returns YYYYMMDD for time.Now().
func todayKey() string { return time.Now().Format("20060102") }

// Suppress unused import warnings for net/http (used by handlers when needed).
var _ = http.MethodPost
var _ = fmt.Sprintf
