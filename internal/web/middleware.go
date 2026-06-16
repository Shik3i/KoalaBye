package web

import "net/http"

func FlashMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flashType, flashMessage, ok := GetFlashFromCookie(r)
		if ok {
			ctx := ContextWithFlash(r.Context(), Flash{Type: flashType, Message: flashMessage})
			ClearFlashCookie(w)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
