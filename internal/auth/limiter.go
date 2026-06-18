package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	count int
	reset time.Time
}

const loginAttemptWindow = 5 * time.Minute

type LoginLimiter struct {
	mu        sync.Mutex
	attempts  map[string]loginAttempt
	lastSweep time.Time
}

func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{attempts: make(map[string]loginAttempt)}
}

func (l *LoginLimiter) Allow(username, ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.sweep(now)

	userIPKey := username + ":" + ip
	userIPAttempt := l.attempts[userIPKey]
	if now.After(userIPAttempt.reset) {
		userIPAttempt = loginAttempt{reset: now.Add(loginAttemptWindow)}
	}
	userIPAttempt.count++
	l.attempts[userIPKey] = userIPAttempt

	ipKey := "ip:" + ip
	ipAttempt := l.attempts[ipKey]
	if now.After(ipAttempt.reset) {
		ipAttempt = loginAttempt{reset: now.Add(loginAttemptWindow)}
	}
	ipAttempt.count++
	l.attempts[ipKey] = ipAttempt

	return userIPAttempt.count <= 5 && ipAttempt.count <= 50
}

// sweep removes expired attempt windows so the map cannot grow without bound
// from failed logins or rotated IPs. It runs at most once per window and
// assumes the caller holds the mutex.
func (l *LoginLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < loginAttemptWindow {
		return
	}
	l.lastSweep = now
	for key, attempt := range l.attempts {
		if now.After(attempt.reset) {
			delete(l.attempts, key)
		}
	}
}

func (l *LoginLimiter) Success(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username+":"+ip)
}
