package main

import (
	"context"
	"flag"
	"fmt"

	"apipro/cmd/rpc/internal/config"
	"apipro/cmd/rpc/internal/server"
	"apipro/cmd/rpc/internal/svc"
	"apipro/desc/proto/gen/apipro"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/apipro.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)

	// Start scheduled cache refresh
	ctx.Scheduler.Start()
	defer ctx.Scheduler.Stop()

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		apipro.RegisterApiproServer(grpcServer, server.NewApiproServer(ctx))
		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	// graceful shutdown context
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = rootCtx

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
