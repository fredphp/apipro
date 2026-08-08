// apiproClient wraps the gRPC ApiproClient for use by the HTTP gateway.
//
// The RPC contract is a single generic `Call` method; the gateway dispatches
// by `method` name. This thin wrapper makes that a bit more ergonomic.
package apiproClient

import (
	"context"

	"apipro/desc/proto/gen/apipro"

	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type (
	CallReq  = apipro.CallReq
	CallResp = apipro.CallResp

	Apipro interface {
		Call(ctx context.Context, in *CallReq, opts ...grpc.CallOption) (*CallResp, error)
	}

	defaultApipro struct {
		cli zrpc.Client
	}
)

func NewApipro(cli zrpc.Client) Apipro {
	return &defaultApipro{cli: cli}
}

func (m *defaultApipro) Call(ctx context.Context, in *CallReq, opts ...grpc.CallOption) (*CallResp, error) {
	client := apipro.NewApiproClient(m.cli.Conn())
	return client.Call(ctx, in, opts...)
}
