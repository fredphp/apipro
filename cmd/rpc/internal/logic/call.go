package logic

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "regexp"
        "strconv"
        "strings"
        "time"
        "unicode/utf8"

        "apipro/cmd/rpc/internal/svc"
        "apipro/common/auth"
        "apipro/common/cache"
        "apipro/common/model"
        "apipro/common/ratelimit"
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
        case "live_types": // live_types.json payload (top-level live type catalog)
                result, code, meg = l.handleLiveTypes()
        case "live_cateList": // AUDIT-001: /live/cateList — rooms filtered by liveTypeId
                result, code, meg = l.handleLiveCateList(in)
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
        // BUG-FIX (spec compliance): for auth methods that issue a NEW session,
        // common_result.new_session_id must carry the server-issued access token
        // (per fy.proto COMMON_RESULT.new_session_id "for rotation"), NOT the
        // client's old guest session ID (which is what in.SessionId holds for
        // login/register/guestLogin/refresh).
        //
        // The new access token is already inside the result JSON (AuthResponse.
        // SessionID = sess.AccessToken). Extract it and override NewSessionId
        // so both common_result.new_session_id AND result.sessionId carry the
        // same new token. Non-auth methods keep the echo-session behavior.
        if code == CodeOK && isAuthMethod(method) {
                if newSID := extractResultSessionID(result); newSID != "" {
                        resp.NewSessionId = newSID
                }
        }
        return resp, nil
}

// isAuthMethod reports whether the method issues a new server session.
// login/register/guestLogin/refresh all return a fresh access token in the
// result JSON; logout and user_detail do NOT issue a new session.
func isAuthMethod(method string) bool {
        switch method {
        case "login", "register", "guestLogin", "refresh":
                return true
        }
        return false
}

// extractResultSessionID parses the auth result JSON and returns the new
// session ID. Tries "sessionId" first (per doc step 16), falls back to
// "accessToken". Returns "" if not found or parse fails (e.g. error responses
// where result is nil).
func extractResultSessionID(result json.RawMessage) string {
        if len(result) == 0 {
                return ""
        }
        var m struct {
                SessionID   string `json:"sessionId"`
                AccessToken string `json:"accessToken"`
        }
        if err := json.Unmarshal(result, &m); err != nil {
                return ""
        }
        if m.SessionID != "" {
                return m.SessionID
        }
        return m.AccessToken
}

// =============================================================
// Auth handlers
// =============================================================

type loginReq struct {
        AccountType int    `json:"accountType"`
        CountryCode string `json:"countryCode"`
        Phone       string `json:"phone"`
        LoginName   string `json:"loginName"` // accountType=2 (account login)
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
        LoginName   string `json:"loginName"` // accountType=2 (account register)
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
        // Validation gate (matches backend-zero auth.go:213).
        // accountType=1 → phone login; accountType=2 → account login (by loginName).
        if req.AccountType == 0 || req.LoginMode != 1 || req.LoginType != 1 {
                return nil, CodeBusinessError, "登录失败"
        }
        if req.PwdType != auth.PwdTypeMD5 {
                return nil, CodeBusinessError, "pwd_type 1 is unsupported; use pwd_type=2"
        }
        // Resolve plat: prefer CLIENT_INFO plat (via CallReq.Plat), fall back to
        // the business JSON plat field (legacy clients).
        plat := resolvePlat(in.Plat, req.Plat)

        rdb := l.svcCtx.Redis

        // ---- Branch: account login (accountType=2) ----
        if req.AccountType == 2 {
                loginName := strings.TrimSpace(req.LoginName)
                if loginName == "" {
                        return nil, CodeBusinessError, "登录失败"
                }
                // Rate limit by loginName (10/min).
                loginLimiter := ratelimit.New(rdb, 10, "yuyan:ratelimit:login")
                if !loginLimiter.Allow(l.ctx, "acct:"+loginName) {
                        return nil, CodeRateLimited, "操作过于频繁，请稍后再试"
                }
                lockKey := "yuyan:login:lock:acct:" + loginName
                if locked, _ := rdb.Get(lockKey); locked != "" {
                        return nil, CodeLoginLocked, "账号已锁定，请30分钟后再试"
                }
                u, err := l.svcCtx.Models.Users.FindByLoginName(l.ctx, loginName)
                if err != nil {
                        if errors.Is(err, model.ErrNotFound) {
                                return nil, CodeAccountNotFound, "账号未注册"
                        }
                        return nil, CodeBusinessError, err.Error()
                }
                if u.PwdType != auth.PwdTypeMD5 {
                        l.recordLoginFailAcct(loginName)
                        return nil, CodePasswordWrong, "密码错误"
                }
                if !auth.VerifyPassword(req.Password, u.Password, u.Salt) {
                        l.recordLoginFailAcct(loginName)
                        return nil, CodePasswordWrong, "密码错误"
                }
                if u.Status != 1 {
                        return nil, CodeUserBanned, "账号已封禁"
                }
                _, _ = rdb.Del("yuyan:login:fail:acct:" + loginName)
                sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), plat)
                if err != nil {
                        return nil, CodeBusinessError, "issue session: " + err.Error()
                }
                resp := l.buildAuthResponse(sess, u)
                return jsonBytes(resp), CodeOK, ""
        }

        // ---- Branch: phone login (accountType=1, existing flow) ----
        if req.Phone == "" {
                return nil, CodeBusinessError, "登录失败"
        }
        cc := normalizeCC(req.CountryCode)
        if !validatePhone(cc, req.Phone) {
                return nil, CodePhoneInvalid, "手机号码格式错误"
        }
        loginLimiter := ratelimit.New(rdb, 10, "yuyan:ratelimit:login")
        if !loginLimiter.Allow(l.ctx, cc+":"+req.Phone) {
                return nil, CodeRateLimited, "操作过于频繁，请稍后再试"
        }
        lockKey := "yuyan:login:lock:" + cc + ":" + req.Phone
        if locked, _ := rdb.Get(lockKey); locked != "" {
                return nil, CodeLoginLocked, "账号已锁定，请30分钟后再试"
        }
        u, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone)
        if err != nil {
                if errors.Is(err, model.ErrNotFound) {
                        return nil, CodeAccountNotFound, "账号未注册"
                }
                return nil, CodeBusinessError, err.Error()
        }
        if u.PwdType != auth.PwdTypeMD5 {
                l.recordLoginFail(cc, req.Phone)
                return nil, CodePasswordWrong, "密码错误"
        }
        if !auth.VerifyPassword(req.Password, u.Password, u.Salt) {
                l.recordLoginFail(cc, req.Phone)
                return nil, CodePasswordWrong, "密码错误"
        }
        if u.Status != 1 {
                return nil, CodeUserBanned, "账号已封禁"
        }
        _, _ = rdb.Del("yuyan:login:fail:" + cc + ":" + req.Phone)
        sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), plat)
        if err != nil {
                return nil, CodeBusinessError, "issue session: " + err.Error()
        }
        resp := l.buildAuthResponse(sess, u)
        return jsonBytes(resp), CodeOK, ""
}

// resolvePlat parses the plat from CallReq.Plat (string, set by the codec
// middleware from CLIENT_INFO) and falls back to the business-JSON plat.
//   3 = Web, 4 = WAP, 0 = unknown (defaults to 4/H5).
func resolvePlat(callReqPlat string, jsonPlat int) int {
        if s := strings.TrimSpace(callReqPlat); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n > 0 {
                        return n
                }
        }
        if jsonPlat > 0 {
                return jsonPlat
        }
        return 4 // default H5/WAP
}

// recordLoginFailAcct is the account-based counterpart of recordLoginFail.
func (l *CallLogic) recordLoginFailAcct(loginName string) {
        rdb := l.svcCtx.Redis
        if rdb == nil {
                return
        }
        failKey := "yuyan:login:fail:acct:" + loginName
        lockKey := "yuyan:login:lock:acct:" + loginName
        cnt, err := rdb.Incr(failKey)
        if err != nil {
                return
        }
        if cnt == 1 {
                _ = rdb.Expire(failKey, 15*60)
        }
        if cnt >= 10 {
                _ = rdb.Setex(lockKey, "1", 30*60)
                _, _ = rdb.Del(failKey)
        }
}

// recordLoginFail increments the per-(cc,phone) fail counter. After 10 fails
// within 15min, a 30min lock is set and the counter is cleared.
// AUDIT-003.
func (l *CallLogic) recordLoginFail(cc, phone string) {
        rdb := l.svcCtx.Redis
        if rdb == nil {
                return
        }
        failKey := "yuyan:login:fail:" + cc + ":" + phone
        lockKey := "yuyan:login:lock:" + cc + ":" + phone
        cnt, err := rdb.Incr(failKey)
        if err != nil {
                return
        }
        // Set 15min TTL on first fail (best-effort; Idempotent on subsequent fails).
        if cnt == 1 {
                _ = rdb.Expire(failKey, 15*60)
        }
        if cnt >= 10 {
                _ = rdb.Setex(lockKey, "1", 30*60)
                _, _ = rdb.Del(failKey)
        }
}

func (l *CallLogic) handleRegister(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req registerReq
        if err := json.Unmarshal([]byte(in.ParamJson), &req); err != nil {
                return nil, CodeBusinessError, "invalid request"
        }
        if req.AccountType == 0 || req.NickName == "" || req.PwdType != auth.PwdTypeMD5 {
                return nil, CodeBusinessError, "注册失败"
        }
        // AUDIT-005: nickname length check (>= 2 runes).
        if utf8.RuneCountInString(req.NickName) < 2 {
                return nil, CodeBusinessError, "昵称至少2个字符"
        }
        // Resolve plat from CLIENT_INFO (via CallReq.Plat), fall back to JSON plat.
        plat := resolvePlat(in.Plat, req.Plat)

        // ---- Branch: account register (accountType=2) — NO SMS, only kaptcha ----
        if req.AccountType == 2 {
                loginName := strings.TrimSpace(req.LoginName)
                if loginName == "" {
                        return nil, CodeBusinessError, "注册失败"
                }
                // AUDIT-005: verify kaptcha — keyed by loginName.
                kaptchaKey := "yuyan:kaptcha:" + loginName
                storedKaptcha, _ := l.svcCtx.Cache.Rdb().Get(kaptchaKey)
                _, _ = l.svcCtx.Cache.Rdb().Del(kaptchaKey)
                if !strings.EqualFold(strings.TrimSpace(storedKaptcha), strings.TrimSpace(req.Kaptcha)) {
                        return nil, CodeKaptchaInvalid, "图形验证码错误"
                }
                // Check loginName not already registered.
                if existing, err := l.svcCtx.Models.Users.FindByLoginName(l.ctx, loginName); err == nil && existing != nil {
                        return nil, CodePhoneAlreadyReg, "账号已被注册"
                }
                uid, err := l.svcCtx.AllocUID(l.ctx)
                if err != nil {
                        return nil, CodeBusinessError, "uid alloc: " + err.Error()
                }
                stored, salt, err := auth.HashPassword(req.Password)
                if err != nil {
                        return nil, CodeBusinessError, "hash: " + err.Error()
                }
                u := &model.User{
                        UID:       uid,
                        LoginName: loginName,
                        NickName:  req.NickName,
                        Password:  stored,
                        Salt:      salt,
                        PwdType:   auth.PwdTypeMD5,
                        UserType:  1, // audience
                        Status:    1,
                        Icon:      req.Icon,
                        Gender:    0,
                        Plat:      int32(plat),
                }
                if err := l.svcCtx.Models.Users.Insert(l.ctx, u); err != nil {
                        if errors.Is(err, model.ErrDuplicate) {
                                logx.Errorf("register: uid collision uid=%d (retry recommended)", uid)
                                return nil, CodeBusinessError, "注册失败请重试"
                        }
                        return nil, CodeBusinessError, "insert: " + err.Error()
                }
                sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), plat)
                if err != nil {
                        return nil, CodeBusinessError, "issue session: " + err.Error()
                }
                resp := l.buildAuthResponse(sess, u)
                return jsonBytes(resp), CodeOK, ""
        }

        // ---- Branch: phone register (accountType=1, existing flow) ----
        if req.Phone == "" {
                return nil, CodeBusinessError, "注册失败"
        }
        cc := normalizeCC(req.CountryCode)
        // AUDIT-003: phone format validation.
        if !validatePhone(cc, req.Phone) {
                return nil, CodePhoneInvalid, "手机号码格式错误"
        }
        // Check SMS code (type=1 for register)
        if !l.svcCtx.SmsStore.Verify(l.ctx, cc, req.Phone, 1, req.SmsCode) {
                return nil, CodeSmsCheckFailed, "验证码错误"
        }
        // AUDIT-005: verify kaptcha (image captcha) — one-shot.
        kaptchaKey := "yuyan:kaptcha:" + req.Phone
        storedKaptcha, _ := l.svcCtx.Cache.Rdb().Get(kaptchaKey)
        _, _ = l.svcCtx.Cache.Rdb().Del(kaptchaKey)
        if !strings.EqualFold(strings.TrimSpace(storedKaptcha), strings.TrimSpace(req.Kaptcha)) {
                return nil, CodeKaptchaInvalid, "图形验证码错误"
        }
        // Check phone not already registered
        if existing, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone); err == nil && existing != nil {
                return nil, CodePhoneAlreadyReg, "手机号码已被注册"
        }
        // AUDIT-008: allocate UID atomically via Redis INCR (avoids race).
        uid, err := l.svcCtx.AllocUID(l.ctx)
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
                Plat:        int32(plat),
        }
        if err := l.svcCtx.Models.Users.Insert(l.ctx, u); err != nil {
                // AUDIT-008: PK violation on uid is a race / counter collision, NOT a
                // phone duplication — return a generic error so the user retries.
                if errors.Is(err, model.ErrDuplicate) {
                        logx.Errorf("register: uid collision uid=%d (retry recommended)", uid)
                        return nil, CodeBusinessError, "注册失败请重试"
                }
                return nil, CodeBusinessError, "insert: " + err.Error()
        }
        sess, err := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), plat)
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
        plat := resolvePlat(in.Plat, req.Plat)
        sess, err := l.svcCtx.Sessions.IssueGuest(l.ctx, plat)
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
                        // AUDIT-007: guest tier is "audience" (1) — clients gating on
                        // userType==1 must accept guest sessions.
                        UserType: 1,
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
        // AUDIT-003: phone format validation.
        if !validatePhone(cc, req.Phone) {
                return nil, CodePhoneInvalid, "手机号码格式错误"
        }
        rdb := l.svcCtx.Redis
        if rdb != nil {
                // AUDIT-004: 60s cooldown per (cc,phone) — SETNXEX returns false if key exists.
                cooldownKey := "yuyan:ratelimit:cooldown:sms:" + cc + ":" + req.Phone
                ok, _ := rdb.SetnxEx(cooldownKey, "1", 60)
                if !ok {
                        return nil, CodeRateLimited, "验证码发送过频，请60秒后再试"
                }
                // AUDIT-004: hourly limit — INCR with 1h TTL, max 10/hour.
                hourKey := "yuyan:ratelimit:sms:" + cc + ":" + req.Phone + ":hour"
                cnt, err := rdb.Incr(hourKey)
                if err == nil && cnt == 1 {
                        _ = rdb.Expire(hourKey, 3600)
                }
                if cnt > 10 {
                        return nil, CodeRateLimited, "验证码发送次数已达上限"
                }
        }
        // AUDIT-004: register guard — if type=1 and phone already registered, refuse.
        if req.Type == 1 {
                if existing, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone); err == nil && existing != nil {
                        return nil, CodePhoneAlreadyReg, "手机号码已被注册"
                }
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
        // AUDIT-014: cache the (match, rooms) payload for 60s.
        cacheKey := "detail:" + strconv.FormatInt(req.ScheduleID, 10)
        out, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", cacheKey, 60*time.Second, func() (map[string]any, error) {
                return l.buildMatchDetailPayload(req.ScheduleID)
        })
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        if out == nil {
                return nil, CodeBusinessError, "match not found"
        }
        return jsonBytes(out), CodeOK, ""
}

// buildMatchDetailPayload fetches the catalog row + linked rooms for a
// schedule and returns the {match, rooms} payload. Returns nil map (with nil
// error) if the schedule is not found.
func (l *CallLogic) buildMatchDetailPayload(scheduleID int64) (map[string]any, error) {
        // Fetch the catalog row + anchors
        rows, err := l.svcCtx.Models.Matches.ListCatalog(l.ctx, []int64{1, 2, 5}, 200)
        if err != nil {
                return nil, err
        }
        var found *svc.MatchCatalogItem
        items := groupCatalogRowsPublic(rows)
        for i := range items {
                if items[i].ScheduleID == scheduleID {
                        found = &items[i]
                        break
                }
        }
        if found == nil {
                return nil, nil
        }
        // AUDIT-006: fetch only the rooms LINKED to this schedule (not all rooms).
        rooms, _ := l.svcCtx.Models.Matches.ListRoomsBySchedule(l.ctx, scheduleID, 50)
        roomResults := roomsToResultsPublic(rooms)
        // AUDIT-023: the "match" field uses the simpler MatchItem struct (8
        // fields) rather than the MatchCatalogItem superset — matches backend-zero.
        matchItem := svc.MatchItem{
                ScheduleID:   found.ScheduleID,
                MatchTime:    found.MatchTime,
                HostName:     found.HostName,
                GuestName:    found.GuestName,
                HostIcon:     found.HostIcon,
                GuestIcon:    found.GuestIcon,
                SubCateName:  found.SubCateName,
                CategoryIcon: found.CategoryIcon,
        }
        return map[string]any{
                "match": matchItem,
                "rooms": roomResults,
        }, nil
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
        // AUDIT-014: cache live types for 15s.
        out, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "live", "types", 15*time.Second, func() ([]svc.LiveTypeJSON, error) {
                lts, err := l.svcCtx.Models.LiveTypes.ListTopLevel(l.ctx)
                if err != nil {
                        return nil, err
                }
                return svc.BuildLiveTypesJSON(lts), nil
        })
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        return jsonBytes(out), CodeOK, ""
}

// AUDIT-001: /live/cateList dispatches here. Returns rooms filtered by
// liveTypeId (top-level parent), same shape as ListByType.
func (l *CallLogic) handleLiveCateList(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                LiveTypeID int64 `json:"liveTypeId"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.LiveTypeID == 0 {
                return nil, CodeBusinessError, "missing liveTypeId"
        }
        rooms, err := l.svcCtx.Models.Rooms.ListByType(l.ctx, req.LiveTypeID, 50)
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        resp := map[string]any{
                "rooms": svc.RoomsToResults(rooms),
        }
        return jsonBytes(resp), CodeOK, ""
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
        // AUDIT-014: cache room detail for 30s.
        out, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "detail:"+req.RoomNum, 30*time.Second, func() (map[string]any, error) {
                detail := svc.BuildRoomDetail(l.ctx, l.svcCtx.Models, req.RoomNum)
                if detail == nil {
                        return nil, nil
                }
                return detail, nil
        })
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
        if out == nil {
                return nil, CodeBusinessError, "room not found"
        }
        return jsonBytes(out), CodeOK, ""
}

func (l *CallLogic) handleRoomSchedule(in *apipro.CallReq) (json.RawMessage, int, string) {
        var req struct {
                RoomNum string `json:"roomNum"`
        }
        _ = json.Unmarshal([]byte(in.ParamJson), &req)
        if req.RoomNum == "" {
                return nil, CodeBusinessError, "missing roomNum"
        }
        // AUDIT-014: cache room schedule for 60s.
        out, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "schedule:"+req.RoomNum, 60*time.Second, func() (map[string][]svc.MatchItem, error) {
                return svc.BuildRoomSchedule(l.ctx, l.svcCtx.Models, req.RoomNum), nil
        })
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
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
        // AUDIT-014: cache gift rank for 30s.
        out, err := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "room", "gift_rank:"+req.RoomNum, 30*time.Second, func() ([]svc.GiftRankItem, error) {
                return svc.BuildGiftRank(l.ctx, l.svcCtx.Models, req.RoomNum), nil
        })
        if err != nil {
                return nil, CodeBusinessError, err.Error()
        }
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
        // AUDIT-013: use cache.GetOrLoad (singleflight + stats) instead of raw Get.
        out, _ := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "catalog", 60*time.Second, func() (map[string][]svc.MatchCatalogItem, error) {
                return svc.BuildMatchCatalog(l.ctx, l.svcCtx.Models), nil
        })
        return out
}

func (l *CallLogic) loadRecommend() []svc.MatchCatalogItem {
        out, _ := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "recommend", 60*time.Second, func() ([]svc.MatchCatalogItem, error) {
                return svc.BuildRecommend(l.ctx, l.svcCtx.Models), nil
        })
        return out
}

func (l *CallLogic) loadMatchByDate(date string) []svc.MatchCatalogItem {
        out, _ := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "match", "date:"+date, 60*time.Second, func() ([]svc.MatchCatalogItem, error) {
                return svc.BuildMatchByDate(l.ctx, l.svcCtx.Models, date), nil
        })
        return out
}

func (l *CallLogic) loadAllLiveRooms() map[string]any {
        out, _ := cache.GetOrLoad(l.ctx, l.svcCtx.Cache, "live", "all", 15*time.Second, func() (map[string]any, error) {
                return svc.BuildAllLiveRooms(l.ctx, l.svcCtx.Models), nil
        })
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

// vnPhoneRe matches Vietnam mobile numbers (9 digits, first digit in [35789]).
var vnPhoneRe = regexp.MustCompile(`^[35789][0-9]{8}$`)

// validatePhone checks phone format per country code (AUDIT-003).
//   - "86" / "+86": 11 digits starting with 1 (China mobile)
//   - "84" / "+84": 9 digits starting with [35789] (Vietnam mobile)
//   - other: at least 5 digits
func validatePhone(cc, phone string) bool {
        phone = strings.TrimSpace(phone)
        cc = normalizeCC(cc)
        if phone == "" {
                return false
        }
        switch cc {
        case "86":
                // China mobile: 11 digits, starts with 1.
                if len(phone) != 11 || phone[0] != '1' {
                        return false
                }
                for i := 0; i < len(phone); i++ {
                        if phone[i] < '0' || phone[i] > '9' {
                                return false
                        }
                }
                return true
        case "84":
                // Vietnam mobile: 9 digits, starts with [35789].
                return vnPhoneRe.MatchString(phone)
        default:
                // Generic: at least 5 digits.
                if len(phone) < 5 {
                        return false
                }
                for i := 0; i < len(phone); i++ {
                        if phone[i] < '0' || phone[i] > '9' {
                                return false
                        }
                }
                return true
        }
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
