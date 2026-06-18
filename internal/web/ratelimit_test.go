package web

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	l := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("hit %d should be allowed", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("fourth hit should be blocked")
	}
	if !l.Allow("b") {
		t.Fatal("a different key should have its own window")
	}
}

func TestRateLimiterSweepEvictsExpiredEntries(t *testing.T) {
	l := NewRateLimiter(1, time.Minute)
	l.Allow("old")

	// Force the stored window to be expired and the sweep to be due.
	l.mu.Lock()
	l.hits["old"] = rateWindow{count: 1, reset: time.Now().Add(-time.Hour)}
	l.lastSweep = time.Now().Add(-time.Hour)
	l.mu.Unlock()

	l.Allow("new")

	l.mu.Lock()
	_, present := l.hits["old"]
	l.mu.Unlock()
	if present {
		t.Fatal("expired entry should have been swept")
	}
}
