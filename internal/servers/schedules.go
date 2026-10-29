package servers

import (
	"context"
	"fmt"

	"github.com/mcm-panel/mcm/internal/schedules"
)

// ExecuteScheduledTask shares lifecycle and console behavior with manual actions.
func (s *Store) ExecuteScheduledTask(ctx context.Context, task schedules.Task) error {
	var err error
	switch task.Action {
	case "command":
		err = s.SendConsoleCommand(ctx, task.ServerID, task.Command)
	case "start":
		_, err = s.Start(ctx, task.ServerID)
	case "stop":
		_, err = s.Stop(ctx, task.ServerID)
	case "restart":
		_, err = s.Restart(ctx, task.ServerID)
	default:
		err = fmt.Errorf("unsupported scheduled action %q", task.Action)
	}
	return err
}

// New servers retain the existing default of one automatic backup every 12 hours.
func (s *Store) createBackupSchedule(ctx context.Context, serverID string) error {
	_, err := schedules.NewStore(s.db).Create(ctx, serverID, schedules.Input{
		Name: "Automatic backup", Action: "backup", Cron: "@every 720m", Enabled: true,
	})
	return err
}

// Copy backup configuration without copying other server automation or run history.
func (s *Store) copyBackupSchedules(ctx context.Context, sourceID, serverID string) error {
	store := schedules.NewStore(s.db)
	tasks, err := store.List(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Action != "backup" {
			continue
		}
		if _, err := store.Create(ctx, serverID, schedules.Input{
			Name: task.Name, Action: task.Action, Cron: task.Cron,
			Timezone: task.Timezone, Enabled: task.Enabled, BackupTarget: task.BackupTarget,
		}); err != nil {
			return err
		}
	}
	return nil
}
