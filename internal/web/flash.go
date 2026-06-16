package web

import (
	"context"
	"net/http"
	"strings"
)

const flashCookieName = "koalabye_flash"

type Flash struct {
	Type    string
	Message string
}

type flashContextKey struct{}

func ContextWithFlash(ctx context.Context, f Flash) context.Context {
	return context.WithValue(ctx, flashContextKey{}, f)
}

func FlashFromContext(ctx context.Context) (Flash, bool) {
	f, ok := ctx.Value(flashContextKey{}).(Flash)
	return f, ok
}

func SetFlashCookie(w http.ResponseWriter, flashType, message string) {
	http.SetCookie(w, &http.Cookie{
		Name: flashCookieName, Value: flashType + ":" + message,
		Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 10,
	})
}

func ClearFlashCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: flashCookieName, Value: "", Path: "/", MaxAge: -1,
	})
}

func GetFlashFromCookie(r *http.Request) (string, string, bool) {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(cookie.Value, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
