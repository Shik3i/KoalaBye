package web

import (
	"sync"
	"time"
)

// RateLimiter is a small fixed-window, in-memory limiter for abuse control on
// unauthenticated endpoints. Keys (typically client IPs) live only in memory
// for at most one window and are never persisted, matching the privacy model.
type RateLimiter struct {
	mu        sync.Mutex
	hits      map[string]rateWindow
	limit     int
	window    time.Duration
	lastSweep time.Time
}

type rateWindow struct {
	count int
	reset time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{hits: make(map[string]rateWindow), limit: limit, window: window}
}

// Allow records one hit for key and reports whether it is within the limit.
func (l *RateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.sweep(now)

	entry := l.hits[key]
	if now.After(entry.reset) {
		entry = rateWindow{reset: now.Add(l.window)}
	}
	entry.count++
	l.hits[key] = entry
	return entry.count <= l.limit
}

// sweep evicts expired windows so the map cannot grow without bound. It runs at
// most once per window and assumes the caller holds the mutex.
func (l *RateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for key, entry := range l.hits {
		if now.After(entry.reset) {
			delete(l.hits, key)
		}
	}
}
