// Package backups implements S3-compatible world backup and restore for MCM.
// Each server's world data is archived to a tar.gz and stored in a
// path-style S3-compatible object store (MinIO, AWS S3, etc).
package backups

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status values for backup records.
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// ErrNotFound is returned when a backup record does not exist.
var ErrNotFound = errors.New("backup not found")

// ErrNotConfigured is returned when no S3 endpoint is configured.
var ErrNotConfigured = errors.New("S3 backup storage is not configured")

// Backup is a remote world snapshot record.
type Backup struct {
	ID        string `json:"id"`
	ServerID  string `json:"server_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Location  string `json:"location"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// Store coordinates backup records in SQLite and object movement to S3.
type Store struct {
	db      *sql.DB
	client  *s3Client
	dataDir string
}

// S3Config configures the remote object store.
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
}

// New constructs a backup Store. When endpoint is empty the returned store
// records metadata but returns ErrNotConfigured on actual upload/download.
func New(handle *sql.DB, cfg S3Config, dataDir string) *Store {
	s := &Store{db: handle, dataDir: dataDir}
	if cfg.Endpoint != "" && cfg.Bucket != "" {
		s.client = newS3Client(cfg)
	}
	return s
}

// serverDataDir returns the per-server data directory.
func (s *Store) serverDataDir(serverID string) string {
	return filepath.Join(s.dataDir, "servers", serverID)
}

func (s *Store) objectKey(backupID, serverID string) string {
	return fmt.Sprintf("backups/%s/%s.tar.gz", serverID, backupID)
}

// List returns all backups for a server, newest first.
func (s *Store) List(ctx context.Context, serverID string) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_id, name, COALESCE(size_bytes,0), COALESCE(location,''), status, created_at FROM backups WHERE server_id = ? ORDER BY created_at DESC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.ServerID, &b.Name, &b.SizeBytes, &b.Location, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Get returns a single backup record by id.
func (s *Store) Get(ctx context.Context, id string) (Backup, error) {
	var b Backup
	err := s.db.QueryRowContext(ctx,
		`SELECT id, server_id, name, COALESCE(size_bytes,0), COALESCE(location,''), status, created_at FROM backups WHERE id = ?`, id).
		Scan(&b.ID, &b.ServerID, &b.Name, &b.SizeBytes, &b.Location, &b.Status, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Backup{}, ErrNotFound
	}
	if err != nil {
		return Backup{}, err
	}
	return b, nil
}

// Backup archives a server's world directory and uploads it to the object
// store, then records the result in the database.
func (s *Store) Backup(ctx context.Context, serverID, name string) (*Backup, error) {
	if s.client == nil {
		return nil, ErrNotConfigured
	}
	if name == "" {
		name = time.Now().UTC().Format("2006-01-02T15-04-05")
	}
	id := uuid.NewString()
	location := s.objectKey(id, serverID)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, 0, ?, ?, ?)`,
		id, serverID, name, location, StatusPending, now); err != nil {
		return nil, fmt.Errorf("insert backup record: %w", err)
	}

	backup := &Backup{ID: id, ServerID: serverID, Name: name, Location: location, Status: StatusPending, CreatedAt: now}

	// Archive the current world into a temporary file.
	archivePath, err := s.archiveWorld(ctx, serverID)
	if err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		return backup, err
	}
	defer os.Remove(archivePath)

	stat, err := os.Stat(archivePath)
	if err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		return backup, err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		return backup, err
	}
	defer f.Close()

	if err := s.client.putObject(ctx, location, f); err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		return backup, err
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE backups SET size_bytes=?, status=? WHERE id=?`, stat.Size(), StatusCompleted, id); err != nil {
		return backup, err
	}
	backup.SizeBytes = stat.Size()
	backup.Status = StatusCompleted

	if err := s.enforceRetention(ctx, serverID); err != nil {
		return backup, err
	}
	return backup, nil
}

// Restore downloads a backup archive and restores it into the server data
// directory.
func (s *Store) Restore(ctx context.Context, backupID string) error {
	if s.client == nil {
		return ErrNotConfigured
	}
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}
	rc, err := s.client.getObject(ctx, b.Location)
	if err != nil {
		return err
	}
	defer rc.Close()

	archivePath := filepath.Join(os.TempDir(), "mcm-restore-"+backupID+".tar.gz")
	out, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		os.Remove(archivePath)
		return err
	}
	out.Close()
	defer os.Remove(archivePath)

	return s.extractWorld(ctx, b.ServerID, archivePath)
}

// Delete removes a backup from the object store and its database record.
func (s *Store) Delete(ctx context.Context, backupID string) error {
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}
	if s.client != nil {
		if err := s.client.deleteObject(ctx, b.Location); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, backupID)
	return err
}

// SetStatus updates the status of a backup record.
func (s *Store) SetStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE backups SET status=? WHERE id=?`, status, id)
	return err
}

// Retention returns the number of backups to keep per server. A non-positive
// value means keep everything.
func (s *Store) Retention(ctx context.Context) (keep int) {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='backup_retention'`).Scan(&v); err != nil {
		return 0
	}
	fmt.Sscanf(v, "%d", &keep)
	return keep
}

// enforceRetention deletes the oldest backups beyond the configured retention
// count for a server.
func (s *Store) enforceRetention(ctx context.Context, serverID string) error {
	keep := s.Retention(ctx)
	if keep <= 0 {
		return nil
	}
	list, err := s.List(ctx, serverID)
	if err != nil {
		return err
	}
	// List is newest-first; delete the tail beyond keep.
	if len(list) <= keep {
		return nil
	}
	for _, b := range list[keep:] {
		if err := s.Delete(ctx, b.ID); err != nil {
			return err
		}
	}
	return nil
}

// archiveWorld tar.gz's the contents of a server data directory (excluding a
// lock file) into a temporary file and returns its path.
func (s *Store) archiveWorld(ctx context.Context, serverID string) (string, error) {
	srcDir := s.serverDataDir(serverID)
	if fi, err := os.Stat(srcDir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("server data directory %s is not accessible", srcDir)
	}

	archivePath := filepath.Join(os.TempDir(), "mcm-backup-"+uuid.NewString()+".tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !d.IsDir() {
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			if _, err := io.Copy(tw, in); err != nil {
				in.Close()
				return err
			}
			in.Close()
		}
		return nil
	})
	if err != nil {
		tw.Close()
		gz.Close()
		f.Close()
		os.Remove(archivePath)
		return "", err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		f.Close()
		os.Remove(archivePath)
		return "", err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		os.Remove(archivePath)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(archivePath)
		return "", err
	}
	return archivePath, nil
}

// extractWorld restores a tar.gz archive into a server data directory.
func (s *Store) extractWorld(ctx context.Context, serverID, archivePath string) error {
	dstDir := s.serverDataDir(serverID)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(hdr.Name)
		// Guard against path traversal.
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("archive contains unsafe path %q", hdr.Name)
		}
		target := filepath.Join(dstDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
