package ctxdata

// Extract JWT claim values from the go-zero request context.
// go-zero's authhandler stores each map claim key as a context value keyed by the claim name string.

import (
	"context"
	"errors"
)

// UidFromCtx returns the uid held in the JWT claims.
func UidFromCtx(ctx context.Context) (string, error) {
	v := ctx.Value("uid")
	if v == nil {
		return "", errors.New("no uid in context")
	}
	s, ok := v.(string)
	if !ok {
		return "", errors.New("uid not string")
	}
	return s, nil
}
