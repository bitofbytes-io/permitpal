package auth

import (
	"fmt"
	"testing"
	"time"
)

func newTestLimiter(now *time.Time) *LoginLimiter {
	limiter := NewLoginLimiter(5, 15*time.Minute)
	limiter.now = func() time.Time { return *now }
	return limiter
}

func TestLoginLimiterBlocksUsernameAfterFailures(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for i := range 5 {
		if _, ok := limiter.Allow(fmt.Sprintf("192.0.2.%d", i+1), "aiden"); !ok {
			t.Fatalf("attempt %d blocked", i+1)
		}
	}
	now = now.Add(time.Minute)
	wait, ok := limiter.Allow("198.51.100.9", "aiden")
	if ok || wait != 14*time.Minute {
		t.Fatalf("Allow = %v, %v; want blocked for 14m", wait, ok)
	}
	if _, ok := limiter.Allow("198.51.100.9", "caleb"); !ok {
		t.Fatal("other username from a fresh IP was blocked")
	}
	now = now.Add(14 * time.Minute)
	if _, ok := limiter.Allow("198.51.100.9", "aiden"); !ok {
		t.Fatal("username still blocked after the window expired")
	}
}

func TestLoginLimiterBlocksIPAcrossUsernames(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for _, username := range []string{"a", "b", "c", "d", "e"} {
		if _, ok := limiter.Allow("192.0.2.1", username); !ok {
			t.Fatalf("attempt for %s blocked", username)
		}
	}
	if _, ok := limiter.Allow("192.0.2.1", "f"); ok {
		t.Fatal("IP was not blocked after five failures")
	}
	if _, ok := limiter.Allow("192.0.2.2", "f"); !ok {
		t.Fatal("different IP was blocked")
	}
}

func TestLoginLimiterSuccessClearsUsernameFailures(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for range 4 {
		limiter.Allow("192.0.2.1", "aiden")
	}
	limiter.Allow("192.0.2.2", "aiden")
	limiter.Succeed("192.0.2.2", "aiden")
	if _, ok := limiter.failures["user:aiden"]; ok {
		t.Fatal("success did not clear username failures")
	}
	if _, ok := limiter.failures["ip:192.0.2.2"]; ok {
		t.Fatal("success counted against the client IP")
	}
	if f := limiter.failures["ip:192.0.2.1"]; f.count != 4 {
		t.Fatalf("other IP failures = %d, want 4", f.count)
	}
	for i := range 5 {
		if _, ok := limiter.Allow("192.0.2.3", "aiden"); !ok {
			t.Fatalf("attempt %d after success blocked", i+1)
		}
	}
}

func TestLoginLimiterSweepsExpiredEntries(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	limiter.Allow("192.0.2.1", "aiden")
	now = now.Add(16 * time.Minute)
	limiter.Allow("192.0.2.2", "caleb")
	if len(limiter.failures) != 2 {
		t.Fatalf("failures = %v, want only the latest attempt", limiter.failures)
	}
}

func TestLoginLimiterSkipsUnknownIP(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for i := range 10 {
		if _, ok := limiter.Allow("", fmt.Sprintf("user%d", i)); !ok {
			t.Fatalf("attempt %d with unknown IP blocked", i+1)
		}
	}
	if _, ok := limiter.failures["ip:"]; ok {
		t.Fatal("unknown IP was tracked")
	}
	limiter.Succeed("", "user0")
	if _, ok := limiter.failures["user:user0"]; ok {
		t.Fatal("success did not clear username failures")
	}
}

func TestLoginLimiterSkipsEmptyUsername(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	for i := range 5 {
		if _, ok := limiter.Allow("192.0.2.1", ""); !ok {
			t.Fatalf("attempt %d blocked", i+1)
		}
	}
	if len(limiter.failures) != 1 {
		t.Fatalf("failures = %v, want only the IP key", limiter.failures)
	}
	if _, ok := limiter.Allow("192.0.2.1", ""); ok {
		t.Fatal("IP limit did not apply without a username")
	}
}

func TestLoginLimiterCapsEntries(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	limiter := newTestLimiter(&now)
	limiter.maxEntries = 3
	for i := range 10 {
		now = now.Add(time.Second)
		limiter.Allow("", fmt.Sprintf("user%d", i))
		if len(limiter.failures) > 3 {
			t.Fatalf("entries = %d, want at most 3", len(limiter.failures))
		}
	}
	for _, key := range []string{"user:user7", "user:user8", "user:user9"} {
		if _, ok := limiter.failures[key]; !ok {
			t.Fatalf("newest entry %s was evicted: %v", key, limiter.failures)
		}
	}
}
