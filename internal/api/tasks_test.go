package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestHandleTaskProgress(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "mcm_api_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	users := auth.NewUsers(store.DB)
	user, err := users.Create(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessions := auth.NewManager(store.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	jr := jars.NewResolver()
	serverStore := servers.NewStore(store, nil, jr, 25565, 25575, tempDir, "")

	apiServer := New(Options{
		DB:       store,
		Users:    users,
		Sessions: sessions,
		Servers:  serverStore,
	})

	srv, err := serverStore.Create(ctx, servers.CreateInput{
		Name:       "Task Server",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// 1. Initial check - active: false
	req := httptest.NewRequest(http.MethodGet, "/api/servers/"+srv.ID+"/tasks/progress", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	apiServer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res["active"] != false {
		t.Fatalf("expected active: false, got %v", res["active"])
	}

	// 2. Set progress
	serverStore.SetTaskProgress(srv.ID, servers.TaskProgress{
		Operation:  "modpack_install",
		Stage:      "extracting",
		StageTitle: "Extracting Files",
		Percent:    45,
	})

	req2 := httptest.NewRequest(http.MethodGet, "/api/servers/"+srv.ID+"/tasks/progress", nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w2 := httptest.NewRecorder()
	apiServer.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var res2 struct {
		Active   bool                  `json:"active"`
		Progress *servers.TaskProgress `json:"progress"`
	}
	if err := json.NewDecoder(w2.Body).Decode(&res2); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !res2.Active || res2.Progress == nil || res2.Progress.Percent != 45 {
		t.Fatalf("unexpected progress response: %+v", res2)
	}
}
