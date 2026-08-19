package backups

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"
)

// Scheduler periodically triggers backups for servers that have scheduling
// enabled. It runs one poll tick at the configured interval and fires backups
// for servers whose last backup is older than their configured interval.
type Scheduler struct {
	store *Store
	db    *sql.DB
	log   *log.Logger
	every time.Duration

	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	last   map[string]time.Time
}

// NewScheduler builds a backup scheduler. every is the poll cadence; a zero or
// negative value disables automatic scheduling.
func NewScheduler(store *Store, db *sql.DB, logger *log.Logger, every time.Duration) *Scheduler {
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		store: store,
		db:    db,
		log:   logger,
		every: every,
		last:  map[string]time.Time{},
	}
}

// Start launches the scheduler loop in a background goroutine.
func (s *Scheduler) Start() {
	if s.every <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.loop(ctx)
}

// Stop terminates the scheduler loop and waits for it to exit.
func (s *Scheduler) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)
	// Run an initial pass shortly after startup.
	s.tick(ctx)
	ticker := time.NewTicker(s.every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Scheduler) tick(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, backup_enabled, backup_interval_minutes FROM servers WHERE backup_enabled = 1`)
	if err != nil {
		s.log.Printf("backup scheduler: query servers: %v", err)
		return
	}
	defer rows.Close()

	type srv struct {
		id       string
		interval int
	}
	var servers []srv
	for rows.Next() {
		var id string
		var enabled, interval int
		if err := rows.Scan(&id, &enabled, &interval); err != nil {
			s.log.Printf("backup scheduler: scan server: %v", err)
			return
		}
		servers = append(servers, srv{id: id, interval: interval})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, sr := range servers {
		interval := time.Duration(sr.interval) * time.Minute
		if interval <= 0 {
			interval = 12 * time.Hour
		}
		last, ok := s.last[sr.id]
		if !ok {
			// Never tracked in this process; record it so the first backup is
			// not fired immediately on startup unless truly overdue.
			s.last[sr.id] = now
			continue
		}
		if now.Sub(last) < interval {
			continue
		}
		s.last[sr.id] = now
		go func(ctx context.Context, id string) {
			if _, err := s.store.Backup(ctx, id, ""); err != nil {
				s.log.Printf("backup scheduler: server %s: %v", id, err)
			}
		}(context.WithoutCancel(ctx), sr.id)
	}
}
