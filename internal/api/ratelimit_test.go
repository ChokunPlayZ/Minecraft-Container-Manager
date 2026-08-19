package api

import (
	"net/http"
	"testing"
	"time"
)

func TestLoginLimiterLockout(t *testing.T) {
	l := newLoginLimiter(3, time.Minute, 5*time.Minute)
	now := time.Now()
	key := "192.168.1.1"

	// First two attempts are allowed.
	for i := 0; i < 2; i++ {
		ok, retry := l.allow(key, now)
		if !ok || retry != 0 {
			t.Fatalf("attempt %d: expected allowed with no retry, got ok=%v retry=%v", i, ok, retry)
		}
		// Record both as failures (one more would trip the limit).
		l.record(key, true, now)
	}

	// Third failed attempt fills the window.
	if ok, _ := l.allow(key, now); !ok {
		t.Fatal("third attempt should be allowed")
	}
	l.record(key, true, now)

	// Fourth attempt is now locked out.
	if ok, retry := l.allow(key, now.Add(time.Second)); ok || retry <= 0 {
		t.Fatalf("expected lockout, got ok=%v retry=%v", ok, retry)
	}

	// A successful login must clear the lockout.
	l.record(key, false, now.Add(10*time.Second))
	if ok, _ := l.allow(key, now.Add(11*time.Second)); !ok {
		t.Fatal("successful login should reset the lockout")
	}
}

func TestLoginLimiterWindowExpiry(t *testing.T) {
	l := newLoginLimiter(2, time.Minute, 5*time.Minute)
	now := time.Now()
	key := "10.0.0.5"

	l.record(key, true, now)
	l.record(key, true, now.Add(10*time.Second))

	// Beyond the window, old attempts should be pruned and a fresh attempt allowed.
	later := now.Add(2 * time.Minute)
	if ok, _ := l.allow(key, later); !ok {
		t.Fatal("attempts outside the window should be pruned")
	}
}

func TestIPLimiter(t *testing.T) {
	l := newIPLimiter(2, time.Minute)
	now := time.Now()
	key := "203.0.113.9"

	if !l.allow(key, now) {
		t.Fatal("first request should be allowed")
	}
	if !l.allow(key, now.Add(time.Second)) {
		t.Fatal("second request should be allowed")
	}
	if l.allow(key, now.Add(2*time.Second)) {
		t.Fatal("third request should be refused")
	}

	// A different client is unaffected.
	if !l.allow("other", now.Add(3*time.Second)) {
		t.Fatal("other client should be allowed")
	}
}

func TestClientIP(t *testing.T) {
	s := &Server{}
	r := &http.Request{RemoteAddr: "192.168.1.7:44567"}
	if got := s.clientIP(r); got != "192.168.1.7" {
		t.Fatalf("clientIP = %q, want 192.168.1.7", got)
	}
}
