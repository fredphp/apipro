package codec

import (
        "bytes"
        "context"
        "encoding/json"
        "errors"
        "io"
        "net/http"
        "strconv"

        "apipro/desc/proto/gen/fy"

        "google.golang.org/protobuf/proto"
)

type ctxKey int

const (
        ctxParam ctxKey = iota
        ctxSeq
        ctxSessionID
        ctxPlat
        ctxWireProto // true if the request came in as protobuf FY_CLIENT
)

// TransportConfig holds the transport encryption keys.
//
// Per docs/password-login-register.txt there are TWO response keys:
//   - Web (plat=3): ResponseKey = qlCJekfRKwWkQxl7
//   - WAP (plat=4): WapResponseKey = PHp1st5vEg5Ca8FH (same as request key)
//
// WapResponseKey is optional — when empty, ResponseKey is used for all plats.
type TransportConfig struct {
        RequestKey     []byte // decrypt client→server (always PHp1st5vEg5Ca8FH in production)
        ResponseKey    []byte // encrypt server→client for Web (plat=3)
        WapResponseKey []byte // encrypt server→client for WAP (plat=4); optional
}

// ParamJSON returns the decrypted business-param JSON bytes from the request
// context (set by Transport middleware).
func ParamJSON(ctx context.Context) []byte {
        if v, ok := ctx.Value(ctxParam).([]byte); ok {
                return v
        }
        return nil
}

// SessionID returns the access token sent in the request envelope.
func SessionID(ctx context.Context) string {
        if v, ok := ctx.Value(ctxSessionID).(string); ok {
                return v
        }
        return ""
}

// Seq returns the client sequence id.
func Seq(ctx context.Context) int32 {
        if v, ok := ctx.Value(ctxSeq).(int32); ok {
                return v
        }
        return 0
}

// Plat returns the client platform code from CLIENT_INFO.
//   3 = Web, 4 = WAP
func Plat(ctx context.Context) int32 {
        if v, ok := ctx.Value(ctxPlat).(int32); ok {
                return v
        }
        return 0
}

// IsWireProto reports whether the request arrived as a protobuf FY_CLIENT
// envelope (true) or as a plain JSON body (false).
func IsWireProto(ctx context.Context) bool {
        if v, ok := ctx.Value(ctxWireProto).(bool); ok {
                return v
        }
        return false
}

// WithParamCtx stores the decrypted params + session info in ctx.
func WithParamCtx(ctx context.Context, param []byte, sessionID string, plat int32, seq int32) context.Context {
        ctx = context.WithValue(ctx, ctxParam, param)
        ctx = context.WithValue(ctx, ctxSessionID, sessionID)
        ctx = context.WithValue(ctx, ctxPlat, plat)
        ctx = context.WithValue(ctx, ctxSeq, seq)
        return ctx
}

// PlainResp is the plaintext JSON response envelope used by the legacy JSON
// wire path. The middleware wraps it inside the encrypted frame when the
// request was JSON; when the request was protobuf FY_CLIENT, the middleware
// instead builds a protobuf COMMON_RESP envelope (per spec).
type PlainResp struct {
        Code       int             `json:"code"`
        Meg        string          `json:"meg"`
        Seq        int32           `json:"seq"`
        NewSession string          `json:"newSessionId,omitempty"`
        Result     json.RawMessage `json:"result,omitempty"`
}

// Transport returns an http middleware that:
//  1. Reads the encrypted binary body.
//  2. Decrypts with RequestKey.
//  3. Parses the plaintext:
//     - If it starts with '{', treat as JSON (legacy envelope or raw business JSON).
//     - Otherwise parse as protobuf FY_CLIENT and extract COMMON_REQ.param + CLIENT_INFO.
//  4. Stashes the param JSON + session/seq/plat in ctx for the handler.
//  5. After the handler writes a JSON envelope, wraps it:
//     - If request was protobuf: build FY_CLIENT{ common_resp: { common_result, result } } protobuf.
//     - If request was JSON: keep as JSON envelope.
//  6. Encrypts with the plat-appropriate ResponseKey and writes the framed binary back.
func Transport(cfg TransportConfig) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
                return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                        body, err := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024))
                        if err != nil {
                                abortEncrypted(w, cfg.ResponseKey, 101, "invalid request body", 0, "")
                                return
                        }
                        _ = r.Body.Close()

                        // Decrypt: try framed first; if that fails, try raw AES (no header).
                        var plain []byte
                        if len(body) >= frameHeaderLen && body[0] == frameByte0 && body[1] == frameByte1 {
                                plain, err = DecodeFrame(body, cfg.RequestKey)
                        } else {
                                plain, err = Decrypt(body, cfg.RequestKey)
                        }
                        if err != nil {
                                abortEncrypted(w, cfg.ResponseKey, 101, "invalid encrypted request", 0, "")
                                return
                        }

                        // Parse the plaintext envelope.
                        var (
                                paramJSON  []byte
                                sessionID  string
                                plat       int32
                                seq        int32
                                wireProto  bool
                        )

                        if len(plain) > 0 && plain[0] == '{' {
                                // JSON path (legacy): {sessionId,seq,plat,param} OR raw business JSON.
                                if env, ok := tryParseEnvelope(plain); ok {
                                        paramJSON = env.Param
                                        sessionID = env.SessionID
                                        plat = env.PlatInt
                                        seq = env.Seq
                                } else {
                                        paramJSON = plain
                                        sessionID = r.Header.Get("X-Session")
                                        if v := r.Header.Get("X-Plat"); v != "" {
                                                if n, e := strconv.Atoi(v); e == nil {
                                                        plat = int32(n)
                                                }
                                        }
                                        if v := r.Header.Get("X-Seq"); v != "" {
                                                if n, e := strconv.Atoi(v); e == nil {
                                                        seq = int32(n)
                                                }
                                        }
                                }
                        } else {
                                // Protobuf FY_CLIENT path (per docs/password-login-register.txt).
                                var fc fy.FY_CLIENT
                                if perr := proto.Unmarshal(plain, &fc); perr == nil && fc.CommonReq != nil {
                                        wireProto = true
                                        if fc.CommonReq.ClientInfo != nil {
                                                ci := fc.CommonReq.ClientInfo
                                                sessionID = ci.SessionId
                                                seq = ci.Seq
                                                plat = ci.Plat
                                                // Doc-typo fallback: if plat (field 5) is 0 but app_ver
                                                // (field 3) is non-zero, the client may have encoded plat
                                                // at field 3 (the doc shows plat=3 colliding with app_ver).
                                                if plat == 0 && ci.AppVer != 0 {
                                                        plat = ci.AppVer
                                                }
                                        }
                                        paramJSON = fc.CommonReq.Param
                                        if paramJSON == nil {
                                                paramJSON = []byte("{}")
                                        }
                                } else {
                                        // Not a valid FY_CLIENT; fall back to treating as raw JSON
                                        // (best-effort for legacy clients/tests).
                                        paramJSON = plain
                                }
                        }

                        ctx := context.WithValue(r.Context(), ctxWireProto, wireProto)
                        ctx = WithParamCtx(ctx, paramJSON, sessionID, plat, seq)

                        // Capture the handler's plain-JSON output via a buffer.
                        buf := &captureWriter{header: make(http.Header), buf: &bytes.Buffer{}}
                        next.ServeHTTP(buf, r.WithContext(ctx))

                        // Build the response payload (protobuf or JSON depending on wire).
                        respBytes := buf.buf.Bytes()
                        var envelope []byte
                        if wireProto {
                                envelope = buildProtobufEnvelope(respBytes, seq, sessionID)
                        } else {
                                envelope = buildEnvelope(respBytes, seq, sessionID)
                        }

                        // Select response key by plat (WAP=4 uses WapResponseKey when set).
                        respKey := cfg.ResponseKey
                        if plat == 4 && len(cfg.WapResponseKey) > 0 {
                                respKey = cfg.WapResponseKey
                        }

                        ct, err := EncodeFrame(envelope, respKey)
                        if err != nil {
                                http.Error(w, "encrypt error", http.StatusInternalServerError)
                                return
                        }
                        w.Header().Set("Content-Type", "application/json charset=utf-8")
                        w.Header().Set("Content-Length", strconv.Itoa(len(ct)))
                        w.WriteHeader(http.StatusOK)
                        _, _ = w.Write(ct)
                })
        }
}

// envelope is the optional client→server JSON wrapper (legacy path).
type envelope struct {
        SessionID string          `json:"sessionId,omitempty"`
        Seq       int32           `json:"seq,omitempty"`
        Plat      string          `json:"plat,omitempty"`
        PlatInt   int32           `json:"-"` // parsed from Plat
        Param     json.RawMessage `json:"param,omitempty"`
}

func tryParseEnvelope(plain []byte) (*envelope, bool) {
        // Heuristic: must start with `{` and contain "param"
        if len(plain) == 0 || plain[0] != '{' {
                return nil, false
        }
        var e envelope
        if err := json.Unmarshal(plain, &e); err != nil {
                return nil, false
        }
        if len(e.Param) == 0 && e.SessionID == "" && e.Seq == 0 && e.Plat == "" {
                return nil, false
        }
        if len(e.Param) == 0 {
                return nil, false
        }
        // Parse plat: accept "3"/"4" or "Web"/"WAP" (case-insensitive).
        if e.Plat != "" {
                if n, err := strconv.Atoi(e.Plat); err == nil {
                        e.PlatInt = int32(n)
                }
        }
        return &e, true
}

// buildProtobufEnvelope wraps the handler's JSON output into a FY_CLIENT
// protobuf message per the spec:
//
//      FY_CLIENT.common_resp.common_result.{err_code, err_msg, seq, new_session_id}
//      FY_CLIENT.common_resp.result = <JSON bytes>
//
// If the handler wrote a {code,meg,seq,newSessionId,result} JSON envelope,
// those fields are mapped to the protobuf fields. Otherwise the body is
// treated as the `result` bytes with err_code=200.
func buildProtobufEnvelope(respBytes []byte, seq int32, sessionID string) []byte {
        var pr PlainResp
        code := int32(200)
        meg := ""
        newSess := ""
        result := respBytes

        if err := json.Unmarshal(respBytes, &pr); err == nil && pr.Code != 0 {
                code = int32(pr.Code)
                meg = pr.Meg
                if pr.Seq != 0 {
                        seq = pr.Seq
                }
                newSess = pr.NewSession
                if len(pr.Result) > 0 {
                        result = pr.Result
                } else {
                        result = nil
                }
        } else {
                // Wrap raw body as success result.
                if sessionID != "" {
                        newSess = sessionID
                }
        }

        cr := &fy.COMMON_RESULT{
                ErrCode:      code,
                ErrMsg:       meg,
                Seq:          seq,
                NewSessionId: newSess,
        }
        creq := &fy.COMMON_RESP{
                CommonResult: cr,
                Result:       result,
        }
        fc := &fy.FY_CLIENT{CommonResp: creq}
        out, err := proto.Marshal(fc)
        if err != nil {
                // Fallback: empty error envelope
                fc2 := &fy.FY_CLIENT{CommonResp: &fy.COMMON_RESP{
                        CommonResult: &fy.COMMON_RESULT{ErrCode: 500, ErrMsg: "marshal error", Seq: seq},
                }}
                out, _ = proto.Marshal(fc2)
        }
        return out
}

func buildEnvelope(respBytes []byte, seq int32, sessionID string) []byte {
        // Try parse as PlainResp.
        var pr PlainResp
        if err := json.Unmarshal(respBytes, &pr); err == nil && pr.Code != 0 {
                // Already an envelope — fill in seq if missing.
                if pr.Seq == 0 {
                        pr.Seq = seq
                }
                out, err := json.Marshal(pr)
                if err == nil {
                        return out
                }
        }
        // Wrap as success result.
        pr2 := PlainResp{Code: 200, Seq: seq, Result: respBytes}
        if sessionID != "" {
                pr2.NewSession = sessionID
        }
        out, _ := json.Marshal(pr2)
        return out
}

// abortEncrypted writes an error envelope directly. Uses Web ResponseKey by
// default (the request hadn't been parsed yet, so plat is unknown).
func abortEncrypted(w http.ResponseWriter, respKey []byte, code int, msg string, seq int32, newSessionID string) {
        pr := PlainResp{Code: code, Meg: msg, Seq: seq, NewSession: newSessionID}
        body, _ := json.Marshal(pr)
        ct, err := EncodeFrame(body, respKey)
        if err != nil {
                http.Error(w, "encrypt error", http.StatusInternalServerError)
                return
        }
        w.Header().Set("Content-Type", "application/json charset=utf-8")
        w.Header().Set("Content-Length", strconv.Itoa(len(ct)))
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(ct)
}

// WriteOK writes a plaintext success envelope to the ResponseWriter. The
// middleware will encrypt it. result may be nil for empty body.
func WriteOK(w http.ResponseWriter, result any) {
        WriteResp(w, 200, "", result)
}

// WriteErr writes a plaintext error envelope.
func WriteErr(w http.ResponseWriter, code int, meg string) {
        WriteResp(w, code, meg, nil)
}

// WriteResp writes a fully-customized plaintext envelope.
func WriteResp(w http.ResponseWriter, code int, meg string, result any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        var raw json.RawMessage
        if result != nil {
                b, err := json.Marshal(result)
                if err != nil {
                        w.WriteHeader(http.StatusInternalServerError)
                        _, _ = w.Write([]byte(`{"code":500,"meg":"marshal error"}`))
                        return
                }
                raw = b
        }
        pr := PlainResp{Code: code, Meg: meg, Result: raw}
        b, _ := json.Marshal(pr)
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

// WriteRaw lets the handler write a pre-built envelope (already in
// {code,meg,seq,result} shape) as raw bytes.
func WriteRaw(w http.ResponseWriter, b []byte) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write(b)
}

// captureWriter buffers the handler's plain-JSON output. It ignores status
// codes from the handler (we always re-encode into an envelope and write 200).
type captureWriter struct {
        header http.Header
        buf    *bytes.Buffer
}

func (c *captureWriter) Header() http.Header       { return c.header }
func (c *captureWriter) WriteHeader(statusCode int) {}
func (c *captureWriter) Write(p []byte) (int, error) { return c.buf.Write(p) }

// Errors that handlers may return to indicate auth failures.
var (
        ErrLoginRequired = errors.New("login required")
        ErrGuestReauth   = errors.New("guest reauth")
)
