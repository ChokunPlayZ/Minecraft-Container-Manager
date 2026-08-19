// Package dns is a typed placeholder for publishing Cloudflare SRV records that
// point a domain at running MCM servers. No runtime logic is implemented yet.
package dns

import "context"

// Publisher manages SRV records for server addresses.
//
// TODO: implement Cloudflare DNS API calls to create/update/delete SRV records
// as servers start and stop.
type Publisher interface {
	// Upsert creates or updates the SRV record for a server.
	Upsert(ctx context.Context, serverID, host string, port int) error
	// Remove deletes the SRV record for a server.
	Remove(ctx context.Context, serverID string) error
}

// Service is a no-op implementation of Publisher.
type Service struct{}

var _ Publisher = (*Service)(nil)

// Upsert is a typed stub.
func (s *Service) Upsert(ctx context.Context, serverID, host string, port int) error {
	return nil
}

// Remove is a typed stub.
func (s *Service) Remove(ctx context.Context, serverID string) error {
	return nil
}
