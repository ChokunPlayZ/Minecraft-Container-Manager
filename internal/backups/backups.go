// Package backups is a typed placeholder for S3-based world backup management.
// No runtime logic is implemented yet.
package backups

import "context"

// Store backs up and restores server worlds to object storage (e.g. S3).
//
// TODO: implement S3 upload/download, retention, and restore when the backup
// feature ships.
type Store interface {
	// Backup snapshots a server's world to remote storage.
	Backup(ctx context.Context, serverID string) error
	// Restore restores a world snapshot from remote storage.
	Restore(ctx context.Context, serverID, snapshot string) error
}

// Service is a no-op implementation of Store.
type Service struct{}

var _ Store = (*Service)(nil)

// Backup is a typed stub.
func (s *Service) Backup(ctx context.Context, serverID string) error {
	return nil
}

// Restore is a typed stub.
func (s *Service) Restore(ctx context.Context, serverID, snapshot string) error {
	return nil
}
