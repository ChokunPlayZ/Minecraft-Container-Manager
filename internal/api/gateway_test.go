package api

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

func newGatewayTestServer(t *testing.T) *Server {
	t.Helper()
	handle, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	dm, err := docker.New("unix:///nonexistent-docker.sock")
	if err != nil {
		t.Fatalf("docker client: %v", err)
	}
	store := servers.NewStore(handle, dm, jars.NewResolver(), 25565, 25665, t.TempDir())
	if _, err := handle.DB.Exec(`INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, state, created_at, updated_at) VALUES ('srv1', 'Test', 'paper', '1.21.1', 2048, 29998, 'stopped', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	return &Server{
		cfg:     &config.Config{Gateway: "auto"},
		servers: store,
		logger:  log.New(io.Discard, "", 0),
	}
}

func TestGetGateway(t *testing.T) {
	s := newGatewayTestServer(t)
	r := httptest.NewRequest(http.MethodGet, "/api/servers/srv1/gateway", nil)
	r.SetPathValue("id", "srv1")
	w := httptest.NewRecorder()
	s.handleGetGateway(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"enabled":false`) {
		t.Errorf("body %s missing enabled:false", body)
	}
	if !strings.Contains(body, `"last_motd":""`) && !strings.Contains(body, `"last_motd": ""`) {
		t.Errorf("body %s missing last_motd", body)
	}
}

func TestPutGatewayWakeMessage(t *testing.T) {
	s := newGatewayTestServer(t)
	body := strings.NewReader(`{"wake_message":"Starting up now!"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/servers/srv1/gateway", body)
	r.SetPathValue("id", "srv1")
	w := httptest.NewRecorder()
	s.handlePutGateway(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Starting up now!") {
		t.Errorf("response %s missing wake message", w.Body.String())
	}
	// Verify persistence through the store.
	msg, err := s.servers.WakeMessage(r.Context(), "srv1")
	if err != nil || msg != "Starting up now!" {
		t.Errorf("persisted wake message = %q (err %v)", msg, err)
	}
}
