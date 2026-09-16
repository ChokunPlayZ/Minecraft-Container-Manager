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

func TestHandleServerStats(t *testing.T) {
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
		Name:       "Stats Test Server",
		ServerType: jars.TypePaper,
		Version:    "1.21.4",
		RAMMB:      4096,
		CPULimit:   2.0,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// 1. Success query for server stats
	req := httptest.NewRequest(http.MethodGet, "/api/servers/"+srv.ID+"/stats", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	apiServer.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var stats servers.ServerStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if stats.ServerID != srv.ID {
		t.Errorf("expected ServerID %s, got %s", srv.ID, stats.ServerID)
	}
	if stats.Online {
		t.Errorf("expected Online=false for stopped server")
	}
	if stats.CPULimit != 2.0 {
		t.Errorf("expected CPULimit 2.0, got %f", stats.CPULimit)
	}
	if stats.MemoryLimitBytes != 4096*1024*1024 {
		t.Errorf("expected MemoryLimitBytes 4GB, got %d", stats.MemoryLimitBytes)
	}

	// 2. Query stats for non-existent server
	req404 := httptest.NewRequest(http.MethodGet, "/api/servers/nonexistent-id/stats", nil)
	req404.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w404 := httptest.NewRecorder()
	apiServer.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 NotFound, got %d", w404.Code)
	}
}
