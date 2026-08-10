package handler

import (
	"net/http"

	"apipro/cmd/api/internal/svc"
	"apipro/pkg/codec"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterHandlers wires up all routes.
//
// Routes are grouped into:
//  1. Plaintext JSONP endpoints (GET, no encryption) — /matches.json, /all_live_rooms.json, etc.
//  2. Encrypted endpoints (POST, behind codec.Transport middleware) — /login/*, /live/*, /match/*, /user/*, /sys/*
//  3. Static + WebSocket — /chat.html, /ws/chat, /health, /api/kaptcha
//
// DZ-6 fix (audit-1B): previously AuthLimiter (20/min/IP) was allocated in
// ServiceContext but never wired — dead code. Now /login/* and /sys/* get
// the tighter AuthLimiter on top of the global RateLimiter (120/min/IP) that
// runs as server-level middleware in apipro.go.
func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	// ---------- 1. Plaintext JSONP endpoints ----------
	jsonpRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/matches.json", Handler: MatchesJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/match_all.json", Handler: MatchesJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/all_live_rooms.json", Handler: AllLiveRoomsJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/live_types.json", Handler: LiveTypesJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/hot_anchor.json", Handler: HotAnchorJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/match_recommend.json", Handler: MatchRecommendJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/room/:roomNum/detail.json", Handler: RoomDetailJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/room/:roomNum/schedule.json", Handler: RoomScheduleJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/room/:roomNum/gift_rank.json", Handler: RoomGiftRankJSONPHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/match/:name", Handler: MatchDateJSONPHandler(svcCtx)},
	}
	server.AddRoutes(jsonpRoutes)

	// ---------- 2. Encrypted endpoints ----------
	// The codec.Transport middleware:
	//   - decrypts the request body with the RequestKey
	//   - passes the plaintext JSON to the handler via codec.ParamJSON(ctx)
	//   - encrypts the handler's JSON output with the ResponseKey
	transport := codec.Transport(svcCtx.Transport)
	encrypted := func(h http.HandlerFunc) http.HandlerFunc {
		return transport(http.Handler(h)).ServeHTTP
	}

	// DZ-6 fix: auth-sensitive routes get the AuthLimiter (20/min/IP) on top
	// of transport encryption. The global RateLimiter (120/min/IP) still runs
	// first as server middleware, so the effective cap for /login/* and /sys/*
	// is the TIGHTER of the two (20/min/IP).
	authMW := svcCtx.AuthLimiter.HTTPMiddleware()

	// 2a. Auth + SMS routes (tighter rate limit)
	authRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/login/login", Handler: authMW(encrypted(LoginHandler(svcCtx)))},
		{Method: http.MethodPost, Path: "/login/reg", Handler: authMW(encrypted(RegisterHandler(svcCtx)))},
		{Method: http.MethodPost, Path: "/login/guestLogin", Handler: authMW(encrypted(GuestLoginHandler(svcCtx)))},
		{Method: http.MethodPost, Path: "/login/refresh", Handler: authMW(encrypted(RefreshHandler(svcCtx)))},
		{Method: http.MethodPost, Path: "/login/logout", Handler: authMW(encrypted(LogoutHandler(svcCtx)))},
		{Method: http.MethodPost, Path: "/sys/getSmsCode", Handler: authMW(encrypted(SmsGetCodeHandler(svcCtx)))},
	}
	server.AddRoutes(authRoutes)

	// 2b. All other encrypted routes (display data — keep at global 120/min/IP)
	encryptedRoutes := []rest.Route{
		// live
		{Method: http.MethodPost, Path: "/live/hot", Handler: encrypted(LiveHotHandler(svcCtx))},
		{Method: http.MethodPost, Path: "/live/cateList", Handler: encrypted(LiveCateListHandler(svcCtx))},
		{Method: http.MethodPost, Path: "/live/detail", Handler: encrypted(LiveDetailHandler(svcCtx))},
		// match
		{Method: http.MethodPost, Path: "/match/recommend", Handler: encrypted(MatchRecommendHandler(svcCtx))},
		{Method: http.MethodPost, Path: "/match/cateList", Handler: encrypted(MatchCateListHandler(svcCtx))},
		{Method: http.MethodPost, Path: "/match/detail", Handler: encrypted(MatchDetailHandler(svcCtx))},
		// user
		{Method: http.MethodPost, Path: "/user/detail", Handler: encrypted(UserDetailHandler(svcCtx))},
	}
	server.AddRoutes(encryptedRoutes)

	// ---------- 3. Static + WebSocket + health ----------
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/api/kaptcha",
		Handler: KaptchaHandler(svcCtx),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/health",
		Handler: HealthHandler(svcCtx),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/chat.html",
		Handler: staticFile("public/chat.html", "text/html; charset=utf-8"),
	})
	server.AddRoute(rest.Route{
		Method:  http.MethodGet,
		Path:    "/ws/chat",
		Handler: svcCtx.ChatHub.ServeWS,
	})
}

// staticFile serves a file from disk.
func staticFile(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := readStaticFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write(b)
	}
}
