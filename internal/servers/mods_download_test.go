package servers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
)

func newModTestStore(t *testing.T, serverType string) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	s := &Store{db: dbHandle.DB, dataDir: dir}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test-mod-server", serverType, "1.21.4", "", 1024, 0, 0, 25565, "[]", "", StateRunning, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	return s, id
}

func TestDownloadModSuccess(t *testing.T) {
	jarContent := []byte("PK\x03\x04dummy-jar-content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jarContent)
	}))
	defer ts.Close()

	s, serverID := newModTestStore(t, "fabric")
	ctx := context.Background()

	mod, err := s.DownloadMod(ctx, serverID, "Fabric-API-0.119.4.jar", ts.URL+"/fabric-api.jar")
	if err != nil {
		t.Fatalf("DownloadMod returned error: %v", err)
	}

	if mod.Name != "Fabric-API-0.119.4" {
		t.Errorf("got name %q, want %q", mod.Name, "Fabric-API-0.119.4")
	}
	if mod.File != "Fabric-API-0.119.4.jar" {
		t.Errorf("got file %q, want %q", mod.File, "Fabric-API-0.119.4.jar")
	}
	if !mod.Enabled {
		t.Errorf("expected mod to be enabled")
	}

	// Verify file is on disk in mods/
	modPath := filepath.Join(s.dataPath(serverID), "mods", "Fabric-API-0.119.4.jar")
	data, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatalf("read downloaded mod file: %v", err)
	}
	if string(data) != string(jarContent) {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestDownloadModInvalidFilename(t *testing.T) {
	s, serverID := newModTestStore(t, "fabric")
	ctx := context.Background()

	invalid := []string{"../evil.jar", "evil.txt", "sub/mod.jar", "", "bad.exe"}
	for _, fn := range invalid {
		_, err := s.DownloadMod(ctx, serverID, fn, "http://example.com/mod.jar")
		if !errors.Is(err, ErrInvalidModName) {
			t.Errorf("DownloadMod(%q) expected ErrInvalidModName, got %v", fn, err)
		}
	}
}

func TestDownloadModUnsupportedServer(t *testing.T) {
	s, serverID := newModTestStore(t, "vanilla")
	ctx := context.Background()

	_, err := s.DownloadMod(ctx, serverID, "mod.jar", "http://example.com/mod.jar")
	if !errors.Is(err, ErrUnsupportedMods) {
		t.Errorf("DownloadMod on vanilla expected ErrUnsupportedMods, got %v", err)
	}
}

func TestDownloadModHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	s, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	_, err := s.DownloadMod(ctx, serverID, "Plugin.jar", ts.URL+"/404.jar")
	if !errors.Is(err, ErrDownloadFailed) {
		t.Errorf("expected ErrDownloadFailed on 404, got %v", err)
	}
}

func TestDeleteModAndDownloadNew(t *testing.T) {
	jarContent := []byte("PK\x03\x04new-jar")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jarContent)
	}))
	defer ts.Close()

	s, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	// Seed old version
	oldPath := filepath.Join(s.dataPath(serverID), "plugins", "Chunky-1.4.28.jar")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("old-jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Delete old version by filename
	if err := s.DeleteMod(ctx, serverID, "Chunky-1.4.28.jar"); err != nil {
		t.Fatalf("DeleteMod failed: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old jar to be deleted")
	}

	// Download new version
	newMod, err := s.DownloadMod(ctx, serverID, "Chunky-1.4.29.jar", ts.URL+"/chunky.jar")
	if err != nil {
		t.Fatalf("DownloadMod failed: %v", err)
	}
	if newMod.File != "Chunky-1.4.29.jar" {
		t.Fatalf("expected Chunky-1.4.29.jar, got %q", newMod.File)
	}
	newPath := filepath.Join(s.dataPath(serverID), "plugins", "Chunky-1.4.29.jar")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new jar missing: %v", err)
	}
}

func TestDownloadAndListUniversalGravesWithManifest(t *testing.T) {
	// Create zip with fabric.mod.json
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("fabric.mod.json")
	_, _ = w.Write([]byte(`{"id":"universal-graves","name":"Universal Graves","version":"3.12.0+26.2"}`))
	_ = zw.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/java-archive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer ts.Close()

	s, serverID := newModTestStore(t, "fabric")
	ctx := context.Background()

	mod, err := s.DownloadMod(ctx, serverID, "graves-3.12.0+26.2.jar", ts.URL+"/graves.jar", ModDownloadMeta{
		ProjectID:   "yn9u3ypm",
		ProjectSlug: "universal-graves",
		Provider:    "modrinth",
	})
	if err != nil {
		t.Fatalf("DownloadMod returned error: %v", err)
	}

	if mod.ModID != "universal-graves" {
		t.Errorf("got ModID %q, want 'universal-graves'", mod.ModID)
	}
	if mod.Title != "Universal Graves" {
		t.Errorf("got Title %q, want 'Universal Graves'", mod.Title)
	}
	if mod.ProjectID != "yn9u3ypm" {
		t.Errorf("got ProjectID %q, want 'yn9u3ypm'", mod.ProjectID)
	}
	if mod.SHA1 == "" {
		t.Errorf("expected non-empty SHA1")
	}

	list, err := s.ListMods(ctx, serverID)
	if err != nil {
		t.Fatalf("ListMods returned error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list.Items))
	}
	item := list.Items[0]
	if item.ModID != "universal-graves" || item.ProjectID != "yn9u3ypm" || item.Title != "Universal Graves" {
		t.Errorf("unexpected item fields in ListMods: %+v", item)
	}
}


