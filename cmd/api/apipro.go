package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"apipro/cmd/api/internal/config"
	"apipro/cmd/api/internal/handler"
	"apipro/cmd/api/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/apipro.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf,
		rest.WithNotAllowedHandler(http.HandlerFunc(methodNotAllowed)),
	)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)

	// Global middlewares: CORS + rate limit.
	server.Use(corsMiddleware(c.CorsOrigin))
	server.Use(ctx.RateLimiter.HTTPMiddleware())

	// All routes (JSONP + encrypted + static + WS).
	handler.RegisterHandlers(server, ctx)

	// Start chat hub.
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ctx.ChatHub.Run(rootCtx)

	fmt.Printf("Starting apipro http at %s:%d...\n", c.Host, c.Port)
	server.Start()
}

func corsMiddleware(origin string) func(http.HandlerFunc) http.HandlerFunc {
	if origin == "" {
		origin = "*"
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Transform-Port, XTransformPort, X-Session, X-Plat, X-Seq")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "SAMEORIGIN")
			w.Header().Set("Referrer-Policy", "no-referrer")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	}
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, _ = w.Write([]byte(`{"code":405,"message":"method not allowed"}`))
}

// guard against unused import warnings if strings removed later
var _ = strings.TrimSpace
var _ = time.Now
var _ = os.Getenv
