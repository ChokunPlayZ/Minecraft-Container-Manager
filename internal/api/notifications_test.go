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

func TestDiscordNotificationAPI(t *testing.T) {
	h, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	users := auth.NewUsers(h.DB)
	user, err := users.Create(context.Background(), "discord@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(h.DB)
	token, err := sessions.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: h, Users: users, Sessions: sessions})
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
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		if rr := request(method, "/api/notifications/discord", `{}`, false); rr.Code == http.StatusOK {
			t.Fatalf("unauthenticated %s accepted", method)
		}
	}
	if rr := request(http.MethodPost, "/api/notifications/discord/test", `{}`, false); rr.Code == http.StatusOK {
		t.Fatal("unauthenticated test accepted")
	}
	rr := request(http.MethodPut, "/api/notifications/discord", `{"enabled":true,"webhook_url":"https://discord.com/api/webhooks/123/private-token","events":["server_crashed"]}`, true)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "private-token") {
		t.Fatalf("save: %d %s", rr.Code, rr.Body.String())
	}
	rr = request(http.MethodGet, "/api/notifications/discord", "", true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"configured":true`) || strings.Contains(rr.Body.String(), "private-token") {
		t.Fatalf("read: %d %s", rr.Code, rr.Body.String())
	}
	rr = request(http.MethodGet, "/api/settings", "", true)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "discord_notifications") || strings.Contains(rr.Body.String(), "private-token") {
		t.Fatalf("generic settings leak: %s", rr.Body.String())
	}
	for _, body := range []string{`{"enabled":true,"webhook_url":"https://localhost/webhook","events":[]}`, `{"enabled":true,"events":["unknown"]}`, `{broken`} {
		if rr := request(http.MethodPut, "/api/notifications/discord", body, true); rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid input: %d %s", rr.Code, rr.Body.String())
		}
	}
	rr = request(http.MethodPut, "/api/settings", `{"settings":{"discord_notifications":"{}"}}`, true)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unvalidated write allowed: %d", rr.Code)
	}
	rr = request(http.MethodPut, "/api/notifications/discord", `{"enabled":false,"webhook_url":"","events":[]}`, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rr.Code, rr.Body.String())
	}
	rr = request(http.MethodPost, "/api/notifications/discord/test", `{}`, true)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("missing webhook: %d", rr.Code)
	}
}
