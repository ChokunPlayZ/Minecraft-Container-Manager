// Package spindown is a typed placeholder for idle spin-down, which stops unused
// servers after a configurable period of inactivity. No runtime logic is
// implemented yet.
package spindown

import "context"

// Policy decides when idle servers should be stopped.
//
// TODO: implement last-activity tracking and a scheduler that stops idle
// servers, with per-server overrides.
type Policy interface {
	// ShouldStop reports whether a server should be spun down.
	ShouldStop(ctx context.Context, serverID string, idleForSeconds int) bool
}

// Service is a no-op implementation of Policy.
type Service struct{}

var _ Policy = (*Service)(nil)

// ShouldStop is a typed stub.
func (s *Service) ShouldStop(ctx context.Context, serverID string, idleForSeconds int) bool {
	return false
}
