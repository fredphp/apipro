package auth

import (
	"net/http"

	"apipro/cmd/api/internal/logic/auth"
	"apipro/cmd/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GuestHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := auth.NewGuestLogic(r.Context(), svcCtx)
		resp, err := l.Guest()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
