package spindown

import (
	"context"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

// effectiveTimeout resolves the timeout for a server: its per-server override
// wins, otherwise the site-wide default (re-read each tick) is used.
func (s *Service) effectiveTimeout(ctx context.Context, overrideMin *int) time.Duration {
	if overrideMin != nil && *overrideMin > 0 {
		return time.Duration(*overrideMin) * time.Minute
	}
	return s.effectiveDefault(ctx)
}

// tick evaluates every server and stops those that have been idle past their
// effective timeout. Servers with no recorded activity yet are seeded with the
// current time so they are not stopped immediately after startup.
func (s *Service) tick(ctx context.Context) {
	if s.disabled {
		return
	}
	list, err := s.spin.List(ctx)
	if err != nil {
		s.log.Printf("spindown: list servers: %v", err)
		return
	}

	now := s.now()
	for _, srv := range list {
		if srv.State != servers.StateRunning && srv.State != servers.StateStarting {
			continue
		}
		s.evaluate(ctx, srv, now)
	}
}

func (s *Service) evaluate(ctx context.Context, srv servers.Server, now time.Time) {
	override, ok, err := s.spin.IdleTimeoutOverride(ctx, srv.ID)
	if err != nil {
		s.log.Printf("spindown: override for %s: %v", srv.ID, err)
		return
	}
	var overrideMin *int
	if ok {
		overrideMin = &override
	}
	timeout := s.effectiveTimeout(ctx, overrideMin)

	activity, err := s.spin.LastActivity(ctx, srv.ID)
	if err != nil {
		if err == servers.ErrNotFound {
			return
		}
		s.log.Printf("spindown: activity for %s: %v", srv.ID, err)
		return
	}

	s.mu.Lock()
	lastSeen, seen := s.lastSeen[srv.ID]
	s.mu.Unlock()

	// Seed the reference time so a freshly started server gets a grace period
	// rather than being stopped on the first tick.
	reference := activity
	if reference.IsZero() {
		reference = now
	}
	if !seen {
		s.mu.Lock()
		s.lastSeen[srv.ID] = reference
		s.mu.Unlock()
		_ = s.spin.SetActivity(ctx, srv.ID, reference)
		return
	}

	_ = lastSeen
	idle := now.Sub(reference)
	if idle < timeout {
		return
	}
	s.spinDown(ctx, srv, timeout)
}

// spinDown stops a server, guarding against repeated concurrent attempts via a
// short per-server cooldown.
func (s *Service) spinDown(ctx context.Context, srv servers.Server, timeout time.Duration) {
	s.mu.Lock()
	if cd, ok := s.cooldown[srv.ID]; ok && s.now().Sub(cd) < 10*time.Second {
		s.mu.Unlock()
		return
	}
	s.cooldown[srv.ID] = s.now()
	s.mu.Unlock()

	s.log.Printf("spindown: stopping %s idle for %s (timeout %s)", srv.ID, s.now().Sub(activityRef(ctx, s.spin, srv)).Round(time.Second), timeout.Round(time.Second))
	if _, err := s.ctrl.Stop(ctx, srv.ID); err != nil {
		s.log.Printf("spindown: stop %s: %v", srv.ID, err)
		return
	}
	// Stop this tick's loop from re-evaluating a state that may lag.
	_ = s.spin.SetActivity(ctx, srv.ID, s.now())
}

// activityRef resolves the server's last activity time, defaulting to now when
// none is recorded. Used only for richer log messages.
func activityRef(ctx context.Context, spin SpinStore, srv servers.Server) time.Time {
	t, err := spin.LastActivity(ctx, srv.ID)
	if err != nil || t.IsZero() {
		return time.Now()
	}
	return t
}

// Wake restarts a server if it is currently stopped, and seeds last-activity
// so the freshly woken server is not immediately spun down. It also refreshes
// the lastSeen guard.
func (s *Service) Wake(ctx context.Context, id string) (servers.Server, error) {
	srv, err := s.ctrl.Start(ctx, id)
	if err != nil {
		return servers.Server{}, err
	}
	now := s.now()
	if serr := s.spin.SetActivity(ctx, id, now); serr != nil {
		s.log.Printf("spindown: seed activity for %s after wake: %v", id, serr)
	}
	s.mu.Lock()
	s.lastSeen[id] = now
	s.mu.Unlock()
	return srv, nil
}
