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
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestUpdateCheckerAPIOptInAndAuthentication(t *testing.T) {
	h, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	users := auth.NewUsers(h.DB)
	user, err := users.Create(context.Background(), "updates@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(h.DB)
	token, err := sessions.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: h, Servers: servers.NewStore(h, nil, nil, 25565, 25575, t.TempDir(), ""), Users: users, Sessions: sessions})
	request := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			addAuthAndCSRF(t, req, token)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	for _, item := range []struct{ method, path string }{{"GET", "/api/updates"}, {"PUT", "/api/updates/settings"}, {"POST", "/api/updates/check"}} {
		if rr := request(item.method, item.path, `{"enabled":true}`, false); rr.Code < 400 {
			t.Fatalf("unauthenticated %s accepted", item.path)
		}
	}
	if rr := request("GET", "/api/updates", "", true); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("default: %d %s", rr.Code, rr.Body.String())
	}
	if rr := request("POST", "/api/updates/check", `{}`, true); rr.Code != 400 {
		t.Fatalf("opt-out accepted a scan: %d", rr.Code)
	}
	for _, body := range []string{`{}`, `{"enabled":"true"}`, `{"enabled":null}`} {
		if rr := request("PUT", "/api/updates/settings", body, true); rr.Code != 400 {
			t.Fatalf("invalid setting: %d", rr.Code)
		}
	}
	if rr := request("PUT", "/api/updates/settings", `{"enabled":true}`, true); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Fatalf("enable: %d %s", rr.Code, rr.Body.String())
	}
	if rr := request("PUT", "/api/settings", `{"settings":{"update_checker_enabled":"true"}}`, true); rr.Code != http.StatusBadRequest {
		t.Fatal("generic settings bypassed opt-in endpoint")
	}
	if rr := request("PUT", "/api/updates/settings", `{"enabled":false}`, true); rr.Code != 200 {
		t.Fatalf("disable: %d", rr.Code)
	}
}
