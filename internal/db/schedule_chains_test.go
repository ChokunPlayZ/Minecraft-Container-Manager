package db

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/migrations"
)

func TestChainMigrationPreservesSingleActionsAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chains.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	previous, err := migrations.FS.ReadFile("0023_backup_schedules.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the actual preceding schema with remote backup configuration.
	schema := string(previous)
	schema = schema[strings.Index(schema, "CREATE TABLE scheduled_tasks_new"):strings.Index(schema, "INSERT INTO scheduled_tasks_new")]
	schema = strings.ReplaceAll(schema, "scheduled_tasks_new", "scheduled_tasks")
	_, err = store.DB.Exec(`DROP TABLE scheduled_tasks;` + schema + `
		DELETE FROM schema_migrations WHERE name='0025_schedule_chains.sql';
		INSERT INTO servers (id,name,server_type,version,ram_mb,created_at,updated_at)
		VALUES ('server','Server','paper','1.21.4',2048,'created','updated');
		INSERT INTO scheduled_tasks (id,server_id,name,action,command,backup_target,cron,timezone,enabled,next_run_at,last_run_at,last_status,last_error,created_at,updated_at)
		VALUES ('backup','server','Offsite','backup','','s3','@every 95m','UTC',0,'','last','failed','storage unavailable','created','updated'),
		('command','server','Save world','command','save-all','','midnight','Asia/Bangkok',1,'next','last','success','','created','updated');
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
		var target, steps, status, detail, updated string
		var enabled bool
		err = upgraded.DB.QueryRow(`SELECT backup_target,steps,enabled,last_status,last_error,updated_at FROM scheduled_tasks WHERE id='backup'`).Scan(&target, &steps, &enabled, &status, &detail, &updated)
		if err != nil || target != "s3" || steps != "[]" || enabled || status != "failed" || detail != "storage unavailable" || updated != "updated" {
			t.Fatalf("backup configuration or history lost: target=%q steps=%q enabled=%v status=%q detail=%q updated=%q err=%v", target, steps, enabled, status, detail, updated, err)
		}
		var command, zone, next string
		err = upgraded.DB.QueryRow(`SELECT command,steps,timezone,next_run_at,last_status FROM scheduled_tasks WHERE id='command'`).Scan(&command, &steps, &zone, &next, &status)
		if err != nil || command != "save-all" || steps != "[]" || zone != "Asia/Bangkok" || next != "next" || status != "success" {
			t.Fatalf("command schedule changed: command=%q steps=%q zone=%q next=%q status=%q err=%v", command, steps, zone, next, status, err)
		}
		if run == 1 {
			if _, err := upgraded.DB.Exec(`DELETE FROM servers WHERE id='server'`); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := upgraded.DB.QueryRow(`SELECT count(*) FROM scheduled_tasks`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("migration lost cascade deletion: count=%d err=%v", count, err)
			}
		}
		upgraded.Close()
	}
}
