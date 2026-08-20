// Package spindown stops idle servers after a configurable period of
// inactivity and wakes them back up when a player reconnects. It tracks the
// last player activity per server, applies a global inactivity timeout with
// per-server overrides, and schedules periodic evaluation.
package spindown

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

// Policy decides when idle servers should be stopped. It is the public surface
// kept for compatibility and unit testing.
type Policy interface {
	// ShouldStop reports whether a server that has been idle for idleForSeconds
	// seconds should be spun down.
	ShouldStop(ctx context.Context, serverID string, idleForSeconds int) bool
}

// SpinStore reads and writes the per-server activity state and server list used
// by the scheduler. *servers.Store satisfies it.
type SpinStore interface {
	List(ctx context.Context) ([]servers.Server, error)
	LastActivity(ctx context.Context, id string) (time.Time, error)
	SetActivity(ctx context.Context, id string, t time.Time) error
	IdleTimeoutOverride(ctx context.Context, id string) (minutes int, ok bool, err error)
	DefaultIdleTimeout(ctx context.Context, fallback time.Duration) (time.Duration, error)
}

// Control starts and stops servers. *servers.Store satisfies it.
type Control interface {
	Stop(ctx context.Context, id string) (servers.Server, error)
	Start(ctx context.Context, id string) (servers.Server, error)
}

// Enter is a constructor alias kept so callers can read the wiring intent; the
// canonical constructor is New.
//
// Deprecated: use New.
func Enter(spin SpinStore, ctrl Control, logger *log.Logger, defaultTimeout time.Duration) *Service {
	return New(spin, ctrl, logger, defaultTimeout)
}

// Service orchestrates idle spin-down and join wake.
type Service struct {
	spin SpinStore
	ctrl Control
	log  *log.Logger

	defaultTimeout time.Duration
	every          time.Duration
	now            func() time.Time
	disabled       bool

	mu       sync.Mutex
	cooldown map[string]time.Time
	lastSeen map[string]time.Time

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

var _ Policy = (*Service)(nil)

// New builds a spin-down Service. defaultTimeout is the fallback inactivity
// timeout used when no site-wide setting or per-server override is configured.
// every is the scheduler cadence; a zero or negative value disables the ticker
// (manual Wake/Stop still work).
func New(spin SpinStore, ctrl Control, logger *log.Logger, defaultTimeout time.Duration) *Service {
	if logger == nil {
		logger = log.Default()
	}
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Minute
	}
	return &Service{
		spin:           spin,
		ctrl:           ctrl,
		log:            logger,
		defaultTimeout: defaultTimeout,
		every:          time.Minute,
		now:            time.Now,
		cooldown:       map[string]time.Time{},
		lastSeen:       map[string]time.Time{},
	}
}

// SetInterval overrides the scheduler cadence. Zero or negative disables the
// ticker. Intended for tests and customization.
func (s *Service) SetInterval(every time.Duration) *Service {
	s.every = every
	return s
}

// SetClock replaces the time source, useful for deterministic tests.
func (s *Service) SetClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// SetDisabled toggles the automatic scheduler. Running servers are never
// automatically stopped while disabled; manual Wake/Stop calls still work.
func (s *Service) SetDisabled(disabled bool) {
	s.disabled = disabled
}

// Start launches the scheduler loop in a background goroutine.
func (s *Service) Start() {
	if s.every <= 0 || s.disabled {
		return
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.done = make(chan struct{})
	go s.loop(s.ctx)
}

// Stop terminates the scheduler loop and waits for it to finish.
func (s *Service) Stop() {
	if s.cancel == nil {
		return
	}
	s.cancel()
	<-s.done
	s.cancel = nil
}

func (s *Service) loop(ctx context.Context) {
	defer close(s.done)
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

// ShouldStop implements Policy. It uses the per-server override when present,
// otherwise the effective default as resolved at construct time (site-wide
// settings are re-read by the scheduler each tick via effectiveTimeout).
func (s *Service) ShouldStop(ctx context.Context, serverID string, idleForSeconds int) bool {
	override, ok, err := s.spin.IdleTimeoutOverride(ctx, serverID)
	if err != nil {
		return false
	}
	var timeout time.Duration
	if ok && override > 0 {
		timeout = time.Duration(override) * time.Minute
	} else {
		timeout = s.effectiveDefault(ctx)
	}
	return time.Duration(idleForSeconds)*time.Second >= timeout
}

// DefaultTimeout reports the effective default inactivity timeout.
func (s *Service) DefaultTimeout() time.Duration {
	return s.defaultTimeout
}

// HandleJoin records player activity for a server and, when the server is
// stopped, wakes it back up. It is the join-wake hook: a reconnecting player
// restarts an idle-stopped server and resets its idle clock. It is defensive
// and best-effort for unknown or transient states.
func (s *Service) HandleJoin(ctx context.Context, id string) error {
	now := s.now()
	list, err := s.spin.List(ctx)
	if err == nil {
		for _, srv := range list {
			if srv.ID == id {
				if srv.State == servers.StateStopped || srv.State == servers.StateStopping || srv.State == servers.StateError {
					_, werr := s.Wake(ctx, id)
					return werr
				}
				break
			}
		}
	}
	// Running/starting (or unknown): just refresh the idle clock defensively.
	if err := s.spin.SetActivity(ctx, id, now); err != nil {
		return err
	}
	s.mu.Lock()
	s.lastSeen[id] = now
	s.mu.Unlock()
	return nil
}

func (s *Service) effectiveDefault(ctx context.Context) time.Duration {
	d, err := s.spin.DefaultIdleTimeout(ctx, s.defaultTimeout)
	if err != nil {
		return s.defaultTimeout
	}
	return d
}

// ServerStatus describes the idle spin-down state of a single server.
type ServerStatus struct {
	ID               string    `json:"id"`
	State            string    `json:"state"`
	LastActivity     time.Time `json:"last_activity"`
	IdleTimeoutMin   int       `json:"idle_timeout_minutes"`
	IdleOverrideMin  *int      `json:"idle_override_minutes,omitempty"`
	SpinDownDisabled bool      `json:"spin_down_disabled"`
}

// Status snapshots idle spin-down state for every server, re-reading the
// effective timeout so callers can render current per-server settings.
func (s *Service) Status(ctx context.Context) ([]ServerStatus, error) {
	list, err := s.spin.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ServerStatus, 0, len(list))
	for _, srv := range list {
		status := ServerStatus{
			ID:               srv.ID,
			State:            srv.State,
			SpinDownDisabled: srv.SpinDownDisabled,
		}
		override, ok, oerr := s.spin.IdleTimeoutOverride(ctx, srv.ID)
		if oerr != nil {
			return nil, oerr
		}
		var overrideMin *int
		if ok {
			overrideMin = &override
		}
		activity, aerr := s.spin.LastActivity(ctx, srv.ID)
		if aerr != nil {
			activity = time.Time{}
		}
		status.LastActivity = activity
		status.IdleTimeoutMin = int(s.effectiveTimeout(ctx, overrideMin) / time.Minute)
		status.IdleOverrideMin = overrideMin
		out = append(out, status)
	}
	return out, nil
}
