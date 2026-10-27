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
