package match

import (
	"net/http"

	"apipro/cmd/api/internal/logic/match"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListByCateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.CateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := match.NewListByCateLogic(r.Context(), svcCtx)
		resp, err := l.ListByCate(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
