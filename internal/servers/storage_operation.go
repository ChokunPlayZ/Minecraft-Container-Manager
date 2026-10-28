package servers

import (
	"context"
	"fmt"
	"github.com/docker/docker/errdefs"
)

func (s *Store) removeContainer(ctx context.Context, id string) error {
	if isDockerNil(s.docker) {
		return fmt.Errorf("container runtime unavailable")
	}
	if err := s.docker.Remove(ctx, id); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// WithStoppedData serializes backups/restores with lifecycle, world edits and
// modpack installs. Stopping flushes the world before taking a snapshot.
func (s *Store) WithStoppedData(ctx context.Context, id string, restore bool, operation func(context.Context) error) error {
	mu := s.lifecycleLock(id)
	mu.Lock()
	defer mu.Unlock()
	unlock := s.lockWorld(id, true)
	defer func() { unlock() }()
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	wasRunning := srv.State == StateRunning || srv.State == StateStarting || srv.State == StateBuilding || srv.State == StateInstalling
	if task := s.GetTaskProgress(id); task != nil && task.Stage != "completed" && task.Stage != "failed" {
		return ErrWorldBusy
	}
	if _, err = s.stopLocked(ctx, id); err != nil {
		return err
	}
	srv, err = s.Get(ctx, id)
	if err != nil {
		return err
	}
	if !s.nbtStopped(ctx, srv) {
		return ErrWorldBusy
	}
	s.fileEditMu.Lock()
	err = operation(ctx)
	s.fileEditMu.Unlock()
	// A backup resumes an originally running server. Restore deliberately remains stopped.
	if !restore && wasRunning {
		unlock()
		unlock = func() {}
		// The exclusive data lock must be released before startLocked takes its read lock.
		_, startErr := s.startLocked(ctx, id)
		if err == nil {
			err = startErr
		}
	}
	return err
}
