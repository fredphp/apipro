package match

import (
	"net/http"

	"apipro/cmd/api/internal/logic/match"
	"apipro/cmd/api/internal/svc"
	"apipro/cmd/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListByDateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DateReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := match.NewListByDateLogic(r.Context(), svcCtx)
		resp, err := l.ListByDate(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
