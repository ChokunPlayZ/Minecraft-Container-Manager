package servers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
	"github.com/mcm-panel/mcm/internal/storage"
)

type importTestEntry struct {
	name string
	data string
	mode fs.FileMode
}

func importTestZIP(t *testing.T, entries ...importTestEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			h.SetMode(entry.mode)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func importTestStore(t *testing.T, end int) (*Store, *db.Store) {
	t.Helper()
	dir := t.TempDir()
	dbh, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	return NewStore(dbh, nil, nil, 25565, end, dir, ""), dbh
}

func TestImportExportedServerRoundTrip(t *testing.T) {
	s, dbh := importTestStore(t, 25570)
	insertServer(t, dbh, "source", 25565, "old-container", StateRunning)
	_, err := dbh.DB.Exec(`UPDATE servers SET name='Original', ram_mb=4096, java_version=17, cpu_limit=2, memory_limit_mb=5120, extra_ports='[{"host_port":25569,"container_port":8123,"protocol":"tcp"}]' WHERE id='source'`)
	if err != nil {
		t.Fatal(err)
	}
	dir := s.dataPath("source")
	if err := os.MkdirAll(filepath.Join(dir, "world"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"server.jar": "original jar", "world/level.dat": "world", "run.sh": "#!/bin/sh", "server.properties": "motd=Original"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Reserved export metadata on disk cannot override current panel settings.
	if err := os.WriteFile(filepath.Join(dir, exportManifestName), []byte("stale metadata"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("server.jar", filepath.Join(dir, "linked.jar")); err != nil {
		t.Fatal(err)
	}
	var exported bytes.Buffer
	if err := s.Export(context.Background(), "source", &exported); err != nil {
		t.Fatal(err)
	}
	srv, err := s.Import(context.Background(), bytes.NewReader(exported.Bytes()), int64(exported.Len()), ImportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if srv.ID == "source" || srv.Name != "Original" || srv.ServerType != "paper" || srv.Version != "1.21.1" || srv.Build != "120" || srv.RAMMB != 4096 || srv.JavaVersion != 17 || srv.CPULimit != 2 || srv.MemoryLimitMB != 5120 {
		t.Fatalf("settings did not round trip: %+v", srv)
	}
	if srv.HostPort == 25565 || srv.ContainerID != "" || srv.State != StateStopped || len(srv.ExtraPorts) != 0 || srv.StartedAt != "" {
		t.Fatalf("import reused runtime identity or networking: %+v", srv)
	}
	for name, want := range map[string]string{"server.jar": "original jar", "world/level.dat": "world", "run.sh": "#!/bin/sh", "server.properties": "motd=Original"} {
		got, err := os.ReadFile(filepath.Join(s.dataPath(srv.ID), name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
	info, err := os.Stat(filepath.Join(s.dataPath(srv.ID), "run.sh"))
	if err != nil || info.Mode().Perm()&0100 == 0 {
		t.Fatalf("launch script lost executable permission: %v", err)
	}
	for _, name := range []string{exportManifestName, "linked.jar"} {
		if _, err := os.Lstat(filepath.Join(s.dataPath(srv.ID), name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected imported file %s: %v", name, err)
		}
	}
	var schedules int
	if err := dbh.DB.QueryRow(`SELECT COUNT(*) FROM scheduled_tasks WHERE server_id=?`, srv.ID).Scan(&schedules); err != nil || schedules != 1 {
		t.Fatalf("default backup schedule: %d, %v", schedules, err)
	}
}

func TestImportLegacyServerAndOverrides(t *testing.T) {
	s, _ := importTestStore(t, 25570)
	data := importTestZIP(t, importTestEntry{name: "server.jar", data: "jar"})
	_, err := s.Import(context.Background(), bytes.NewReader(data), int64(len(data)), ImportInput{Name: "Legacy"})
	var validation *ImportValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("legacy ZIP must request missing settings: %v", err)
	}
	srv, err := s.Import(context.Background(), bytes.NewReader(data), int64(len(data)), ImportInput{Name: " Legacy ", ServerType: jars.TypeFabric, Version: "1.20.1", NoHostPort: true})
	if err != nil {
		t.Fatal(err)
	}
	if srv.Name != "Legacy" || srv.RAMMB != 2048 || srv.JavaVersion != 17 || srv.HostPort != 0 || srv.ServerType != "fabric" {
		t.Fatalf("legacy defaults: %+v", srv)
	}
	manifest, _ := json.Marshal(exportManifest{FormatVersion: 1, Server: exportSettings{Name: "Old", ServerType: jars.TypePaper, Version: "1.21.1", RAMMB: 2048, JavaVersion: 21}})
	data = importTestZIP(t, importTestEntry{name: exportManifestName, data: string(manifest)}, importTestEntry{name: "server.jar", data: "jar"})
	srv, err = s.Import(context.Background(), bytes.NewReader(data), int64(len(data)), ImportInput{Name: "Override", RAMMB: 8192, JavaVersion: 25})
	if err != nil || srv.Name != "Override" || srv.RAMMB != 8192 || srv.JavaVersion != 25 {
		t.Fatalf("override settings: %+v, %v", srv, err)
	}
}

func TestImportRejectsUnsafeAndDamagedZIPs(t *testing.T) {
	valid := ImportInput{Name: "Rejected", ServerType: jars.TypePaper, Version: "1.21.1"}
	corrupt := importTestZIP(t, importTestEntry{name: "world/level.dat", data: "sentinel-content"})
	corrupt[bytes.Index(corrupt, []byte("sentinel-content"))] ^= 1
	for name, data := range map[string][]byte{
		"traversal":        importTestZIP(t, importTestEntry{name: "../escape", data: "bad"}),
		"absolute":         importTestZIP(t, importTestEntry{name: "/escape", data: "bad"}),
		"backslash":        importTestZIP(t, importTestEntry{name: `world\escape`, data: "bad"}),
		"symlink":          importTestZIP(t, importTestEntry{name: "link", data: "../escape", mode: os.ModeSymlink | 0777}),
		"duplicate":        importTestZIP(t, importTestEntry{name: "server.jar", data: "a"}, importTestEntry{name: "server.jar", data: "b"}),
		"alias duplicate":  importTestZIP(t, importTestEntry{name: "server.jar", data: "a"}, importTestEntry{name: "world/../server.jar", data: "b"}),
		"checksum":         corrupt,
		"not zip":          []byte("bad"),
		"empty zip":        importTestZIP(t),
		"invalid metadata": importTestZIP(t, importTestEntry{name: exportManifestName, data: "bad"}),
		"future metadata":  importTestZIP(t, importTestEntry{name: exportManifestName, data: `{"format_version":2}`}),
	} {
		t.Run(name, func(t *testing.T) {
			s, dbh := importTestStore(t, 25570)
			if _, err := s.Import(context.Background(), bytes.NewReader(data), int64(len(data)), valid); err == nil {
				t.Fatal("invalid ZIP accepted")
			}
			var count int
			if err := dbh.DB.QueryRow(`SELECT COUNT(*) FROM servers`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("failed import published a server: %d, %v", count, err)
			}
			entries, err := os.ReadDir(filepath.Join(s.dataDir, "servers"))
			if err != nil && !os.IsNotExist(err) || len(entries) != 0 {
				t.Fatalf("failed import left files: %v, %v", entries, err)
			}
		})
	}
}

func TestImportFailureCleanupAndLimits(t *testing.T) {
	data := importTestZIP(t, importTestEntry{name: "server.jar", data: "jar"})
	valid := ImportInput{Name: "Imported", ServerType: jars.TypePaper, Version: "1.21.1"}
	for _, failure := range []string{"port pool full", "schedule failure", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			s, dbh := importTestStore(t, 25565)
			ctx := context.Background()
			switch failure {
			case "port pool full":
				insertServer(t, dbh, "existing", 25565, "", StateStopped)
			case "schedule failure":
				if _, err := dbh.DB.Exec(`DROP TABLE scheduled_tasks`); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := s.Import(ctx, bytes.NewReader(data), int64(len(data)), valid)
			if err == nil || failure == "port pool full" && !errors.Is(err, ports.ErrPortPoolFull) {
				t.Fatalf("expected %s: %v", failure, err)
			}
			var count int
			if err := dbh.DB.QueryRow(`SELECT COUNT(*) FROM servers WHERE name='Imported'`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("leftover server: %d, %v", count, err)
			}
			entries, err := os.ReadDir(filepath.Join(s.dataDir, "servers"))
			if err != nil && !os.IsNotExist(err) || len(entries) != 0 {
				t.Fatalf("leftover files: %v, %v", entries, err)
			}
		})
	}
	s, _ := importTestStore(t, 25570)
	if _, err := s.Import(context.Background(), bytes.NewReader(data), storage.MaxUploadBytes+1, valid); err == nil {
		t.Fatal("oversized upload accepted")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.CreateRaw(&zip.FileHeader{Name: "oversized", Method: zip.Store, UncompressedSize64: uint64(storage.MaxArchiveBytes) + 1}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(context.Background(), bytes.NewReader(buf.Bytes()), int64(buf.Len()), valid); err == nil {
		t.Fatal("oversized extracted file accepted")
	}
}
