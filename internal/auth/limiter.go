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

func (l *LoginLimiter) Allow(username, ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	userIPKey := username + ":" + ip
	userIPAttempt := l.attempts[userIPKey]
	if now.After(userIPAttempt.reset) {
		userIPAttempt = loginAttempt{reset: now.Add(5 * time.Minute)}
	}
	userIPAttempt.count++
	l.attempts[userIPKey] = userIPAttempt

	ipKey := "ip:" + ip
	ipAttempt := l.attempts[ipKey]
	if now.After(ipAttempt.reset) {
		ipAttempt = loginAttempt{reset: now.Add(5 * time.Minute)}
	}
	ipAttempt.count++
	l.attempts[ipKey] = ipAttempt

	return userIPAttempt.count <= 5 && ipAttempt.count <= 50
}

func (l *LoginLimiter) Success(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, username+":"+ip)
}
