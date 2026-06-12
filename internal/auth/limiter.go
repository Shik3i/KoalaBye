package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	count int
	reset time.Time
}

type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: make(map[string]loginAttempt)}
}

func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	attempt := l.attempts[key]
	if now.After(attempt.reset) {
		attempt = loginAttempt{reset: now.Add(5 * time.Minute)}
	}
	attempt.count++
	l.attempts[key] = attempt
	return attempt.count <= 8
}

func (l *LoginLimiter) Success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
