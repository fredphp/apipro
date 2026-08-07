package live

import (
	"net/http"

	"apipro/cmd/api/internal/logic/live"
	"apipro/cmd/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func HotAnchorHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := live.NewHotAnchorLogic(r.Context(), svcCtx)
		resp, err := l.HotAnchor()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
