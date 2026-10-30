package backups

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"
)

var ErrInvalidPolicy = errors.New("invalid retention policy")
var ErrBackupConflict = errors.New("backup cannot be changed")

// Policy limits completed, unprotected backups. Zero disables a particular limit.
// Protected backups are additional to KeepCount. The newest successful backup
// is always retained, even when it exceeds MaxAgeDays.
type Policy struct {
	KeepCount          int `json:"keep_count"`
	MaxAgeDays         int `json:"max_age_days"`
	TrashRetentionDays int `json:"trash_retention_days"`
}

func (p Policy) Validate() error {
	if p.KeepCount < 0 || p.KeepCount > 100000 || p.MaxAgeDays < 0 || p.MaxAgeDays > 36500 || p.TrashRetentionDays < 0 || p.TrashRetentionDays > 36500 {
		return fmt.Errorf("%w: keep count must be 0–100000; age and trash days must be 0–36500", ErrInvalidPolicy)
	}
	return nil
}

func (s *Store) GetPolicy(ctx context.Context, serverID string) (Policy, error) {
	var p Policy
	err := s.db.QueryRowContext(ctx, `SELECT keep_count, max_age_days, trash_retention_days FROM backup_policies WHERE server_id=?`, serverID).
		Scan(&p.KeepCount, &p.MaxAgeDays, &p.TrashRetentionDays)
	if errors.Is(err, sql.ErrNoRows) {
		p.KeepCount = max(0, min(s.Retention(ctx), 100000))
		p.TrashRetentionDays = 30
		return p, nil
	}
	return p, err
}

// SavePolicy does not move or delete archives. Cleanup is a separate action.
func (s *Store) SavePolicy(ctx context.Context, serverID string, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	release, err := s.lockOperation(ctx, serverID)
	if err != nil {
		return err
	}
	defer release()
	_, err = s.db.ExecContext(ctx, `INSERT INTO backup_policies(server_id,keep_count,max_age_days,trash_retention_days) VALUES(?,?,?,?)
		ON CONFLICT(server_id) DO UPDATE SET keep_count=excluded.keep_count,max_age_days=excluded.max_age_days,trash_retention_days=excluded.trash_retention_days`,
		serverID, p.KeepCount, p.MaxAgeDays, p.TrashRetentionDays)
	return err
}

func (s *Store) ListTrash(ctx context.Context, serverID string) ([]Backup, error) {
	return s.list(ctx, serverID, true)
}

type RetentionCandidate struct {
	Backup
	Reason string `json:"reason"`
}

type RetentionPreview struct {
	Policy     Policy               `json:"policy"`
	ToTrash    []RetentionCandidate `json:"to_trash"`
	ToPurge    []Backup             `json:"to_purge"`
	TrashBytes int64                `json:"trash_bytes"`
	PurgeBytes int64                `json:"purge_bytes"`
}

func (s *Store) PreviewRetention(ctx context.Context, serverID string) (RetentionPreview, error) {
	p, err := s.GetPolicy(ctx, serverID)
	if err != nil {
		return RetentionPreview{}, err
	}
	active, err := s.List(ctx, serverID)
	if err != nil {
		return RetentionPreview{}, err
	}
	trash, err := s.ListTrash(ctx, serverID)
	if err != nil {
		return RetentionPreview{}, err
	}
	return planRetention(p, active, trash, time.Now().UTC()), nil
}

func planRetention(p Policy, active, trash []Backup, now time.Time) RetentionPreview {
	plan := RetentionPreview{Policy: p, ToTrash: []RetentionCandidate{}, ToPurge: []Backup{}}
	completed, unprotected := 0, 0
	for _, b := range active {
		if b.Status != StatusCompleted {
			continue
		}
		completed++
		if b.Protected {
			continue
		}
		unprotected++
		// Always leave the newest successful snapshot available for recovery.
		if completed == 1 {
			continue
		}
		reason := ""
		if p.KeepCount > 0 && unprotected > p.KeepCount {
			reason = "count"
		}
		created, err := time.Parse(time.RFC3339, b.CreatedAt)
		if p.MaxAgeDays > 0 && err == nil && !created.After(now.Add(-time.Duration(p.MaxAgeDays)*24*time.Hour)) {
			reason = "age"
		}
		if reason != "" {
			plan.ToTrash = append(plan.ToTrash, RetentionCandidate{Backup: b, Reason: reason})
			plan.TrashBytes += b.SizeBytes
		}
	}
	for _, b := range trash {
		deleted, err := time.Parse(time.RFC3339, b.DeletedAt)
		if p.TrashRetentionDays > 0 && !b.Protected && err == nil && !deleted.After(now.Add(-time.Duration(p.TrashRetentionDays)*24*time.Hour)) {
			plan.ToPurge = append(plan.ToPurge, b)
			plan.PurgeBytes += b.SizeBytes
		}
	}
	return plan
}

// changeBackup serializes metadata changes with backup creation, restore and cleanup.
func (s *Store) changeBackup(ctx context.Context, id string, change func(Backup) error) error {
	b, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	release, err := s.lockOperation(ctx, b.ServerID)
	if err != nil {
		return err
	}
	defer release()
	b, err = s.Get(ctx, id)
	if err != nil {
		return err
	}
	return change(b)
}

func (s *Store) Trash(ctx context.Context, id string) error {
	return s.changeBackup(ctx, id, func(b Backup) error { return s.trashLocked(ctx, b) })
}

func (s *Store) trashLocked(ctx context.Context, b Backup) error {
	if b.Protected || b.Status == StatusPending {
		return fmt.Errorf("%w: unprotect the backup or wait for it to finish before moving it to trash", ErrBackupConflict)
	}
	if b.DeletedAt != "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE backups SET deleted_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), b.ID)
	return err
}

// Recover protects successful snapshots so automatic retention cannot immediately
// trash the recovered backup again. It does not change the live server world.
func (s *Store) Recover(ctx context.Context, id string) error {
	return s.changeBackup(ctx, id, func(b Backup) error {
		if b.DeletedAt == "" {
			return fmt.Errorf("%w: backup is not in trash", ErrBackupConflict)
		}
		_, err := s.db.ExecContext(ctx, `UPDATE backups SET deleted_at='', protected=(status='completed') WHERE id=?`, id)
		return err
	})
}

func (s *Store) Protect(ctx context.Context, id string, protected bool) error {
	return s.changeBackup(ctx, id, func(b Backup) error {
		if b.DeletedAt != "" || b.Status != StatusCompleted {
			return fmt.Errorf("%w: only active completed backups can be protected", ErrBackupConflict)
		}
		_, err := s.db.ExecContext(ctx, `UPDATE backups SET protected=? WHERE id=?`, protected, id)
		return err
	})
}

func (s *Store) Purge(ctx context.Context, id string) error {
	return s.changeBackup(ctx, id, func(b Backup) error {
		if b.DeletedAt == "" || b.Protected {
			return fmt.Errorf("%w: only unprotected backups in trash can be permanently deleted", ErrBackupConflict)
		}
		return s.deleteLocked(ctx, b)
	})
}

type CleanupResult struct {
	Trashed int `json:"trashed"`
	Purged  int `json:"purged"`
}

func (s *Store) Cleanup(ctx context.Context, serverID string, emptyTrash bool) (CleanupResult, error) {
	release, err := s.lockOperation(ctx, serverID)
	if err != nil {
		return CleanupResult{}, err
	}
	defer release()
	return s.cleanupLocked(ctx, serverID, emptyTrash)
}

func (s *Store) cleanupLocked(ctx context.Context, serverID string, emptyTrash bool) (CleanupResult, error) {
	var result CleanupResult
	plan, err := s.PreviewRetention(ctx, serverID)
	if err != nil {
		return result, err
	}
	if emptyTrash {
		plan.ToPurge, err = s.ListTrash(ctx, serverID)
		if err != nil {
			return result, err
		}
		// Emptying trash never changes the active backup list.
		plan.ToTrash = nil
	}
	var failures []error
	for _, b := range plan.ToPurge {
		if b.Protected {
			continue
		}
		if err := s.deleteLocked(ctx, b); err != nil {
			failures = append(failures, fmt.Errorf("purge %s: %w", b.Name, err))
			continue
		}
		result.Purged++
	}
	for _, candidate := range plan.ToTrash {
		if err := s.trashLocked(ctx, candidate.Backup); err != nil {
			failures = append(failures, err)
			continue
		}
		result.Trashed++
	}
	return result, errors.Join(failures...)
}

// RunRetention expires trash and applies age limits even on servers with no
// scheduled backups. Each server has its own lock and failure does not stop others.
func (s *Store) RunRetention(ctx context.Context, logger *log.Logger) {
	run := func() {
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT server_id FROM backups WHERE server_id IS NOT NULL AND server_id != ''`)
		if err != nil {
			if logger != nil {
				logger.Printf("backup retention: %v", err)
			}
			return
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close() // SQLite has a single connection; release it before cleanup.
		if err != nil {
			if logger != nil {
				logger.Printf("backup retention: %v", err)
			}
			return
		}
		for _, id := range ids {
			work, cancel := context.WithTimeout(ctx, 5*time.Minute)
			_, err := s.Cleanup(work, id, false)
			cancel()
			if err != nil && logger != nil {
				logger.Printf("backup retention server=%s: %v", id, err)
			}
			if ctx.Err() != nil {
				return
			}
		}
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
