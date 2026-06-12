package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"
)

const CSRFCookieName = "koalabye_csrf"

type CSRF struct {
	secret        []byte
	secureCookies bool
}

func NewCSRF(secret string, secureCookies bool) *CSRF {
	return &CSRF{secret: []byte(secret), secureCookies: secureCookies}
}

func (c *CSRF) Token(w http.ResponseWriter, r *http.Request) (string, error) {
	if cookie, err := r.Cookie(CSRFCookieName); err == nil && c.valid(cookie.Value) {
		return cookie.Value, nil
	}
	nonce, err := randomToken(24)
	if err != nil {
		return "", err
	}
	token := nonce + "." + c.sign(nonce)
	http.SetCookie(w, &http.Cookie{
		Name: CSRFCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: c.secureCookies, SameSite: http.SameSiteStrictMode,
		MaxAge: int((24 * time.Hour).Seconds()),
	})
	return token, nil
}

func (c *CSRF) Validate(r *http.Request) error {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return errors.New("missing CSRF cookie")
	}
	formToken := r.PostFormValue("csrf_token")
	if !c.valid(cookie.Value) || !hmac.Equal([]byte(cookie.Value), []byte(formToken)) {
		return errors.New("invalid CSRF token")
	}
	return nil
}

func (c *CSRF) valid(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	return hmac.Equal([]byte(parts[1]), []byte(c.sign(parts[0])))
}

func (c *CSRF) sign(value string) string {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
