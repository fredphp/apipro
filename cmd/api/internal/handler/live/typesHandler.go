package live

import (
	"net/http"

	"apipro/cmd/api/internal/logic/live"
	"apipro/cmd/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func TypesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := live.NewTypesLogic(r.Context(), svcCtx)
		resp, err := l.Types()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
