package backups

import (
	"testing"

	"github.com/mcm-panel/mcm/internal/notifications"
)

type backupEventRecorder struct{ events []notifications.Event }

func (r *backupEventRecorder) Notify(e notifications.Event) { r.events = append(r.events, e) }

func TestBackupNotifications(t *testing.T) {
	s, _ := openTestStore(t)
	r := &backupEventRecorder{}
	s.SetNotifier(r)
	for _, p := range []BackupProgress{
		{ID: "one", Operation: "backup", Stage: "scanning"},
		{ID: "one", Operation: "backup", Stage: "completed", Message: "Backup created successfully"},
		{ID: "one", Operation: "backup", Stage: "completed"},
		{ID: "two", Operation: "backup", Stage: "failed", Message: "Backup failed"},
		{ID: "three", Operation: "restore", Stage: "completed"},
	} {
		s.SetProgress("server", p)
	}
	if len(r.events) != 2 || r.events[0].Kind != notifications.BackupCompleted || r.events[1].Kind != notifications.BackupFailed || r.events[0].ServerName != "server" {
		t.Fatalf("events: %+v", r.events)
	}
}
