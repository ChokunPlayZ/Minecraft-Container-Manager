package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestHandleCopyServer(t *testing.T) {
	tempDir := t.TempDir()
	store, err := db.Open(filepath.Join(tempDir, "mcm_api_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	// Seed user and session
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

	// Create source server
	src, err := serverStore.Create(ctx, servers.CreateInput{
		Name:       "Original Server",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("create source server: %v", err)
	}

	// Create some dummy world files in source
	srcDir := filepath.Join(tempDir, "servers", src.ID)
	if err := os.MkdirAll(filepath.Join(srcDir, "world"), 0o755); err != nil {
		t.Fatalf("mkdir world: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "world", "level.dat"), []byte("dat"), 0o644); err != nil {
		t.Fatalf("write level.dat: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "server.properties"), []byte("motd=Test"), 0o644); err != nil {
		t.Fatalf("write server.properties: %v", err)
	}

	// 1. Success copy
	copyBody, _ := json.Marshal(map[string]any{
		"name":          "Copied Server",
		"include_world": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/servers/"+src.ID+"/copy", bytes.NewReader(copyBody))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrf := mustCSRF(t)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	w := httptest.NewRecorder()
	apiServer.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var copiedServer servers.Server
	if err := json.Unmarshal(w.Body.Bytes(), &copiedServer); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if copiedServer.Name != "Copied Server" {
		t.Errorf("expected name %q, got %q", "Copied Server", copiedServer.Name)
	}
	if copiedServer.HostPort == src.HostPort {
		t.Errorf("expected different port from original, got %d", copiedServer.HostPort)
	}

	// 2. Missing name validation
	badBody, _ := json.Marshal(map[string]any{
		"name": "",
	})
	reqBad := httptest.NewRequest(http.MethodPost, "/api/servers/"+src.ID+"/copy", bytes.NewReader(badBody))
	reqBad.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrfBad := mustCSRF(t)
	reqBad.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfBad})
	reqBad.Header.Set(csrfHeaderName, csrfBad)
	wBad := httptest.NewRecorder()
	apiServer.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty name, got %d", wBad.Code)
	}

	// 3. Not found server
	reqNotFound := httptest.NewRequest(http.MethodPost, "/api/servers/non-existent-id/copy", bytes.NewReader(copyBody))
	reqNotFound.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrfNF := mustCSRF(t)
	reqNotFound.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrfNF})
	reqNotFound.Header.Set(csrfHeaderName, csrfNF)
	wNotFound := httptest.NewRecorder()
	apiServer.ServeHTTP(wNotFound, reqNotFound)
	if wNotFound.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", wNotFound.Code)
	}
}
