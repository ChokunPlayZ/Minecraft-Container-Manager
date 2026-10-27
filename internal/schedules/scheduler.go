package schedules

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Execute uses the same server actions as interactive API requests.
type Execute func(context.Context, Task) error

type Scheduler struct {
	store   *Store
	execute Execute
	logger  *log.Logger
	cancel  context.CancelFunc
	done    chan struct{}
	workers sync.WaitGroup
	mu      sync.Mutex
	running map[string]bool
}

func NewScheduler(store *Store, execute Execute, logger *log.Logger) *Scheduler {
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{store: store, execute: execute, logger: logger, running: make(map[string]bool)}
}

func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		if _, err := s.store.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_status='failed', last_error='Interrupted when the panel stopped.' WHERE last_status='running'`); err != nil {
			s.logger.Printf("task scheduler: recover interrupted tasks: %v", err)
		}
		// Do not replay missed restarts or commands after panel downtime.
		s.tick(ctx, time.Now(), true)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.tick(ctx, now, false)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.workers.Wait()
}

func (s *Scheduler) tick(ctx context.Context, now time.Time, startup bool) {
	tasks, err := s.store.query(ctx, `SELECT `+columns+` FROM scheduled_tasks WHERE enabled=1 AND next_run_at != '' AND next_run_at <= ? ORDER BY next_run_at, id`, stamp(now))
	if err != nil {
		s.logger.Printf("task scheduler: list due: %v", err)
		return
	}
	for _, task := range tasks {
		schedule, err := Parse(task.Cron, task.Timezone)
		if err != nil {
			s.logger.Printf("task scheduler: parse %s: %v", task.ID, err)
			continue
		}
		next := schedule.Next(now)
		if next.IsZero() {
			continue
		}
		s.mu.Lock()
		busy := s.running[task.ID]
		// Claim by advancing the persisted due time before dispatch. The compare
		// also prevents a second panel process from dispatching the same run.
		result, err := s.store.db.ExecContext(ctx, `UPDATE scheduled_tasks SET next_run_at=? WHERE id=? AND enabled=1 AND next_run_at=? AND updated_at=?`, stamp(next), task.ID, task.NextRunAt, task.UpdatedAt)
		if err != nil {
			s.mu.Unlock()
			s.logger.Printf("task scheduler: claim %s: %v", task.ID, err)
			continue
		}
		claimed, err := result.RowsAffected()
		if err != nil || claimed == 0 || busy {
			s.mu.Unlock()
			continue
		}
		if startup {
			s.mu.Unlock()
			_, err := s.store.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at=?, last_status='skipped', last_error='Missed while the panel was offline; waiting for the next run.' WHERE id=?`, task.NextRunAt, task.ID)
			if err != nil {
				s.logger.Printf("task scheduler: skip %s: %v", task.ID, err)
			}
			continue
		}
		s.running[task.ID] = true
		s.mu.Unlock()
		_, err = s.store.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at=?, last_status='running', last_error='' WHERE id=?`, stamp(now), task.ID)
		if err != nil {
			s.mu.Lock()
			delete(s.running, task.ID)
			s.mu.Unlock()
			s.logger.Printf("task scheduler: record start %s: %v", task.ID, err)
			continue
		}
		s.workers.Add(1)
		go s.run(ctx, task)
	}
}

func (s *Scheduler) run(parent context.Context, task Task) {
	defer s.workers.Done()
	defer func() { s.mu.Lock(); delete(s.running, task.ID); s.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	// A single failed task must not terminate the scheduler or the panel.
	err := func() (err error) {
		defer func() {
			if value := recover(); value != nil {
				err = fmt.Errorf("task panicked: %v", value)
			}
		}()
		return s.execute(ctx, task)
	}()
	status, detail := "success", ""
	if err != nil {
		status, detail = "failed", err.Error()
		s.logger.Printf("task scheduler: task %s server %s: %v", task.ID, task.ServerID, err)
	}
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()
	if _, err := s.store.db.ExecContext(writeCtx, `UPDATE scheduled_tasks SET last_status=?, last_error=? WHERE id=?`, status, detail, task.ID); err != nil {
		s.logger.Printf("task scheduler: record result %s: %v", task.ID, err)
	}
}
