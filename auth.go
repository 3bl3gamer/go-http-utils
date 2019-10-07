package httputils

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/ansel1/merry"
	"github.com/julienschmidt/httprouter"
)

var ErrSessidNotFound = merry.New("sessid not found")

type SessionStore interface {
	FindUserIDBySessid(context.Context, string) (int64, error)
	UpdateSessionData(context.Context, http.ResponseWriter, string, int64) error
}

func RandHexString(n int) string {
	buf := make([]byte, n/2)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func WrapSession(sessStore SessionStore, handle HandlerExt) HandlerExt {
	return func(wr http.ResponseWriter, r *http.Request, ps httprouter.Params) error {
		ctx := r.Context().Value(CtxKeyMain).(*MainCtx)

		// куки
		cookie, err := r.Cookie("sessid")
		if err == nil {
			sessid := cookie.Value
			userID, err := sessStore.FindUserIDBySessid(r.Context(), sessid)
			if merry.Is(err, ErrSessidNotFound) {
				ctx.Sess.ID = "" //неправильный/просроченный sessid
			} else if err != nil {
				return merry.Wrap(err)
			} else {
				ctx.Sess.ID = sessid
				ctx.Sess.UserID = userID
			}
		}
		if ctx.Sess.ID == "" {
			ctx.Sess.ID = RandHexString(32)
		}

		err = sessStore.UpdateSessionData(r.Context(), wr, ctx.Sess.ID, ctx.Sess.UserID)
		if err != nil {
			return merry.Wrap(err)
		}

		return merry.Wrap(handle(wr, r, ps))
	}
}
