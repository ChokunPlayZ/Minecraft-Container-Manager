package api

import (
	"context"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccountLoginLimitSurvivesIPRotation(t *testing.T) {
	handle, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	users := auth.NewUsers(handle.DB)
	if _, err = users.Create(context.Background(), "alice@example.test", "bad-hash"); err != nil {
		t.Fatal(err)
	}
	s := &Server{users: users, loginLimit: newLoginLimiter(2, time.Minute, time.Minute)}
	for i, ip := range []string{"192.0.2.1:1", "192.0.2.2:1", "192.0.2.3:1"} {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"  ALICE@example.test ","password":"wrong"}`))
		request.RemoteAddr = ip
		response := httptest.NewRecorder()
		s.handleLogin(response, request)
		want := http.StatusUnauthorized
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("rotated-IP login %d: %d %s", i, response.Code, response.Body.String())
		}
	}
}
func TestForwardedIPRequiresTrustedProxy(t *testing.T) {
	s := &Server{cfg: &config.Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.3:8080"
	request.Header.Set("X-Forwarded-For", "198.51.100.1")
	if got := s.clientIP(request); got != "192.0.2.3" {
		t.Fatal("untrusted forwarding accepted", got)
	}
	request.RemoteAddr = "10.0.0.1:8080"
	request.Header.Set("X-Forwarded-For", "203.0.113.5, 198.51.100.1, 10.0.0.2")
	if got := s.clientIP(request); got != "198.51.100.1" {
		t.Fatal("incorrect trusted chain", got)
	}
}
