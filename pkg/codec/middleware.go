package codec

import (
        "bytes"
        "context"
        "encoding/json"
        "errors"
        "io"
        "net/http"
        "strconv"
)

type ctxKey int

const (
        ctxParam ctxKey = iota
        ctxSeq
        ctxSessionID
        ctxPlat
)

// TransportConfig holds the two 16-byte ASCII keys.
type TransportConfig struct {
        RequestKey  []byte // decrypt client→server
        ResponseKey []byte // encrypt server→client
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

// WithParamCtx stores the decrypted params + session info in ctx.
func WithParamCtx(ctx context.Context, param []byte, sessionID, plat string, seq int32) context.Context {
        ctx = context.WithValue(ctx, ctxParam, param)
        ctx = context.WithValue(ctx, ctxSessionID, sessionID)
        ctx = context.WithValue(ctx, ctxPlat, plat)
        ctx = context.WithValue(ctx, ctxSeq, seq)
        return ctx
}

// PlainResp is the plaintext response envelope returned by handlers. The
// middleware wraps it inside the encrypted frame: {code, meg, seq, new_session_id, result}.
type PlainResp struct {
        Code        int             `json:"code"`
        Meg         string          `json:"meg"`
        Seq         int32           `json:"seq"`
        NewSession  string          `json:"newSessionId,omitempty"`
        Result      json.RawMessage `json:"result,omitempty"`
}

// Transport returns an http middleware that:
//  1. Reads the encrypted binary body.
//  2. Decrypts with RequestKey (the plaintext is the raw business JSON).
//  3. Stashes the JSON in ctx for the handler.
//  4. After the handler writes a JSON envelope {code, meg, seq, result} (plain
//     JSON), encrypts it with ResponseKey and writes the framed binary back.
//
// The inner envelope plaintext IS the JSON envelope directly (no protobuf).
//
// Handler contract: handlers should write `WriteResp(w, ctx, code, meg, result)`
// OR any plain JSON of the envelope shape. For convenience, handlers can use
// `WriteOK`/`WriteErr` helpers which produce the right shape.
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

                        // The plaintext may be the bare business JSON OR an envelope
                        // {clientInfo:{sessionId, seq, plat}, param:<json>}. We support
                        // both forms: if it parses as envelope with a `param` field, use
                        // the param bytes and extract session/seq/plat. Otherwise treat
                        // the whole plaintext as the business param.
                        paramJSON := plain
                        var sessionID, plat string
                        var seq int32
                        if env, ok := tryParseEnvelope(plain); ok {
                                paramJSON = env.Param
                                sessionID = env.SessionID
                                plat = env.Plat
                                seq = env.Seq
                        } else {
                                // Fallback: read session/plat/seq from headers.
                                sessionID = r.Header.Get("X-Session")
                                plat = r.Header.Get("X-Plat")
                                if v := r.Header.Get("X-Seq"); v != "" {
                                        if n, err := strconv.Atoi(v); err == nil {
                                                seq = int32(n)
                                        }
                                }
                        }
                        ctx := WithParamCtx(r.Context(), paramJSON, sessionID, plat, seq)

                        // Capture the handler's plain-JSON output via a buffer.
                        buf := &captureWriter{header: make(http.Header), buf: &bytes.Buffer{}}
                        next.ServeHTTP(buf, r.WithContext(ctx))

                        // Determine the response envelope. If the handler already wrote a
                        // {code,meg,seq,result,...} envelope, use as-is. Otherwise wrap
                        // the body as `result` with code=200.
                        respBytes := buf.buf.Bytes()
                        envelope := buildEnvelope(respBytes, seq, sessionID)

                        ct, err := EncodeFrame(envelope, cfg.ResponseKey)
                        if err != nil {
                                http.Error(w, "encrypt error", http.StatusInternalServerError)
                                return
                        }
                        w.Header().Set("Content-Type", "application/json charset=utf-8")
                        w.Header().Set("Content-Length", strconv.Itoa(len(ct)))
                        w.WriteHeader(http.StatusOK)
                        _, _ = w.Write(ct)
                        _ = ctx // suppress unused
                })
        }
}

// envelope is the optional client→server wrapper carrying session/seq/plat.
type envelope struct {
        SessionID string          `json:"sessionId,omitempty"`
        Seq       int32           `json:"seq,omitempty"`
        Plat      string          `json:"plat,omitempty"`
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
                // not an envelope
                return nil, false
        }
        return &e, true
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

// abortEncrypted writes an error envelope directly.
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

func (c *captureWriter) Header() http.Header      { return c.header }
func (c *captureWriter) WriteHeader(statusCode int) {}
func (c *captureWriter) Write(p []byte) (int, error) { return c.buf.Write(p) }

// Errors that handlers may return to indicate auth failures.
var (
        ErrLoginRequired = errors.New("login required")
        ErrGuestReauth   = errors.New("guest reauth")
)
