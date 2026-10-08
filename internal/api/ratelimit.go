package api

import (
	"sync"
	"time"
)

// Login rate limiting: after too many failed attempts for a given key (email),
// further attempts are locked out for a cooling-off window. This counters online
// password guessing without any third-party dependency. In-memory and per
// process — fine for a small self-hosted deployment.
const (
	maxLoginFails = 5                // failures before lockout kicks in
	loginLockout  = 5 * time.Minute  // how long a locked key stays blocked
	failWindow    = 15 * time.Minute // failures older than this are forgotten
)

type attemptInfo struct {
	fails       int
	lockedUntil time.Time
	last        time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptInfo
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: map[string]*attemptInfo{}}
}

// allowed reports whether an attempt for key may proceed. When locked, it also
// returns how long the caller should wait.
func (l *loginLimiter) allowed(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[key]
	if a == nil {
		return true, 0
	}
	now := time.Now()
	if now.Before(a.lockedUntil) {
		return false, time.Until(a.lockedUntil)
	}
	// Drop stale failure history so counts don't accumulate forever.
	if now.Sub(a.last) > failWindow {
		delete(l.attempts, key)
	}
	return true, 0
}

// fail records a failed attempt and locks the key once the threshold is hit.
func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	a := l.attempts[key]
	if a == nil || now.Sub(a.last) > failWindow {
		a = &attemptInfo{}
		l.attempts[key] = a
	}
	a.fails++
	a.last = now
	if a.fails >= maxLoginFails {
		a.lockedUntil = now.Add(loginLockout)
		a.fails = 0 // restart the count after the lockout window
	}
}

// reset clears the record for a key after a successful login.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// sweep drops entries that are no longer locked and whose last failure has aged
// out — so a flood of attempts with distinct keys can't grow the map unbounded.
func (l *loginLimiter) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, a := range l.attempts {
		if now.After(a.lockedUntil) && now.Sub(a.last) > failWindow {
			delete(l.attempts, k)
		}
	}
}

// startSweeper periodically prunes stale entries.
func (l *loginLimiter) startSweeper() {
	go func() {
		t := time.NewTicker(failWindow)
		defer t.Stop()
		for range t.C {
			l.sweep()
		}
	}()
}

// loginRL is the shared login limiter for this process.
var loginRL = newLoginLimiter()

// StartRateLimiterSweeper starts the background prune of the login limiter.
func (s *Server) StartRateLimiterSweeper() {
	loginRL.startSweeper()
}
