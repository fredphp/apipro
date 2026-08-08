package handler

import (
        "context"
        "crypto/rand"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "strconv"
        "strings"

        "apipro/cmd/api/internal/svc"
        "apipro/cmd/rpc/apiproClient"
        "apipro/pkg/codec"

        "github.com/zeromicro/go-zero/core/logx"
)

// alias so the kaptcha code reads naturally
var cryptoRand = rand.Reader

// rpcClient is a short alias for the apipro client.
type rpcClient = apiproClient.Apipro

// callRPC invokes the RPC `Call` method with the given method+param+session.
// Returns the parsed code/meg/result. On RPC error, returns a business-error
// envelope with the error message.
func callRPC(svcCtx *svc.ServiceContext, w http.ResponseWriter, r *http.Request, method, paramJSON string) (*apiproClient.CallResp, error) {
        sid := r.Header.Get("X-Session")
        cli := apiproClient.NewApipro(svcCtx.ApiproRpc)
        resp, err := cli.Call(r.Context(), &apiproClient.CallReq{
                Method:    method,
                ParamJson: paramJSON,
                SessionId: sid,
        })
        if err != nil {
                logx.Errorf("rpc.Call %s: %v", method, err)
                codec.WriteErr(w, 500, "rpc unavailable")
                return nil, err
        }
        return resp, nil
}

// readParamBody reads the decrypted business JSON from the request context
// (set by the codec.Transport middleware). Falls back to raw body if no
// transport was applied (debug path).
func readParamBody(r *http.Request) string {
        if b := codec.ParamJSON(r.Context()); b != nil {
                return string(b)
        }
        // Fallback: read raw body
        b, _ := io.ReadAll(r.Body)
        return string(b)
}

// =============================================================
// Encrypted auth handlers (POST /login/*, /sys/*, /user/*, etc.)
// All sit behind the codec.Transport middleware.
// =============================================================

// loginHandler — POST /login/login
func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "login", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// registerHandler — POST /login/reg
func RegisterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "register", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// guestLoginHandler — POST /login/guestLogin
func GuestLoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "guestLogin", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// refreshHandler — POST /login/refresh
func RefreshHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "refresh", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// logoutHandler — POST /login/logout
func LogoutHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "logout", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// =============================================================
// Encrypted match/live/room handlers
// =============================================================

func LiveHotHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                resp, err := callRPC(svcCtx, w, r, "live_hot", "{}")
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func LiveCateListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                // AUDIT-001: dispatch to live_cateList (rooms filtered by liveTypeId),
                // NOT live_types (which returns the live-type catalog for /live_types.json).
                resp, err := callRPC(svcCtx, w, r, "live_cateList", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func LiveDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "live_detail", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func MatchRecommendHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                resp, err := callRPC(svcCtx, w, r, "match_recommend", "{}")
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func MatchCateListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                resp, err := callRPC(svcCtx, w, r, "match_cateList", "{}")
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func MatchDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "match_detail", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

func UserDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                resp, err := callRPC(svcCtx, w, r, "user_detail", "{}")
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// =============================================================
// SMS + kaptcha
// =============================================================

func SmsGetCodeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                param := readParamBody(r)
                resp, err := callRPC(svcCtx, w, r, "sms_getCode", param)
                if err != nil {
                        return
                }
                writeEnvelope(w, resp)
        }
}

// KaptchaHandler — GET /api/kaptcha?mobile=<phone>
// Plaintext SVG image (NOT encrypted).
func KaptchaHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                mobile := r.URL.Query().Get("mobile")
                if mobile == "" {
                        http.Error(w, "missing mobile", http.StatusBadRequest)
                        return
                }
                // Generate 5-char code from visually-unambiguous alphabet.
                code := genKaptchaCode(5)
                // Store in Redis (5min TTL) under yuyan:kaptcha:<mobile>.
                _ = svcCtx.Redis.Setex("yuyan:kaptcha:"+mobile, code, 300)
                // Render SVG.
                svg := renderKaptchaSVG(code)
                w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
                w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
                w.WriteHeader(http.StatusOK)
                _, _ = w.Write([]byte(svg))
        }
}

// =============================================================
// Helpers
// =============================================================

// writeEnvelope writes a plaintext {code, meg, seq, newSessionId, result}
// envelope to the ResponseWriter. The codec.Transport middleware will
// encrypt this body before sending to the client.
func writeEnvelope(w http.ResponseWriter, resp *apiproClient.CallResp) {
        // If result is empty/nil, still emit a proper envelope.
        out := map[string]any{
                "code": resp.Code,
                "meg":  resp.Meg,
                "seq":  resp.Seq,
        }
        if resp.NewSessionId != "" {
                out["newSessionId"] = resp.NewSessionId
        }
        if len(resp.Result) > 0 {
                // Embed the raw result JSON directly (not as a string).
                out["result"] = json.RawMessage(resp.Result)
        }
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        enc := json.NewEncoder(w)
        _ = enc.Encode(out)
}

// genKaptchaCode returns N chars from a visually-unambiguous alphabet.
// Uses crypto/rand.
func genKaptchaCode(n int) string {
        const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
        out := make([]byte, n)
        for i := range out {
                b := make([]byte, 1)
                _, _ = cryptoRand.Read(b)
                out[i] = alphabet[int(b[0])%len(alphabet)]
        }
        return string(out)
}

func renderKaptchaSVG(code string) string {
        return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40">
  <rect width="120" height="40" fill="#f0f0f0"/>
  <text x="12" y="28" font-family="monospace" font-size="22" fill="#333">%s</text>
</svg>`, code)
}

// Suppress unused-import warnings.
var _ = context.Background
var _ = strconv.Itoa
var _ = strings.TrimSpace
