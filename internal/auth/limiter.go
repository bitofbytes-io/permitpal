package auth

import (
	"sync"
	"time"
)

// LoginLimiter throttles failed logins per client IP and per username. Each
// attempt is reserved before the password is checked, so concurrent requests
// cannot all run bcrypt past the limit; a successful login releases its IP
// reservation and clears the username's failures.
type LoginLimiter struct {
	mu        sync.Mutex
	max       int
	window    time.Duration
	now       func() time.Time
	failures  map[string]failureWindow
	lastSweep time.Time
}

type failureWindow struct {
	count int
	start time.Time
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{max: max, window: window, now: time.Now, failures: make(map[string]failureWindow)}
}

// Allow reserves a login attempt for ip and username. An empty ip, used when
// the real client IP is unknown, skips the per-IP limit. When either key has
// reached the failure limit, it returns false and the time until the oldest
// window expires.
func (l *LoginLimiter) Allow(ip, username string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	keys := limiterKeys(ip, username)
	var wait time.Duration
	for _, key := range keys {
		f, ok := l.active(key, now)
		if ok && f.count >= l.max {
			wait = max(wait, f.start.Add(l.window).Sub(now))
		}
	}
	if wait > 0 {
		return wait, false
	}
	for _, key := range keys {
		f, ok := l.active(key, now)
		if !ok {
			f = failureWindow{start: now}
		}
		f.count++
		l.failures[key] = f
	}
	return 0, true
}

// Succeed releases the IP reservation made by Allow and clears the
// username's failures.
func (l *LoginLimiter) Succeed(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, "user:"+username)
	if ip == "" {
		return
	}
	key := "ip:" + ip
	if f, ok := l.active(key, l.now()); ok && f.count > 1 {
		f.count--
		l.failures[key] = f
	} else {
		delete(l.failures, key)
	}
}

func (l *LoginLimiter) active(key string, now time.Time) (failureWindow, bool) {
	f, ok := l.failures[key]
	if ok && !now.Before(f.start.Add(l.window)) {
		delete(l.failures, key)
		return failureWindow{}, false
	}
	return f, ok
}

func (l *LoginLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for key := range l.failures {
		l.active(key, now)
	}
}

func limiterKeys(ip, username string) []string {
	keys := []string{"user:" + username}
	if ip != "" {
		keys = append(keys, "ip:"+ip)
	}
	return keys
}
