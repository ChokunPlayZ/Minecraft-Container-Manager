package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/schedules"
)

func TestScheduledBackupUsesBackupStore(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if _, err := database.DB.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,created_at,updated_at) VALUES ('server','Server','paper','1.21.4',2048,'','')`); err != nil {
		t.Fatal(err)
	}
	world := filepath.Join(dir, "servers", "server", "world")
	if err := os.MkdirAll(world, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(world, "level.dat"), []byte("world snapshot"), 0644); err != nil {
		t.Fatal(err)
	}
	store := backups.New(database.DB, backups.S3Config{}, dir)
	execute := scheduledTaskExecutor(nil, store)
	ctx := context.Background()
	if err := execute(ctx, schedules.Task{Action: "backup", ServerID: "server"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx, "server")
	if err != nil || len(list) != 1 || list[0].Status != backups.StatusCompleted {
		t.Fatalf("scheduled archive missing: %+v %v", list, err)
	}
	if err := execute(ctx, schedules.Task{Action: "backup", ServerID: "missing"}); err == nil {
		t.Fatal("backup failure was not returned to the scheduler")
	}
	if err := execute(ctx, schedules.Task{Action: "backup", ServerID: "server", BackupTarget: "s3"}); !errors.Is(err, backups.ErrNotConfigured) {
		t.Fatalf("unconfigured remote target: %v", err)
	}

	var uploads atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/snapshots/") {
			http.Error(w, "unexpected object request", http.StatusBadRequest)
			return
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil || len(payload) == 0 {
			t.Error("remote backup archive was empty", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		uploads.Add(1)
	}))
	t.Cleanup(remote.Close)
	remoteStore := backups.New(database.DB, backups.S3Config{Endpoint: remote.URL, Bucket: "snapshots", AccessKey: "test", SecretKey: "test", Region: "us-east-1"}, dir)
	execute = scheduledTaskExecutor(nil, remoteStore)
	for _, tc := range []struct {
		name, panelDefault, target string
		wantUploads                int32
	}{
		{"explicit remote overrides local default", "local", "s3", 1},
		{"explicit local overrides remote default", "s3", "local", 1},
		{"panel default uses configured storage", "s3", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := database.DB.Exec(`INSERT INTO settings (key,value) VALUES ('backup_storage_type',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, tc.panelDefault); err != nil {
				t.Fatal(err)
			}
			if err := execute(ctx, schedules.Task{Action: "backup", ServerID: "server", BackupTarget: tc.target}); err != nil {
				t.Fatal(err)
			}
			if uploads.Load() != tc.wantUploads {
				t.Fatalf("wrong storage selected: uploads=%d want=%d", uploads.Load(), tc.wantUploads)
			}
		})
	}
}
