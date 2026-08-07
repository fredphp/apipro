package admin

import (
	"net/http"

	"apipro/cmd/api/internal/logic/admin"
	"apipro/cmd/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func StatsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := admin.NewStatsLogic(r.Context(), svcCtx)
		resp, err := l.Stats()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
