package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const flashCookieName = "koalabye_flash"

type Flash struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type flashContextKey struct{}

func FlashFromContext(ctx context.Context) (Flash, bool) {
	flash, ok := ctx.Value(flashContextKey{}).(Flash)
	return flash, ok && flash.Key != ""
}

func FlashMiddleware(secret string, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(flashCookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			clearFlashCookie(w, secureCookies)
			flash, ok := decodeFlash(cookie.Value, secret)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), flashContextKey{}, flash)))
		})
	}
}

func SetFlash(w http.ResponseWriter, secret string, secureCookies bool, kind, key string) {
	if kind != "error" {
		kind = "success"
	}
	value, err := encodeFlash(Flash{Kind: kind, Key: key}, secret)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: flashCookieName, Value: value, Path: "/", HttpOnly: true,
		Secure: secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: 60, Expires: time.Now().Add(time.Minute),
	})
}

func encodeFlash(flash Flash, secret string) (string, error) {
	payload, err := json.Marshal(flash)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + flashSignature(encoded, secret), nil
}

func decodeFlash(value, secret string) (Flash, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(flashSignature(parts[0], secret))) {
		return Flash{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Flash{}, false
	}
	var flash Flash
	if json.Unmarshal(payload, &flash) != nil || flash.Key == "" || (flash.Kind != "success" && flash.Kind != "error") {
		return Flash{}, false
	}
	return flash, true
}

func flashSignature(value, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func clearFlashCookie(w http.ResponseWriter, secureCookies bool) {
	http.SetCookie(w, &http.Cookie{
		Name: flashCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}
