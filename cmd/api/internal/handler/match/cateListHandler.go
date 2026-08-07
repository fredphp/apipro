package match

import (
	"net/http"

	"apipro/cmd/api/internal/logic/match"
	"apipro/cmd/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func CateListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := match.NewCateListLogic(r.Context(), svcCtx)
		resp, err := l.CateList()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
