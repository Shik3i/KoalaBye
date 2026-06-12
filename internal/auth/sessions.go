package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/koalastuff/koalabye/internal/db"
)

const SessionCookieName = "koalabye_session"

type SessionManager struct {
	queries       *db.Querier
	secureCookies bool
	lifetime      time.Duration
}

func NewSessionManager(queries *db.Querier, secureCookies bool) *SessionManager {
	return &SessionManager{queries: queries, secureCookies: secureCookies, lifetime: 30 * 24 * time.Hour}
}

func (s *SessionManager) Start(ctx context.Context, w http.ResponseWriter, userID int64) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.queries.CreateSession(ctx, userID, HashSessionToken(token), now.Format(time.RFC3339Nano), now.Add(s.lifetime).Format(time.RFC3339Nano)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: s.secureCookies, SameSite: http.SameSiteLaxMode,
		MaxAge: int(s.lifetime.Seconds()), Expires: now.Add(s.lifetime),
	})
	return nil
}

func (s *SessionManager) CurrentUser(ctx context.Context, r *http.Request) (db.User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return db.User{}, err
	}
	user, err := s.queries.GetActiveSessionUser(ctx, HashSessionToken(cookie.Value), db.Now())
	if err == nil {
		_ = s.queries.TouchSession(ctx, HashSessionToken(cookie.Value), db.Now())
	}
	return user, err
}

func (s *SessionManager) End(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if err := s.queries.RevokeSession(ctx, HashSessionToken(cookie.Value), db.Now()); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: s.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	return nil
}

func HashSessionToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
