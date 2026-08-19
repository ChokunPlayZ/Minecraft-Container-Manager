package servers

import (
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	handle, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	dm, err := docker.New("unix:///nonexistent-docker.sock")
	if err != nil {
		t.Skipf("docker client unavailable: %v", err)
	}
	return NewStore(handle, dm, jars.NewResolver(), 25565, 25665, t.TempDir())
}

func TestGatewayAccessors(t *testing.T) {
	s := newTestStore(t)

	// Insert a server row directly so we can exercise the accessors.
	if _, err := s.db.Exec(`INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, state, created_at, updated_at) VALUES ('srv1', 'Test', 'paper', '1.21.1', 2048, 29999, 'stopped', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert server: %v", err)
	}

	// Wake message default empty.
	msg, err := s.WakeMessage(t.Context(), "srv1")
	if err != nil {
		t.Fatalf("WakeMessage: %v", err)
	}
	if msg != "" {
		t.Errorf("WakeMessage = %q, want empty", msg)
	}
	if err := s.SetWakeMessage(t.Context(), "srv1", "Starting up, one moment..."); err != nil {
		t.Fatalf("SetWakeMessage: %v", err)
	}
	msg, err = s.WakeMessage(t.Context(), "srv1")
	if err != nil || msg != "Starting up, one moment..." {
		t.Errorf("WakeMessage after set = %q (err %v)", msg, err)
	}
	// Clearing falls back to empty.
	if err := s.SetWakeMessage(t.Context(), "srv1", ""); err != nil {
		t.Fatalf("SetWakeMessage clear: %v", err)
	}
	msg, _ = s.WakeMessage(t.Context(), "srv1")
	if msg != "" {
		t.Errorf("WakeMessage after clear = %q, want empty", msg)
	}

	// Last MOTD.
	motd, updated, err := s.LastMotd(t.Context(), "srv1")
	if err != nil {
		t.Fatalf("LastMotd: %v", err)
	}
	if motd != "" || !updated.IsZero() {
		t.Errorf("LastMotd = %q/%v, want empty/zero", motd, updated)
	}
	if err := s.SetLastMotd(t.Context(), "srv1", "My Cool Server"); err != nil {
		t.Fatalf("SetLastMotd: %v", err)
	}
	motd, updated, err = s.LastMotd(t.Context(), "srv1")
	if err != nil || motd != "My Cool Server" || updated.IsZero() {
		t.Errorf("LastMotd after set = %q/%v (err %v)", motd, updated, err)
	}

	// Gateway info.
	info, err := s.GatewayInfo(t.Context(), "srv1", true)
	if err != nil {
		t.Fatalf("GatewayInfo: %v", err)
	}
	if !info.Enabled || info.LastMotd != "My Cool Server" {
		t.Errorf("GatewayInfo = %+v", info)
	}
}

func TestGatewaySettings(t *testing.T) {
	s := newTestStore(t)
	enabled, err := s.GatewayEnabled(t.Context())
	if err != nil {
		t.Fatalf("GatewayEnabled: %v", err)
	}
	if enabled {
		t.Error("gateway_enabled should default to false")
	}
	def, err := s.WakeMessageDefault(t.Context(), "fallback")
	if err != nil {
		t.Fatalf("WakeMessageDefault: %v", err)
	}
	if def != "Server is waking up, please wait..." {
		t.Errorf("WakeMessageDefault = %q", def)
	}
	if _, err := s.db.Exec(`UPDATE settings SET value='true' WHERE key='gateway_enabled'`); err != nil {
		t.Fatalf("enable gateway: %v", err)
	}
	enabled, err = s.GatewayEnabled(t.Context())
	if err != nil || !enabled {
		t.Errorf("GatewayEnabled after set = %v (err %v)", enabled, err)
	}
}
