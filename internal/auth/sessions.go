package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/koalastuff/koalabye/internal/db"
)

const SessionCookieName = "koalabye_session"

// touchInterval bounds how often an active session's last_seen_at is written.
// Without it, every authenticated request issues a write, contending for
// SQLite's single writer and amplifying WAL churn on every page load.
const touchInterval = 10 * time.Minute

type SessionManager struct {
	queries       *db.Querier
	secureCookies bool
	lifetime      time.Duration

	touchMu        sync.Mutex
	lastTouched    map[string]time.Time
	lastTouchSweep time.Time
}

func NewSessionManager(queries *db.Querier, secureCookies bool) *SessionManager {
	return &SessionManager{
		queries:       queries,
		secureCookies: secureCookies,
		lifetime:      30 * 24 * time.Hour,
		lastTouched:   make(map[string]time.Time),
	}
}

// shouldTouch reports whether the session identified by hash is due for a
// last_seen_at write, recording the decision so subsequent requests within the
// interval skip the database entirely.
func (s *SessionManager) shouldTouch(hash string, now time.Time) bool {
	s.touchMu.Lock()
	defer s.touchMu.Unlock()
	if now.Sub(s.lastTouchSweep) >= touchInterval {
		s.lastTouchSweep = now
		for key, seen := range s.lastTouched {
			if now.Sub(seen) >= touchInterval {
				delete(s.lastTouched, key)
			}
		}
	}
	if last, ok := s.lastTouched[hash]; ok && now.Sub(last) < touchInterval {
		return false
	}
	s.lastTouched[hash] = now
	return true
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
	hash := HashSessionToken(cookie.Value)
	user, err := s.queries.GetActiveSessionUser(ctx, hash, db.Now())
	if err == nil && s.shouldTouch(hash, time.Now()) {
		_ = s.queries.TouchSession(ctx, hash, db.Now())
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
