package servers

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
)

func TestCreateWithHostPortAndConflict(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	pool := ports.NewPool(dbHandle.DB, 25565, 25570)
	jr := jars.NewResolver()
	s := NewStore(dbHandle, nil, jr, 25565, 25570, dir, "")
	s.ports = pool

	// Create with specific port 25580.
	srv1, err := s.Create(context.Background(), CreateInput{
		Name:       "server-1",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
		HostPort:   25580,
	})
	if err != nil {
		t.Fatalf("Create srv1: %v", err)
	}
	if srv1.HostPort != 25580 {
		t.Errorf("expected port 25580, got %d", srv1.HostPort)
	}

	// Try creating second server with identical port 25580 -> should fail with ErrPortInUse.
	_, err = s.Create(context.Background(), CreateInput{
		Name:       "server-2",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
		HostPort:   25580,
	})
	if err == nil {
		t.Fatal("expected Create to fail with port conflict, got nil error")
	}
	if !errors.Is(err, ErrPortInUse) {
		t.Errorf("expected ErrPortInUse, got: %v", err)
	}

	// Create third server with port 0 -> should auto-allocate from pool.
	srv3, err := s.Create(context.Background(), CreateInput{
		Name:       "server-3",
		ServerType: jars.TypePaper,
		Version:    "1.21.1",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("Create srv3: %v", err)
	}
	if srv3.HostPort != 25565 {
		t.Errorf("expected srv3 to allocate 25565, got %d", srv3.HostPort)
	}
}

func TestServerExport(t *testing.T) {
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbHandle.Close() })

	s := &Store{db: dbHandle.DB, dataDir: dir}
	id := uuid.NewString()
	insertServer(t, dbHandle, id, 25565, "", StateStopped)

	// Create some files in server dataDir.
	srvDir := s.dataPath(id)
	if err := os.MkdirAll(filepath.Join(srvDir, "plugins"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srvDir, "server.properties"), []byte("motd=Hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srvDir, "plugins", "test.jar"), []byte("jarcontent"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	if err := s.Export(context.Background(), id, &buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Verify zip contents.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}

	foundProps := false
	foundPlugin := false
	for _, f := range zr.File {
		if f.Name == "server.properties" {
			foundProps = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zipped file: %v", err)
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			if string(data) != "motd=Hello" {
				t.Errorf("unexpected content in zipped file: %s", string(data))
			}
		}
		if f.Name == "plugins/test.jar" {
			foundPlugin = true
		}
	}
	if !foundProps {
		t.Error("server.properties not found in export zip")
	}
	if !foundPlugin {
		t.Error("plugins/test.jar not found in export zip")
	}
}
