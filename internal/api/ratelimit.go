package api

import (
	"sync"
	"time"
)

// loginLimiter enforces a maximum number of failed login attempts per key
// (IP and/or email) within a window, then locks the key out for a duration.
// It is safe for concurrent use.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	lockouts map[string]time.Time
	max      int
	window   time.Duration
	lockout  time.Duration
}

func newLoginLimiter(max int, window, lockout time.Duration) *loginLimiter {
	return &loginLimiter{
		attempts: make(map[string][]time.Time),
		lockouts: make(map[string]time.Time),
		max:      max,
		window:   window,
		lockout:  lockout,
	}
}

// allow reports whether the key may attempt a login and how long to wait
// before trying again. If the key is currently locked out, allow returns
// false with the remaining lockout duration.
func (l *loginLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if until, ok := l.lockouts[key]; ok {
		if now.Before(until) {
			return false, until.Sub(now)
		}
		delete(l.lockouts, key)
	}
	l.prune(key, now)
	if len(l.attempts[key]) >= l.max {
		l.lockouts[key] = now.Add(l.lockout)
		return false, l.lockout
	}
	return true, 0
}

// record adds a login attempt for a key. failed indicates whether the attempt
// was unsuccessful and should count against the limit.
func (l *loginLimiter) record(key string, failed bool, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !failed {
		// A successful login resets the attempt count for this key.
		delete(l.attempts, key)
		delete(l.lockouts, key)
		return
	}
	l.prune(key, now)
	if len(l.attempts[key]) < l.max {
		l.attempts[key] = append(l.attempts[key], now)
	}
}

// prune drops attempts outside the sliding window for the given key.
func (l *loginLimiter) prune(key string, now time.Time) {
	cutoff := now.Add(-l.window)
	kept := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.attempts[key] = kept
}
