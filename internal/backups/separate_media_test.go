package backups

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestWorld(t *testing.T, s *Store, serverID, content string) string {
	t.Helper()
	seedBackupServer(t, s, serverID)
	path := filepath.Join(s.serverDataDir(serverID), "world", "level.dat")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertNoStagedArchives(t *testing.T, backupDir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(backupDir, ".tmp"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("staging files remain: %v, %v", entries, err)
	}
}

func TestSeparateBackupMediaLifecycle(t *testing.T) {
	base, handle := openTestStore(t)
	mediaDir := filepath.Join(t.TempDir(), "backup drive", "archives")
	s := New(handle.DB, S3Config{}, base.dataDir, mediaDir)
	ctx := context.Background()
	const serverID = "separate-media"
	worldPath := writeTestWorld(t, s, serverID, "original-world")

	// Staging must also use the backup media, even when TMPDIR is unusable.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-system-temp"))
	archive, err := s.archiveWorld(ctx, serverID)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(archive) != filepath.Join(mediaDir, ".tmp") {
		t.Fatalf("archive staged outside backup media: %s", archive)
	}
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}

	b, err := s.Backup(ctx, serverID, "separate-disk")
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(mediaDir, serverID, b.ID+".tar.gz")
	archiveData, err := os.ReadFile(archivePath)
	if err != nil || len(archiveData) == 0 || b.Status != StatusCompleted {
		t.Fatalf("backup missing from separate media: %v, %#v", err, b)
	}
	if _, err := os.Stat(filepath.Join(base.dataDir, "backups")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup wrote to the server data disk: %v", err)
	}

	uploaded, err := s.UploadBackup(ctx, serverID, "uploaded", bytes.NewReader(archiveData), int64(len(archiveData)), "local")
	if err != nil {
		t.Fatal(err)
	}
	for _, backup := range []*Backup{b, uploaded} {
		rc, size, _, err := s.OpenBackup(ctx, backup.ID)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || !bytes.Equal(data, archiveData) || size != int64(len(archiveData)) {
			t.Fatalf("download differs from archive: size=%d err=%v", size, err)
		}
		writeTestWorld(t, s, serverID, "changed-world")
		if err := s.Restore(ctx, backup.ID); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(worldPath); err != nil || string(data) != "original-world" {
			t.Fatalf("restore failed: %q, %v", data, err)
		}
		if err := s.Delete(ctx, backup.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(mediaDir, serverID, backup.ID+".tar.gz")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted archive remains: %v", err)
		}
		if _, err := s.Get(ctx, backup.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("deleted record remains: %v", err)
		}
	}
	assertNoStagedArchives(t, mediaDir)
}

func TestExistingBackupsAfterChangingMedia(t *testing.T) {
	for _, move := range []bool{false, true} {
		name := "original disk"
		if move {
			name = "moved to backup media"
		}
		t.Run(name, func(t *testing.T) {
			old, handle := openTestStore(t)
			ctx := context.Background()
			const serverID = "existing-server"
			worldPath := writeTestWorld(t, old, serverID, "existing-world")
			b, err := old.Backup(ctx, serverID, "existing", "local")
			if err != nil {
				t.Fatal(err)
			}
			s := New(handle.DB, S3Config{}, old.dataDir, t.TempDir())
			if move {
				if err := os.MkdirAll(filepath.Dir(s.localBackupPath(serverID, b.ID)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(old.localBackupPath(serverID, b.ID), s.localBackupPath(serverID, b.ID)); err != nil {
					t.Fatal(err)
				}
			}
			rc, _, _, err := s.OpenBackup(ctx, b.ID)
			if err != nil {
				t.Fatal(err)
			}
			rc.Close()
			writeTestWorld(t, s, serverID, "changed")
			if err := s.Restore(ctx, b.ID); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(worldPath); err != nil || string(data) != "existing-world" {
				t.Fatalf("existing archive cannot be restored: %q, %v", data, err)
			}
			// Retention must remove the existing archive from its actual disk.
			if _, err := s.db.Exec(`UPDATE backups SET created_at='2000-01-01T00:00:00Z' WHERE id=?`, b.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(`UPDATE settings SET value='1' WHERE key='backup_retention'`); err != nil {
				t.Fatal(err)
			}
			latest, err := s.Backup(ctx, serverID, "new-disk", "local")
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{old.localBackupPath(serverID, b.ID), s.localBackupPath(serverID, b.ID)} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("retention left old archive at %s: %v", path, err)
				}
			}
			if _, err := os.Stat(s.localBackupPath(serverID, latest.ID)); err != nil {
				t.Fatalf("new backup missing from separate media: %v", err)
			}
		})
	}
}

func TestBackupMediaFailureDoesNotWriteToDataDisk(t *testing.T) {
	base, handle := openTestStore(t)
	mediaDir := filepath.Join(t.TempDir(), "unavailable")
	if err := os.WriteFile(mediaDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(handle.DB, S3Config{}, base.dataDir, mediaDir)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "world")
	b, err := s.Backup(ctx, "server", "failed", "local")
	if err == nil || b == nil {
		t.Fatalf("expected backup storage failure, got %#v, %v", b, err)
	}
	record, err := s.Get(ctx, b.ID)
	if err != nil || record.Status != StatusFailed {
		t.Fatalf("failed backup not recorded: %#v, %v", record, err)
	}
	if _, err := s.UploadBackup(ctx, "server", "upload", strings.NewReader("archive"), 7, "local"); err == nil {
		t.Fatal("upload succeeded with unavailable backup media")
	}
	if _, err := os.Stat(filepath.Join(base.dataDir, "backups")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed writes fell back to the data disk: %v", err)
	}
}

func TestFailedLocalDeletePreservesBackupRecord(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	payload := validBackupBytes(t, s, "server")
	b, err := s.UploadBackup(ctx, "server", "archive", bytes.NewReader(payload), int64(len(payload)), "local")
	if err != nil {
		t.Fatal(err)
	}
	path := s.localBackupPath(b.ServerID, b.ID)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// A nonempty directory produces a deletion failure even for a root test user.
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocked"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, b.ID); err == nil {
		t.Fatal("expected storage deletion failure")
	}
	if _, err := s.Get(ctx, b.ID); err != nil {
		t.Fatalf("backup record lost after failed deletion: %v", err)
	}
}

func TestBackupMediaCannotBeInsideArchivedServer(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "nested directory", true: "symlink"}[symlink], func(t *testing.T) {
			s, _ := openTestStore(t)
			writeTestWorld(t, s, "server", "world")
			mediaDir := filepath.Join(s.serverDataDir("server"), "archives")
			if symlink {
				mediaDir = filepath.Join(t.TempDir(), "linked-backups")
				if err := os.Symlink(s.serverDataDir("server"), mediaDir); err != nil {
					t.Fatal(err)
				}
			}
			s.backupDir = mediaDir
			if _, err := s.Backup(context.Background(), "server", "recursive", "local"); err == nil || !strings.Contains(err.Error(), "outside the server data") {
				t.Fatalf("expected recursive archive rejection, got %v", err)
			}
			assertNoStagedArchives(t, mediaDir)
		})
	}
}

type mediaTestTransport func(*http.Request) (*http.Response, error)

func (f mediaTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestS3BackupUsesSeparateStagingMedia(t *testing.T) {
	base, handle := openTestStore(t)
	mediaDir := t.TempDir()
	s := New(handle.DB, S3Config{Endpoint: "https://storage.invalid", Bucket: "worlds"}, base.dataDir, mediaDir)
	objects := map[string][]byte{}
	s.client.httpClient.Transport = mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.Method {
		case http.MethodPut:
			entries, err := os.ReadDir(filepath.Join(mediaDir, ".tmp"))
			if err != nil || len(entries) != 1 {
				t.Errorf("S3 upload is not staged on backup media: %v, %v", entries, err)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			objects[r.URL.Path] = data
		case http.MethodGet:
			body = objects[r.URL.Path]
		case http.MethodDelete:
			delete(objects, r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing-system-temp"))
	ctx := context.Background()
	worldPath := writeTestWorld(t, s, "cloud-server", "cloud-world")
	b, err := s.Backup(ctx, "cloud-server", "cloud")
	if err != nil {
		t.Fatal(err)
	}
	data := objects["/worlds/"+b.Location]
	uploaded, err := s.UploadBackup(ctx, b.ServerID, "uploaded", bytes.NewReader(data), int64(len(data)), "s3")
	if err != nil {
		t.Fatal(err)
	}
	for _, backup := range []*Backup{b, uploaded} {
		writeTestWorld(t, s, b.ServerID, "changed")
		if err := s.Restore(ctx, backup.ID); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(worldPath); err != nil || string(data) != "cloud-world" {
			t.Fatalf("S3 restore failed: %q, %v", data, err)
		}
		if err := s.Delete(ctx, backup.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(objects) != 0 {
		t.Fatalf("S3 objects remain after deletion: %d", len(objects))
	}
	assertNoStagedArchives(t, mediaDir)
}
