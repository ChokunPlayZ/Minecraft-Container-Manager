package db

import (
	"path/filepath"
	"testing"
)

func TestBackupRetentionMigrationPreservesArchivesAndDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-retention.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`
		DROP TABLE backup_policies;
		DROP INDEX backups_trash;
		ALTER TABLE backups DROP COLUMN protected;
		ALTER TABLE backups DROP COLUMN deleted_at;
		DELETE FROM schema_migrations WHERE name='0026_backup_retention.sql';
		UPDATE settings SET value='25' WHERE key='backup_retention';
		INSERT INTO backups(id,server_id,name,size_bytes,location,status,created_at) VALUES
		('existing','server','Original archive',12345,'local:backups/server/existing.tar.gz','completed','2026-10-01T00:00:00Z');
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
		var name, location, deleted, setting string
		var size int64
		var protected bool
		err = upgraded.DB.QueryRow(`SELECT name,location,size_bytes,protected,deleted_at FROM backups WHERE id='existing'`).Scan(&name, &location, &size, &protected, &deleted)
		if err != nil || name != "Original archive" || location != "local:backups/server/existing.tar.gz" || size != 12345 || protected || deleted != "" {
			t.Fatalf("migration changed archive: name=%q location=%q size=%d protected=%v deleted=%q err=%v", name, location, size, protected, deleted, err)
		}
		if err := upgraded.DB.QueryRow(`SELECT value FROM settings WHERE key='backup_retention'`).Scan(&setting); err != nil || setting != "25" {
			t.Fatalf("migration reset retention: %q %v", setting, err)
		}
		upgraded.Close()
	}
}
