package servers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/notifications"
)

// SetNotifier must be called before background monitors or requests start.
func (s *Store) SetNotifier(n notifications.Notifier) { s.notifier = n }

func (s *Store) lifecycleLock(id string) *sync.Mutex {
	mu, _ := s.lifecycleLocks.LoadOrStore(id, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// changeState compares observations transactionally so concurrent status reads
// cannot emit duplicate notifications or overwrite a newer lifecycle operation.
func (s *Store) changeState(ctx context.Context, id, state string, startedAt *string, expected *string, kind, message string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var previous, name string
	err = tx.QueryRowContext(ctx, `SELECT state,name FROM servers WHERE id=?`, id).Scan(&previous, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if expected != nil && previous != *expected {
		return false, nil
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `UPDATE servers SET state=?,started_at=?,updated_at=? WHERE id=?`, state, startedAt, now.Format(time.RFC3339), id)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if previous == state || s.notifier == nil {
		return previous != state, nil
	}
	if kind == "" {
		switch state {
		case StateRunning:
			kind, message = notifications.ServerStarted, "The server container has started."
		case StateStopped:
			if previous == StateRunning || previous == StateStarting || previous == StateStopping {
				kind, message = notifications.ServerStopped, "The server has stopped."
			}
		case StateError:
			kind, message = notifications.ServerError, "A server operation failed. Check the server console and panel logs for details."
		}
	}
	if kind != "" {
		s.notifier.Notify(notifications.Event{Kind: kind, ServerID: id, ServerName: name, Message: message, Time: now})
	}
	return true, nil
}

// RunStatusMonitor detects exits even when no browser is viewing the panel.
// Inspect failures leave the last known state intact; they are not crashes.
func (s *Store) RunStatusMonitor(ctx context.Context, logger *log.Logger) {
	if isDockerNil(s.docker) {
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.reconcileStatuses(ctx); err != nil && ctx.Err() == nil && logger != nil {
			logger.Printf("server status monitor: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) reconcileStatuses(ctx context.Context) error {
	servers, err := s.List(ctx)
	if err != nil {
		return err
	}
	for _, srv := range servers {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if srv.ContainerID == "" {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := s.Status(checkCtx, srv.ID)
		cancel()
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}
