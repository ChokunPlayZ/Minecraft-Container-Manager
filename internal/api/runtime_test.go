package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestPodmanStatusEndpoints(t *testing.T) {
	// Exercise constructor negotiation as well as both authenticated API routes.
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("API-Version", "1.40")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v1.40/images/json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected Podman API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(daemon.Close)
	mgr, err := docker.NewWithRuntime("podman", "tcp://"+strings.TrimPrefix(daemon.URL, "http://"), "eclipse-temurin:21-jre-alpine")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	users := auth.NewUsers(store.DB)
	user, err := users.Create(context.Background(), "runtime@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := mustSession(t, store, user.ID)
	handler := New(Options{
		DB: store, Users: users, Sessions: auth.NewManager(store.DB),
		Servers: servers.NewStore(store, mgr, nil, 25565, 25665, t.TempDir(), ""),
	})
	for _, path := range []string{"/api/runtime/status", "/api/docker/status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		unauthorized := httptest.NewRecorder()
		handler.ServeHTTP(unauthorized, req)
		if unauthorized.Code != http.StatusUnauthorized {
			t.Fatalf("%s without session = %d", path, unauthorized.Code)
		}
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", path, rr.Code, rr.Body.String())
		}
		var status docker.RuntimeStatus
		if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
			t.Fatal(err)
		}
		if status.Runtime != "podman" || !status.Reachable || status.ImageReady || status.Image != "docker.io/library/eclipse-temurin:21-jre-alpine" || !strings.Contains(status.Error, "podman pull") {
			t.Errorf("%s returned incorrect runtime diagnostics: %+v", path, status)
		}
	}
}
