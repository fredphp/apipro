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
	"apipro/common/degrade"
	"apipro/common/model"
	"apipro/common/ratelimit"
	"apipro/desc/proto/gen/apipro"

	"github.com/zeromicro/go-zero/core/logx"
)

// Auth/business error codes — match backend-zero's auth_codes.go.
const (
	CodeOK              = 200
	CodeLoginRequired   = 100
	CodeGuestReauth     = 101
	CodeBusinessError   = 400
	CodeAccountNotFound = 4101
	CodePasswordWrong   = 4102
	CodeUserBanned      = 4103
	CodePhoneAlreadyReg = 4104
	CodeLoginLocked     = 4105
	CodeSmsCheckFailed  = 4106
	CodeRateLimited     = 4113
	CodePhoneInvalid    = 4114
	CodeNickNameBanned  = 4131
	CodeKaptchaInvalid  = 4120
	CodeSensitiveWord   = 1002
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
	case "match_recommend", "match_cateList": // encrypted /match/recommend + /match/cateList
		// Phase 4: both endpoints share cache key "match:recommend" and return the
		// same {count, pageNum, matches} shape. Pre-Phase-4 the cateList path
		// used a separate V2 sample handler; that handler has been merged into
		// handleMatchRecommend which is now itself V2 (Manager.GetOrLoadT).
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

	// AUDIT-003: phone format validation.
	if !validatePhone(cc, req.Phone) {
		return nil, CodePhoneInvalid, "手机号码格式错误"
	}

	// ---- Phase 2 sample: DegradeManager check (Level 2 = Auth) ----
	// Emergency 模式下 Level 2 (login) 仍服务 — 保 /login/login 可用
	// 但如果未来扩展为 Protected 也关闭 login，改这里即可
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，登录暂不可用"
	}

	// AUDIT-003: per-(cc,phone) rate limit — 10/min.
	rdb := l.svcCtx.Redis
	loginLimiter := ratelimit.New(rdb, 10, "yuyan:ratelimit:login")
	if !loginLimiter.Allow(l.ctx, cc+":"+req.Phone) {
		return nil, CodeRateLimited, "操作过于频繁，请稍后再试"
	}
	// AUDIT-003: fail-lockout check — if a lock key exists, refuse.
	lockKey := "yuyan:login:lock:" + cc + ":" + req.Phone
	if locked, _ := rdb.Get(lockKey); locked != "" {
		return nil, CodeLoginLocked, "账号已锁定，请30分钟后再试"
	}

	// ---- Phase 2 sample: DB query wrapped in DBSemaphore.WithToken ----
	// 防止 DB 连接池被打满时所有 goroutine 阻塞在 QueryContext
	// nil Semaphore (maxConcurrent<=0) 时 WithToken 直接执行 f
	var u *model.User
	var findErr error
	dbQueryErr := l.svcCtx.DBSem.WithToken(l.ctx, func() error {
		u, findErr = l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone)
		return findErr
	})
	if dbQueryErr != nil {
		// ctx 取消（信号量等待超时）— 快速失败
		if errors.Is(dbQueryErr, context.Canceled) || errors.Is(dbQueryErr, context.DeadlineExceeded) {
			l.recordDBErrorForDegrade()
			return nil, CodeBusinessError, "服务繁忙，请稍后再试"
		}
		if errors.Is(dbQueryErr, model.ErrNotFound) {
			return nil, CodeAccountNotFound, "账号未注册"
		}
		// 真实 DB 错误 → 触发降级评估
		l.recordDBErrorForDegrade()
		return nil, CodeBusinessError, dbQueryErr.Error()
	}
	// Verify password
	if u.PwdType != auth.PwdTypeMD5 {
		l.recordLoginFail(cc, req.Phone)
		return nil, CodePasswordWrong, "密码错误"
	}
	if !auth.VerifyPassword(req.Password, u.Password, u.Salt) {
		l.recordLoginFail(cc, req.Phone)
		return nil, CodePasswordWrong, "密码错误"
	}
	// Check status
	if u.Status != 1 {
		return nil, CodeUserBanned, "账号已封禁"
	}
	// AUDIT-003: clear fail counter on success.
	_, _ = rdb.Del("yuyan:login:fail:" + cc + ":" + req.Phone)

	// ---- Phase 2 sample: Session issue ----
	// Sessions.IssueUser 走 Redis（不涉及 DB），不需要 Semaphore 包装
	sess, sessErr := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), req.Plat)
	if sessErr != nil {
		return nil, CodeBusinessError, "issue session: " + sessErr.Error()
	}
	resp := l.buildAuthResponse(sess, u)
	return jsonBytes(resp), CodeOK, ""
}

// recordDBErrorForDegrade — Phase 2 sample: 连续 DB 错误时把 DegradeManager 推到 Degraded
// 简单实现：每次 DB 错误都尝试 PromoteIfWorse(Degraded)。
// DegradeManager 内部有 PromoteIfWorse 防回退（不会从 Protected 退到 Degraded）。
// 生产可改为：用 sliding window 统计 1min 内错误数，超阈值才 Promote。
func (l *CallLogic) recordDBErrorForDegrade() {
	if l.svcCtx.Degrade == nil {
		return
	}
	// 仅当当前是 Normal 时才 Promote（避免抖动）
	if l.svcCtx.Degrade.Mode() == degrade.ModeNormal {
		l.svcCtx.Degrade.PromoteIfWorse(degrade.ModeDegraded)
		logx.Errorf("Phase2: DB error triggered DegradeManager → DEGRADED")
	}
}

// handleCacheError — Phase 4 helper: 把 cache.Manager 返回的错误映射为业务 (code, meg)。
//   - ErrDegradeClosed → 4113 (CodeRateLimited) + "服务降级中"
//   - ErrCircuitOpen  → 400 (CodeBusinessError) + "服务暂时不可用"
//   - 其他            → 400 + err.Error()
//
// 所有 Level 1/2 接口在调用 cache.GetOrLoadT 后都用这个 helper 统一映射。
func handleCacheError(err error) (int, string) {
	if errors.Is(err, cache.ErrDegradeClosed) {
		return CodeRateLimited, "服务降级中，请稍后再试"
	}
	if errors.Is(err, cache.ErrCircuitOpen) {
		return CodeBusinessError, "服务暂时不可用，请稍后再试"
	}
	return CodeBusinessError, err.Error()
}

// dbWithSem — Phase 4 helper: 在 DBSemaphore.WithToken 内执行 f，统一处理
//   - ctx 取消/超时 → 触发降级 + 返回 (CodeBusinessError, "服务繁忙")
//   - 真实 DB 错误  → 触发降级评估 + 透传 err.Error()
//   - nil Semaphore → 直接执行 f（no-op）
//
// 返回 (dbErr, code, meg)。调用方根据 dbErr == nil 判断是否成功；
// 当 dbErr != nil 时 code/meg 已是最终业务响应，调用方应直接 return。
//
// 注意：调用方仍需自己处理 model.ErrNotFound / model.ErrDuplicate 等业务语义错误，
// 这些不会被本 helper 当作 DB 错误（f 应把它们包成 nil 返回，由调用方判断具体值）。
func (l *CallLogic) dbWithSem(f func() error) (dbErr error, code int, meg string) {
	err := l.svcCtx.DBSem.WithToken(l.ctx, f)
	if err == nil {
		return nil, CodeOK, ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		l.recordDBErrorForDegrade()
		return err, CodeBusinessError, "服务繁忙，请稍后再试"
	}
	l.recordDBErrorForDegrade()
	return err, CodeBusinessError, err.Error()
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
	if req.AccountType != 1 || req.Phone == "" || req.NickName == "" || req.PwdType != auth.PwdTypeMD5 {
		return nil, CodeBusinessError, "注册失败"
	}
	// AUDIT-005: nickname length check (>= 2 runes).
	if utf8.RuneCountInString(req.NickName) < 2 {
		return nil, CodeBusinessError, "昵称至少2个字符"
	}
	cc := normalizeCC(req.CountryCode)
	// AUDIT-003: phone format validation.
	if !validatePhone(cc, req.Phone) {
		return nil, CodePhoneInvalid, "手机号码格式错误"
	}

	// ---- Phase 3 sample: DegradeManager fail-CLOSED (Level 3 = Write) ----
	// Level 3 在任何非 Normal 模式下都拒绝：
	//   - Degraded/Protected: 不能写入（避免数据不一致）
	//   - Emergency: 全系统压力巨大，拒绝新写
	// 这与 audit-1C 决策一致：写接口用 fail-CLOSED 牺牲少量可用性换安全
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelWrite) {
		logx.Infof("Phase3: register rejected due to degrade mode=%v", l.svcCtx.Degrade.Mode())
		return nil, CodeRateLimited, "服务降级中，注册暂不可用，请稍后再试"
	}

	// ---- Phase 3 sample: 幂等性检查 ----
	// 用 (cc, phone) 作幂等 key（同一手机号 60s 内重复注册请求返回上次结果）
	// 防止客户端网络抖动导致的重复 INSERT 尝试
	idemKey := "yuyan:reg:idem:" + cc + ":" + req.Phone
	if cached, _ := l.svcCtx.Redis.Get(idemKey); cached != "" {
		logx.Infof("Phase3: register idempotent hit phone=%s returning cached result", req.Phone)
		// cached 是 JSON-encoded AuthResponse；直接返回
		return json.RawMessage(cached), CodeOK, ""
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

	// ---- Phase 3 sample: DBSemaphore.WithToken 包装 dup-check + INSERT ----
	// 把 phone dup-check 和 INSERT 放在同一个 token 持有期内，避免 TOCTOU 竞态
	// (传统实现是两次独立 DB 调用，中间存在窗口)
	var sess *auth.Session
	var u *model.User
	dbErr := l.svcCtx.DBSem.WithToken(l.ctx, func() error {
		// Check phone not already registered
		if existing, err := l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone); err == nil && existing != nil {
			return model.ErrDuplicate // 复用 ErrDuplicate 表达"已存在"
		}
		// AUDIT-008: allocate UID atomically via Redis INCR (avoids race).
		// 注意：AllocUID 走 Redis，不在 DB Semaphore 内（不占 DB 连接）
		uid, err := l.svcCtx.AllocUID(l.ctx)
		if err != nil {
			return err
		}
		// Hash password: salt = base64(32 random), stored = md5(client_md5 + salt)
		stored, salt, err := auth.HashPassword(req.Password)
		if err != nil {
			return err
		}
		u = &model.User{
			UID:         uid,
			LoginName:   req.Phone,
			NickName:    req.NickName,
			Phone:       req.Phone,
			CountryCode: cc,
			Password:    stored,
			Salt:        salt,
			PwdType:     auth.PwdTypeMD5,
			UserType:    1,
			Status:      1,
			Icon:        req.Icon,
			Gender:      0,
			Plat:        int32(req.Plat),
		}
		if err := l.svcCtx.Models.Users.Insert(l.ctx, u); err != nil {
			// AUDIT-008: PK violation on uid is a race / counter collision, NOT a
			// phone duplication — return a generic error so the user retries.
			if errors.Is(err, model.ErrDuplicate) {
				logx.Errorf("register: uid collision uid=%d (retry recommended)", uid)
				return fmt.Errorf("注册失败请重试")
			}
			return err
		}
		return nil
	})
	if dbErr != nil {
		// 区分 ErrDuplicate (phone 已注册) vs 其他 DB 错误
		if errors.Is(dbErr, model.ErrDuplicate) {
			return nil, CodePhoneAlreadyReg, "手机号码已被注册"
		}
		// ctx 取消（信号量等待超时）→ 快速失败 + 触发降级
		if errors.Is(dbErr, context.Canceled) || errors.Is(dbErr, context.DeadlineExceeded) {
			l.recordDBErrorForDegrade()
			return nil, CodeBusinessError, "服务繁忙，请稍后再试"
		}
		// 真实 DB 错误 → 触发降级评估
		l.recordDBErrorForDegrade()
		return nil, CodeBusinessError, dbErr.Error()
	}

	// Session issue (走 Redis，不占 DB semaphore)
	sess, sessErr := l.svcCtx.Sessions.IssueUser(l.ctx, u.UID, u.NickName, u.Icon, int(u.UserType), req.Plat)
	if sessErr != nil {
		return nil, CodeBusinessError, "issue session: " + sessErr.Error()
	}
	resp := l.buildAuthResponse(sess, u)

	// ---- Phase 3 sample: 幂等结果缓存 ----
	// 把成功结果缓存 60s，同一 phone 重复请求直接返回（防止客户端重试导致重复 INSERT）
	if respBytes, mErr := json.Marshal(resp); mErr == nil {
		_ = l.svcCtx.Redis.Setex(idemKey, string(respBytes), 60)
	}
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
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// guestLogin is Redis-only (Sessions.IssueGuest) but still subject to
	// DegradeManager — Emergency mode keeps Level 2 open so users can re-auth.
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，登录暂不可用"
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
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// Emergency mode keeps Level 2 open so users can refresh an expired access
	// token during an incident.
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，登录暂不可用"
	}
	// ---- Phase 4: DBSemaphore wraps the statusFn (FindByUid inside) ----
	// statusFn is called by Sessions.Refresh to check user ban status; we wrap
	// it so the DB lookup goes through the semaphore.
	statusFn := func(uid int64) (int, error) {
		var u *model.User
		dbErr, code, meg := l.dbWithSem(func() error {
			var fErr error
			u, fErr = l.svcCtx.Models.Users.FindByUid(l.ctx, uid)
			return fErr
		})
		if dbErr != nil {
			// dbWithSem already mapped to (code, meg) — but statusFn returns error
			// to Sessions.Refresh, not (code, meg). So we log + return the raw err.
			_ = code
			_ = meg
			return 0, dbErr
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
	// ---- Phase 4: DBSemaphore wraps the post-refresh FindByUid ----
	var u *model.User
	dbErr, code, meg := l.dbWithSem(func() error {
		var fErr error
		u, fErr = l.svcCtx.Models.Users.FindByUid(l.ctx, sess.UID)
		return fErr
	})
	if dbErr != nil {
		return nil, code, meg
	}
	resp := l.buildAuthResponse(sess, u)
	return jsonBytes(resp), CodeOK, ""
}

func (l *CallLogic) handleLogout(in *apipro.CallReq) (json.RawMessage, int, string) {
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// Logout is Redis-only (Sessions.Revoke) but stays open in Emergency so
	// users can escape a bad session during an incident.
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，登出暂不可用"
	}
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
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// Emergency mode keeps Level 2 open so the client can still fetch user
	// profile to decide whether to show login-required UI.
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，用户信息暂不可用"
	}
	sess, err := l.svcCtx.Sessions.Get(l.ctx, in.SessionId)
	if err != nil || sess == nil || sess.IsGuest {
		return nil, CodeLoginRequired, "login required"
	}
	// ---- Phase 4: DBSemaphore wraps FindByUid AND BuildUserInfo ----
	// BuildUserInfo internally calls FindUserGrowForValue (another DB query),
	// so the whole block must be inside one semaphore token.
	var u *model.User
	var userInfo svc.UserInfoResult
	dbErr, code, meg := l.dbWithSem(func() error {
		var fErr error
		u, fErr = l.svcCtx.Models.Users.FindByUid(l.ctx, sess.UID)
		if fErr != nil {
			return fErr
		}
		userInfo = svc.BuildUserInfo(l.ctx, l.svcCtx.Models, u)
		return nil
	})
	if dbErr != nil {
		return nil, code, meg
	}
	if u.Status != 1 {
		return nil, CodeUserBanned, "账号已封禁"
	}
	resp := map[string]any{
		"user": userInfo,
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
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// SMS issue is part of the auth flow (register/forget/bind), so it follows
	// Level 2 policy. Emergency mode keeps it open so users can still request
	// SMS during an incident (e.g. to re-bind a phone).
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，验证码暂不可用"
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
	// ---- Phase 4: DBSemaphore wraps FindByPhone dup-check ----
	// AUDIT-004: register guard — if type=1 and phone already registered, refuse.
	if req.Type == 1 {
		var existing *model.User
		dbErr, code, meg := l.dbWithSem(func() error {
			var fErr error
			existing, fErr = l.svcCtx.Models.Users.FindByPhone(l.ctx, cc, req.Phone)
			// ErrNotFound is a business outcome ("not registered”) — not a DB error.
			// Don't surface it as a DB error to dbWithSem.
			if errors.Is(fErr, model.ErrNotFound) {
				return nil
			}
			return fErr
		})
		if dbErr != nil {
			return nil, code, meg
		}
		if existing != nil {
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
	// ---- Phase 4: Level 2 (auth) degrade check ----
	// SMS verify is part of the auth flow (register/forget/bind), so it follows
	// Level 2 policy.
	if !l.svcCtx.Degrade.CanServeLevel(degrade.LevelAuth) {
		return nil, CodeRateLimited, "服务降级中，验证码暂不可用"
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

// ---- Phase 4: Level 1 (display) migration ----
// All Level 1 match/live/room handlers below use cache.GetOrLoadT + Manager
// (L1+L2+SF+Breaker+Fallback+Degrade+Metrics). Cache keys are identical to
// the legacy cache.GetOrLoad paths, so the scheduler's warmers (which still
// use legacy cache.Refresh) keep working — Manager.GetOrLoad reads the same
// apipro:<family>:<key> Redis key.
//
// Migration pattern:
//   items, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, <family>, <key>,
//       <ttl>, degrade.LevelDisplay, func() (T, error) { ... })
//   if err != nil {
//       code, meg := handleCacheError(err)
//       return nil, code, meg
//   }

func (l *CallLogic) handleMatchesJSONP() (json.RawMessage, int, string) {
	catalog, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "catalog",
		60*time.Second, degrade.LevelDisplay,
		func() (map[string][]svc.MatchCatalogItem, error) {
			return svc.BuildMatchCatalog(l.ctx, l.svcCtx.Models), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(catalog), CodeOK, ""
}

func (l *CallLogic) handleMatchRecommend() (json.RawMessage, int, string) {
	items, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "recommend",
		60*time.Second, degrade.LevelDisplay,
		func() ([]svc.MatchCatalogItem, error) {
			return svc.BuildRecommend(l.ctx, l.svcCtx.Models), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	resp := map[string]any{
		"count":   len(items),
		"pageNum": 1,
		"matches": items,
	}
	return jsonBytes(resp), CodeOK, ""
}

// (Phase 4) handleMatchCateListV2 has been removed.
// Pre-Phase-4 the /match/cateList path used a separate V2 sample handler to
// demonstrate the Manager.GetOrLoadT pattern. As of Phase 4 the canonical
// handleMatchRecommend above IS the V2 path (same cache key "match:recommend",
// same {count, pageNum, matches} shape), so the dispatch in Call() now maps
// both "match_recommend" and "match_cateList" RPC methods to handleMatchRecommend.

func (l *CallLogic) handleMatchDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
	var req matchDetailReq
	_ = json.Unmarshal([]byte(in.ParamJson), &req)
	if req.ScheduleID == 0 {
		return nil, CodeBusinessError, "missing scheduleId"
	}
	// Phase 4: migrated to Manager.GetOrLoadT (L1+L2+SF+Breaker+Fallback+Degrade).
	// AUDIT-014: cache the (match, rooms) payload for 60s.
	cacheKey := "detail:" + strconv.FormatInt(req.ScheduleID, 10)
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", cacheKey,
		60*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			return l.buildMatchDetailPayload(req.ScheduleID)
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
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
	items, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "date:"+req.Date,
		60*time.Second, degrade.LevelDisplay,
		func() ([]svc.MatchCatalogItem, error) {
			return svc.BuildMatchByDate(l.ctx, l.svcCtx.Models, req.Date), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(items), CodeOK, ""
}

// =============================================================
// Live handlers
// =============================================================

func (l *CallLogic) handleAllLiveRooms() (json.RawMessage, int, string) {
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "live", "all",
		15*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			return svc.BuildAllLiveRooms(l.ctx, l.svcCtx.Models), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(out), CodeOK, ""
}

func (l *CallLogic) handleLiveTypes() (json.RawMessage, int, string) {
	// Phase 4: migrated to Manager.GetOrLoadT (L1+L2+SF+Breaker+Fallback+Degrade).
	// AUDIT-014: cache live types for 15s.
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "live", "types",
		15*time.Second, degrade.LevelDisplay,
		func() ([]svc.LiveTypeJSON, error) {
			lts, err := l.svcCtx.Models.LiveTypes.ListTopLevel(l.ctx)
			if err != nil {
				return nil, err
			}
			return svc.BuildLiveTypesJSON(lts), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(out), CodeOK, ""
}

// AUDIT-001: /live/cateList dispatches here. Returns rooms filtered by
// liveTypeId (top-level parent), same shape as ListByType.
//
// Phase 4: migrated to Manager.GetOrLoadT (L1+L2+SF+Breaker+Fallback+Degrade).
// The cache key is per-liveTypeId so different categories don't collide.
// Same key as the pre-Phase-4 path ("listByType:<id>"), so the scheduler's
// warmer (if any) keeps working.
//
// Level 1 (display) resilience: loader swallows DB errors and returns an
// empty rooms slice — matches BuildRecommend/BuildAllLiveRooms behavior so
// the client sees a 200 with empty list instead of a 400 when DB is down.
func (l *CallLogic) handleLiveCateList(in *apipro.CallReq) (json.RawMessage, int, string) {
	var req struct {
		LiveTypeID int64 `json:"liveTypeId"`
	}
	_ = json.Unmarshal([]byte(in.ParamJson), &req)
	if req.LiveTypeID == 0 {
		return nil, CodeBusinessError, "missing liveTypeId"
	}
	cacheKey := "listByType:" + strconv.FormatInt(req.LiveTypeID, 10)
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "live", cacheKey,
		15*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			rooms, err := l.svcCtx.Models.Rooms.ListByType(l.ctx, req.LiveTypeID, 50)
			if err != nil {
				return map[string]any{"rooms": []svc.RoomResult{}}, nil
			}
			return map[string]any{"rooms": svc.RoomsToResults(rooms)}, nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(out), CodeOK, ""
}

// Phase 4: migrated to Manager.GetOrLoadT.
// Key "hot" under family "live" — NEW key (not shared with /all_live_rooms.json's
// apipro:live:all because that payload includes ALL types grouped; this is
// just the flat hot list).
//
// Level 1 (display) resilience: loader swallows DB errors and returns an empty
// hot slice — matches BuildAllLiveRooms behavior so the client sees a 200 with
// empty list instead of a 400 when DB is down.
func (l *CallLogic) handleLiveHot() (json.RawMessage, int, string) {
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "live", "hot",
		15*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			rooms, err := l.svcCtx.Models.Rooms.ListHot(l.ctx, 50)
			if err != nil {
				return map[string]any{"hot": []svc.RoomResult{}}, nil
			}
			return map[string]any{"hot": svc.RoomsToResults(rooms)}, nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	return jsonBytes(out), CodeOK, ""
}

// Phase 4: migrated to Manager.GetOrLoadT.
// Key "room:detail:<roomNum>" SHARES with the JSONP /room/<n>/detail.json path
// (handleRoomDetail), so the two paths warm each other and a hit on one is a
// hit on the other.
func (l *CallLogic) handleLiveDetail(in *apipro.CallReq) (json.RawMessage, int, string) {
	var req liveDetailReq
	_ = json.Unmarshal([]byte(in.ParamJson), &req)
	if req.RoomNum == "" {
		return nil, CodeBusinessError, "missing roomNum"
	}
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "room", "detail:"+req.RoomNum,
		30*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			detail := svc.BuildRoomDetail(l.ctx, l.svcCtx.Models, req.RoomNum)
			if detail == nil {
				return nil, nil
			}
			return detail, nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
	}
	if out == nil {
		return nil, CodeBusinessError, "room not found"
	}
	return jsonBytes(out), CodeOK, ""
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
	// Phase 4: migrated to Manager.GetOrLoadT (L1+L2+SF+Breaker+Fallback+Degrade).
	// AUDIT-014: cache room detail for 30s. SHARES the key with /live/detail
	// (handleLiveDetail) so the two paths warm each other.
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "room", "detail:"+req.RoomNum,
		30*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			detail := svc.BuildRoomDetail(l.ctx, l.svcCtx.Models, req.RoomNum)
			if detail == nil {
				return nil, nil
			}
			return detail, nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
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
	// Phase 4: migrated to Manager.GetOrLoadT. AUDIT-014: cache room schedule for 60s.
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "room", "schedule:"+req.RoomNum,
		60*time.Second, degrade.LevelDisplay,
		func() (map[string][]svc.MatchItem, error) {
			return svc.BuildRoomSchedule(l.ctx, l.svcCtx.Models, req.RoomNum), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
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
	// Phase 4: migrated to Manager.GetOrLoadT. AUDIT-014: cache gift rank for 30s.
	out, err := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "room", "gift_rank:"+req.RoomNum,
		30*time.Second, degrade.LevelDisplay,
		func() ([]svc.GiftRankItem, error) {
			return svc.BuildGiftRank(l.ctx, l.svcCtx.Models, req.RoomNum), nil
		},
	)
	if err != nil {
		code, meg := handleCacheError(err)
		return nil, code, meg
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

// loadMatchCatalog / loadRecommend / loadMatchByDate — Phase 4 inline migration.
// Previous helpers wrapped cache.GetOrLoad for two callers (the JSONP path and
// the encrypted path). After Phase 4 each handler calls cache.GetOrLoadT
// directly, so these helpers are no longer referenced. Kept as thin wrappers
// for any future caller; delete if unused.
//
// (Phase 4: kept as no-op fallbacks to avoid breaking any external caller.)
func (l *CallLogic) loadMatchCatalog() map[string][]svc.MatchCatalogItem {
	out, _ := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "catalog",
		60*time.Second, degrade.LevelDisplay,
		func() (map[string][]svc.MatchCatalogItem, error) {
			return svc.BuildMatchCatalog(l.ctx, l.svcCtx.Models), nil
		},
	)
	return out
}

func (l *CallLogic) loadRecommend() []svc.MatchCatalogItem {
	out, _ := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "recommend",
		60*time.Second, degrade.LevelDisplay,
		func() ([]svc.MatchCatalogItem, error) {
			return svc.BuildRecommend(l.ctx, l.svcCtx.Models), nil
		},
	)
	return out
}

func (l *CallLogic) loadMatchByDate(date string) []svc.MatchCatalogItem {
	out, _ := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "match", "date:"+date,
		60*time.Second, degrade.LevelDisplay,
		func() ([]svc.MatchCatalogItem, error) {
			return svc.BuildMatchByDate(l.ctx, l.svcCtx.Models, date), nil
		},
	)
	return out
}

// loadAllLiveRooms — Phase 4 inline migration. Kept as a thin wrapper for
// any future caller; the encrypted path /live/hot is now served directly by
// handleAllLiveRooms which calls cache.GetOrLoadT.
func (l *CallLogic) loadAllLiveRooms() map[string]any {
	out, _ := cache.GetOrLoadT(l.ctx, l.svcCtx.Manager, "live", "all",
		15*time.Second, degrade.LevelDisplay,
		func() (map[string]any, error) {
			return svc.BuildAllLiveRooms(l.ctx, l.svcCtx.Models), nil
		},
	)
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
