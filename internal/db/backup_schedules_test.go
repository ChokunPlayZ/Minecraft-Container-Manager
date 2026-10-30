package db

import (
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/migrations"
)

func TestBackupScheduleMigrationPreservesConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-backups.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Restore the previous task schema before seeding an installation to upgrade.
	oldSchema, err := migrations.FS.ReadFile("0016_scheduled_tasks.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`DROP TABLE scheduled_tasks;` + string(oldSchema)); err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`
		DELETE FROM schema_migrations WHERE name IN ('0023_backup_schedules.sql', '0025_schedule_chains.sql');
		INSERT INTO servers (id,name,server_type,version,ram_mb,backup_enabled,backup_interval_minutes,created_at,updated_at)
		VALUES ('active','Active','paper','1.21.4',2048,1,95,'2026-10-01T00:00:00Z',''),
		('paused','Paused','paper','1.21.4',2048,0,37,'2026-10-01T00:00:00Z',''),
		('fallback','Fallback','paper','1.21.4',2048,1,0,'2026-10-01T00:00:00Z','');
		INSERT INTO backups (id,server_id,name,size_bytes,location,status,created_at)
		VALUES ('completed','active','Snapshot',1,'local','completed','2026-10-27T09:00:00Z'),
		('failed','active','Failed',0,'local','failed','2026-10-27T10:00:00Z');
		INSERT INTO scheduled_tasks (id,server_id,name,action,cron,timezone,next_run_at,last_status,created_at,updated_at)
		VALUES ('restart','active','Nightly restart','restart','midnight','Asia/Bangkok','2026-10-28T17:00:00Z','success','created','updated');
	`)
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		upgraded, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			id, cron, next string
			enabled        bool
		}{
			{"active", "@every 95m", "2026-10-27T10:35:00Z", true},
			{"paused", "@every 37m", "", false},
			{"fallback", "@every 720m", "2026-10-01T12:00:00Z", true},
		} {
			var cron, next, target string
			var enabled bool
			err = upgraded.DB.QueryRow(`SELECT cron,next_run_at,enabled,backup_target FROM scheduled_tasks WHERE server_id=? AND action='backup'`, tc.id).Scan(&cron, &next, &enabled, &target)
			if err != nil || cron != tc.cron || next != tc.next || enabled != tc.enabled || target != "" {
				t.Fatalf("%s migration: cron=%q next=%q enabled=%v err=%v", tc.id, cron, next, enabled, err)
			}
		}
		var count int
		if err := upgraded.DB.QueryRow(`SELECT COUNT(*) FROM scheduled_tasks`).Scan(&count); err != nil || count != 4 {
			t.Fatalf("lost or duplicate tasks: count=%d err=%v", count, err)
		}
		var next, status, timezone string
		if err := upgraded.DB.QueryRow(`SELECT next_run_at,last_status,timezone FROM scheduled_tasks WHERE id='restart'`).Scan(&next, &status, &timezone); err != nil || next != "2026-10-28T17:00:00Z" || status != "success" || timezone != "Asia/Bangkok" {
			t.Fatalf("existing schedule changed: next=%q status=%q zone=%q err=%v", next, status, timezone, err)
		}
		upgraded.Close()
	}
}
