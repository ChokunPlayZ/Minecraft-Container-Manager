package backups

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRetentionPolicyBoundaries(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	p, err := s.GetPolicy(ctx, "server")
	if err != nil || p != (Policy{KeepCount: 10, TrashRetentionDays: 30}) {
		t.Fatalf("legacy defaults not preserved: %+v %v", p, err)
	}
	for _, p := range []Policy{{-1, 0, 30}, {100001, 0, 30}, {1, -1, 30}, {1, 36501, 30}, {1, 0, -1}, {1, 0, 36501}} {
		if err := s.SavePolicy(ctx, "server", p); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("accepted %+v: %v", p, err)
		}
	}
	for _, p := range []Policy{{0, 0, 0}, {100000, 36500, 36500}} {
		if err := s.SavePolicy(ctx, "server", p); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetPolicy(ctx, "server")
		if err != nil || got != p {
			t.Fatalf("policy round trip: %+v %v", got, err)
		}
	}
	other, err := s.GetPolicy(ctx, "srv-local-test")
	if err != nil || other.KeepCount != 10 {
		t.Fatalf("policy crossed server scope: %+v %v", other, err)
	}
}

func TestRetentionPlanSafety(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	backup := func(id, status string, days int, protected bool) Backup {
		return Backup{ID: id, Name: id, Status: status, CreatedAt: now.Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339), SizeBytes: 100, Protected: protected}
	}
	active := []Backup{backup("pending", StatusPending, 0, false), backup("failed", StatusFailed, 0, false), backup("latest", StatusCompleted, 40, false), backup("protected", StatusCompleted, 50, true), backup("old", StatusCompleted, 60, false), backup("oldest", StatusCompleted, 70, false)}
	trash := []Backup{{ID: "expired", SizeBytes: 25, DeletedAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339)}, {ID: "recent", DeletedAt: now.Add(-time.Hour).Format(time.RFC3339)}, {ID: "malformed", DeletedAt: "invalid"}}
	for _, tc := range []struct {
		name   string
		policy Policy
		want   int
		reason string
		purge  int
	}{
		{"count", Policy{1, 0, 30}, 2, "count", 1},
		{"age", Policy{0, 14, 0}, 2, "age", 0},
		{"combined", Policy{1, 14, 30}, 2, "age", 1},
		{"unlimited", Policy{0, 0, 0}, 0, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := planRetention(tc.policy, active, trash, now)
			if len(p.ToTrash) != tc.want || len(p.ToPurge) != tc.purge || p.TrashBytes != int64(tc.want*100) {
				t.Fatalf("unexpected plan: %+v", p)
			}
			for _, b := range p.ToTrash {
				if (b.ID != "old" && b.ID != "oldest") || b.Reason != tc.reason {
					t.Fatalf("unsafe candidate: %+v", b)
				}
			}
		})
	}
	// A protected archive does not consume a slot in the ordinary backup count.
	active[2].Protected = true
	p := planRetention(Policy{1, 0, 30}, active, nil, now)
	if len(p.ToTrash) != 1 || p.ToTrash[0].ID != "oldest" {
		t.Fatalf("protected backups consumed count: %+v", p)
	}
}

func TestTrashRecoverProtectAndPurgeLocal(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "original world")
	b, err := s.Backup(ctx, "server", "snapshot", "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, b.ID); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("purged active backup: %v", err)
	}
	if err := s.Protect(ctx, b.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("trashed protected backup: %v", err)
	}
	if err := s.Protect(ctx, b.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil {
		t.Fatal("trash should be idempotent", err)
	}
	if _, err := os.Stat(s.localBackupPath("server", b.ID)); err != nil {
		t.Fatal("trash removed archive", err)
	}
	list, _ := s.List(ctx, "server")
	trash, _ := s.ListTrash(ctx, "server")
	if len(list) != 0 || len(trash) != 1 || trash[0].DeletedAt == "" {
		t.Fatalf("trash list incorrect: %+v %+v", list, trash)
	}
	if _, _, _, err := s.OpenBackup(ctx, b.ID); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("downloaded trash: %v", err)
	}
	if err := s.Restore(ctx, b.ID); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("restored trash into world: %v", err)
	}
	if err := s.Recover(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(ctx, b.ID)
	if !got.Protected || got.DeletedAt != "" {
		t.Fatalf("recovery did not protect backup: %+v", got)
	}
	if err := s.Restore(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Protect(ctx, b.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, b.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("record survived purge: %v", err)
	}
	if _, err := os.Stat(s.localBackupPath("server", b.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive survived purge: %v", err)
	}
}

func TestCleanupPreviewExpiryAndEmptyTrash(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "world")
	if err := s.SavePolicy(ctx, "server", Policy{0, 0, 30}); err != nil {
		t.Fatal(err)
	}
	var records []*Backup
	for i := 0; i < 4; i++ {
		b, err := s.Backup(ctx, "server", "snapshot", "local")
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, b)
	}
	// Saving limits must never itself mutate existing archives.
	if err := s.SavePolicy(ctx, "server", Policy{1, 0, 1}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(ctx, "server")
	if len(list) != 4 {
		t.Fatal("saving policy ran cleanup")
	}
	preview, err := s.PreviewRetention(ctx, "server")
	if err != nil || len(preview.ToTrash) != 3 || len(preview.ToPurge) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	result, err := s.Cleanup(ctx, "server", false)
	if err != nil || result != (CleanupResult{Trashed: 3}) {
		t.Fatalf("cleanup: %+v %v", result, err)
	}
	for _, b := range records {
		if _, err := os.Stat(s.localBackupPath("server", b.ID)); err != nil {
			t.Fatal("cleanup removed recoverable archive", err)
		}
	}
	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE backups SET deleted_at=? WHERE id=?`, old, records[0].ID); err != nil {
		t.Fatal(err)
	}
	result, err = s.Cleanup(ctx, "server", false)
	if err != nil || result.Purged != 1 || result.Trashed != 0 {
		t.Fatalf("expiry: %+v %v", result, err)
	}
	result, err = s.Cleanup(ctx, "server", true)
	if err != nil || result.Purged != 2 {
		t.Fatalf("empty trash: %+v %v", result, err)
	}
	list, _ = s.List(ctx, "server")
	if len(list) != 1 || list[0].ID != records[3].ID {
		t.Fatalf("empty trash changed active backups: %+v", list)
	}
}

func TestS3TrashAndFailedPurgeRemainRecoverable(t *testing.T) {
	base, handle := openTestStore(t)
	s := New(handle.DB, S3Config{Endpoint: "https://storage.invalid", Bucket: "backups"}, base.dataDir)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "cloud world")
	deletes := 0
	fail := true
	s.client.httpClient.Transport = mediaTestTransport(func(r *http.Request) (*http.Response, error) {
		code := http.StatusOK
		if r.Method == http.MethodDelete {
			deletes++
			if fail {
				code = http.StatusServiceUnavailable
			}
		}
		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	b, err := s.Backup(ctx, "server", "cloud", "s3")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil || deletes != 0 {
		t.Fatalf("trash deleted S3 object: %v %d", err, deletes)
	}
	if err := s.Purge(ctx, b.ID); err == nil {
		t.Fatal("expected S3 failure")
	}
	got, err := s.Get(ctx, b.ID)
	if err != nil || got.DeletedAt == "" {
		t.Fatalf("failed purge lost record: %+v %v", got, err)
	}
	if err := s.Recover(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Protect(ctx, b.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	fail = false
	if err := s.Purge(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if deletes != 2 {
		t.Fatalf("unexpected S3 delete calls: %d", deletes)
	}
}

func TestTrashRejectsPendingAndRecoveryDoesNotUnprotectActive(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	_, err := s.db.Exec(`INSERT INTO backups(id,server_id,name,status,created_at,location) VALUES ('pending','server','pending','pending','2026-10-30T00:00:00Z','local:pending')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, "pending"); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("trashed pending record: %v", err)
	}
	if err := s.Protect(ctx, "pending", true); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("protected pending record: %v", err)
	}
	if err := s.Recover(ctx, "pending"); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("recovered active record: %v", err)
	}
}

func TestSuccessfulBackupReportsCleanupFailureWithoutLosingSnapshot(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "world")
	// Expired remote trash cannot be purged while S3 is unavailable. Local
	// snapshot creation must still succeed and report the separate cleanup issue.
	_, err := s.db.Exec(`INSERT INTO backups(id,server_id,name,status,created_at,location,deleted_at) VALUES('remote-trash','server','remote','completed','2000-01-01T00:00:00Z','remote','2000-01-02T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Backup(ctx, "server", "new snapshot", "local")
	if err != nil || b == nil || b.Status != StatusCompleted || !strings.Contains(b.Warning, "cleanup needs attention") {
		t.Fatalf("valid snapshot reported as failed: %+v %v", b, err)
	}
	p := s.GetProgress("server")
	if p == nil || p.Stage != "completed" || !strings.Contains(p.Message, "cleanup needs attention") {
		t.Fatalf("cleanup warning missing from progress: %+v", p)
	}
	if _, err := s.Get(ctx, "remote-trash"); err != nil {
		t.Fatalf("failed purge lost trash: %v", err)
	}
}

func TestRetentionWorkerRunsWithoutScheduledBackups(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "world")
	b, err := s.Backup(ctx, "server", "expired trash", "local")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE backups SET deleted_at='2000-01-01T00:00:00Z' WHERE id=?`, b.ID); err != nil {
		t.Fatal(err)
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.RunRetention(work, nil) }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("startup retention did not expire trash")
		case <-ticker.C:
			_, err := s.Get(ctx, b.ID)
			if errors.Is(err, ErrNotFound) {
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("retention worker did not stop")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
