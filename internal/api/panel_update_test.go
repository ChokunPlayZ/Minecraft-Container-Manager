package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
)

func TestPanelUpdateAuthenticationAndCSRF(t *testing.T) {
	h, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	users := auth.NewUsers(h.DB)
	user, err := users.Create(context.Background(), "panel@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(h.DB)
	token, err := sessions.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: h, Users: users, Sessions: sessions})
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/panel/update"}, {"POST", "/api/panel/update/check"}, {"POST", "/api/panel/update"}} {
		for _, authenticated := range []bool{false, true} {
			req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader(`{"commit":"abc"}`))
			req.Header.Set("Content-Type", "application/json")
			if authenticated {
				addAuthAndCSRF(t, req, token)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if !authenticated && rr.Code < 400 {
				t.Fatalf("accepted unauthenticated %s", endpoint.path)
			}
			if authenticated && endpoint.method == "GET" && (rr.Code != 200 || !strings.Contains(rr.Body.String(), `"commit":`) || rr.Header().Get("Cache-Control") != "no-store") {
				t.Fatalf("status: %d %s", rr.Code, rr.Body.String())
			}
			if authenticated && endpoint.method == "POST" && rr.Code != http.StatusConflict {
				t.Fatalf("unsupported updater: %d %s", rr.Code, rr.Body.String())
			}
		}
	}
	req := httptest.NewRequest("POST", "/api/panel/update", strings.NewReader(`{"commit":"abc"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatal("accepted update without CSRF")
	}
}
