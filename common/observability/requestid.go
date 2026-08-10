package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/zeromicro/go-zero/core/logx"
)

// ctxKey is an unexported context-key type so that other packages cannot
// collide with or overwrite the request ID stored here.
type ctxKey struct{}

// RequestIDHeader is the HTTP header name used to propagate the request ID
// between client and server (and across services).
const RequestIDHeader = "X-Request-ID"

// NewRequestID generates a 16-character hex string from 8 random bytes.
// Example: a3f5b2c1d4e6f809
//
// crypto/rand.Read is expected never to fail; on the off chance it does we
// log the error and return a fixed placeholder so callers always receive a
// non-empty value (the only invariant they depend on).
func NewRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		logx.Errorf("observability: crypto/rand.Read failed: %v", err)
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

// WithRequestID stores the given request ID in the context. The returned
// context is safe to pass to downstream loaders, loggers and metrics.
func WithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestIDFromContext returns the request ID stored in the context, or the
// empty string if none is present (including when ctx is nil).
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// RequestIDFromRequest returns the request ID associated with an *http.Request.
// It first looks in the request context (set by RequestIDMiddleware) and falls
// back to the X-Request-ID header. Returns "" when neither is present.
func RequestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if id := RequestIDFromContext(r.Context()); id != "" {
		return id
	}
	return r.Header.Get(RequestIDHeader)
}
