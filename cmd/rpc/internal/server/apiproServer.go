package server

import (
	"context"

	"apipro/cmd/rpc/internal/logic"
	"apipro/cmd/rpc/internal/svc"
	"apipro/desc/proto/gen/apipro"
)

type ApiproServer struct {
	svcCtx *svc.ServiceContext
	apipro.UnimplementedApiproServer
}

func NewApiproServer(svcCtx *svc.ServiceContext) *ApiproServer {
	return &ApiproServer{svcCtx: svcCtx}
}

// Call dispatches a single business method by name. The gateway encrypts/
// decrypts the wire bytes — RPC sees plain JSON.
func (s *ApiproServer) Call(ctx context.Context, in *apipro.CallReq) (*apipro.CallResp, error) {
	l := logic.NewCallLogic(ctx, s.svcCtx)
	return l.Call(in)
}
