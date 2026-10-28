package backups

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewRestoreDoesNotRetainPostSnapshotFiles(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	dir := s.serverDataDir("server")
	os.MkdirAll(filepath.Join(dir, "world/region"), 0755)
	os.WriteFile(filepath.Join(dir, "world/region/r.0.0.mca"), []byte("snapshot"), 0644)
	b, e := s.Backup(ctx, "server", "old", "local")
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(dir, "world/region/r.1.0.mca"), []byte("post snapshot"), 0644)
	if e = s.Restore(ctx, b.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, "world/region/r.1.0.mca")); e == nil {
		t.Error("restore retained a region that did not exist in the snapshot")
	}
}
func TestReviewRetentionProtectsSuccessfulBackups(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	s.db.Exec(`INSERT INTO settings(key,value) VALUES('backup_retention','2') ON CONFLICT(key) DO UPDATE SET value='2'`)
	for _, r := range []struct{ id, status, at string }{{"good", StatusCompleted, "2026-10-01T00:00:00Z"}, {"bad", StatusFailed, "2026-10-02T00:00:00Z"}, {"new", StatusCompleted, "2026-10-03T00:00:00Z"}} {
		s.db.Exec(`INSERT INTO backups(id,server_id,name,size_bytes,location,status,created_at) VALUES(?,?,?,0,?,?,?)`, r.id, "server", r.id, "local:test", r.status, r.at)
	}
	if e := s.enforceRetention(ctx, "server"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Get(ctx, "good"); e != nil {
		t.Errorf("retention discarded a successful snapshot while keeping a failed record: %v", e)
	}
}
func TestReviewInvalidStorageDoesNotPanic(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	os.MkdirAll(s.serverDataDir("server"), 0755)
	os.WriteFile(filepath.Join(s.serverDataDir("server"), "file"), []byte("world"), 0644)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("invalid storage request panicked: %v", r)
		}
	}()
	_, e := s.Backup(ctx, "server", "x", "typo")
	if e == nil {
		t.Error("invalid storage accepted")
	}
}
func TestReviewUnsupportedUploadRejected(t *testing.T) {
	s, _ := openTestStore(t)
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	w, _ := z.Create("world/level.dat")
	w.Write([]byte("world"))
	z.Close()
	b, e := s.UploadBackup(context.Background(), "server", "upload.zip", bytes.NewReader(data.Bytes()), int64(data.Len()), "local")
	if e == nil || b != nil {
		t.Fatalf("unsupported upload published: %+v %v", b, e)
	}
}
