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
	"sync"
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

// BackupProgress represents the progress of an active backup operation.
type BackupProgress struct {
	ID         string `json:"id"`
	ServerID   string `json:"server_id"`
	Operation  string `json:"operation"` // "backup", "restore", "upload"
	Stage      string `json:"stage"`     // "scanning", "compressing", "saving", "downloading", "extracting", "completed", "failed"
	Percent    int    `json:"percent"`
	Message    string `json:"message"`
	BytesDone  int64  `json:"bytes_done"`
	BytesTotal int64  `json:"bytes_total"`
	Error      string `json:"error,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

type countingReader struct {
	r      io.Reader
	onRead func(n int)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.onRead != nil {
		c.onRead(n)
	}
	return n, err
}

type countingWriter struct {
	w       io.Writer
	onWrite func(n int)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 && c.onWrite != nil {
		c.onWrite(n)
	}
	return n, err
}

func formatBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	val := float64(bytes)
	for val >= 1024 && i < len(units)-1 {
		val /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", val, units[i])
}

// Store coordinates backup records in SQLite and object movement to S3.
type Store struct {
	db          *sql.DB
	client      *s3Client
	dataDir     string
	mu          sync.RWMutex
	progress    map[string]*BackupProgress
	subscribers map[string][]chan BackupProgress
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
	s := &Store{
		db:          handle,
		dataDir:     dataDir,
		progress:    make(map[string]*BackupProgress),
		subscribers: make(map[string][]chan BackupProgress),
	}
	if cfg.Endpoint != "" && cfg.Bucket != "" {
		s.client = newS3Client(cfg)
	}
	return s
}

// serverDataDir returns the per-server data directory.
func (s *Store) serverDataDir(serverID string) string {
	return filepath.Join(s.dataDir, "servers", serverID)
}

// localBackupPath returns the on-disk file path for a local backup.
func (s *Store) localBackupPath(serverID, backupID string) string {
	return filepath.Join(s.dataDir, "backups", serverID, backupID+".tar.gz")
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

// SetProgress records active progress for a server and notifies subscribers.
func (s *Store) SetProgress(serverID string, p BackupProgress) {
	s.mu.Lock()
	p.ServerID = serverID
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.progress[serverID] = &p
	subs := append([]chan BackupProgress(nil), s.subscribers[serverID]...)
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- p:
		default:
		}
	}
}

// ClearProgress clears any progress state for a server.
func (s *Store) ClearProgress(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.progress, serverID)
}

// GetProgress returns the active progress for a server, or nil.
func (s *Store) GetProgress(serverID string) *BackupProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p := s.progress[serverID]
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

// SubscribeProgress returns a channel receiving progress updates for a server.
func (s *Store) SubscribeProgress(serverID string) (chan BackupProgress, func()) {
	s.mu.Lock()
	ch := make(chan BackupProgress, 10)
	if p, ok := s.progress[serverID]; ok && p != nil {
		ch <- *p
	}
	s.subscribers[serverID] = append(s.subscribers[serverID], ch)
	s.mu.Unlock()

	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.subscribers[serverID]
		for i, c := range subs {
			if c == ch {
				s.subscribers[serverID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
	}
	return ch, cancel
}

// Backup archives a server's world directory and saves it to local disk or S3,
// then records the result in the database.
func (s *Store) Backup(ctx context.Context, serverID, name string, storage ...string) (*Backup, error) {
	target := ""
	if len(storage) > 0 && storage[0] != "" {
		target = strings.ToLower(strings.TrimSpace(storage[0]))
	} else if s.db != nil {
		var val string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'backup_storage_type'`).Scan(&val); err == nil && val != "" {
			target = strings.ToLower(strings.TrimSpace(val))
		}
	}
	if target == "" {
		if s.client != nil {
			target = "s3"
		} else {
			target = "local"
		}
	}

	if target == "s3" && s.client == nil {
		return nil, ErrNotConfigured
	}

	if name == "" {
		name = time.Now().UTC().Format("2006-01-02T15-04-05")
	}
	id := uuid.NewString()
	location := s.objectKey(id, serverID)
	if target == "local" {
		location = "local:" + location
	}
	now := time.Now().UTC().Format(time.RFC3339)

	s.SetProgress(serverID, BackupProgress{
		ID:        id,
		Operation: "backup",
		Stage:     "scanning",
		Percent:   5,
		Message:   "Preparing backup...",
	})

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, 0, ?, ?, ?)`,
		id, serverID, name, location, StatusPending, now); err != nil {
		s.SetProgress(serverID, BackupProgress{
			ID:        id,
			Operation: "backup",
			Stage:     "failed",
			Percent:   100,
			Message:   "Backup failed: " + err.Error(),
			Error:     err.Error(),
		})
		return nil, fmt.Errorf("insert backup record: %w", err)
	}

	backup := &Backup{ID: id, ServerID: serverID, Name: name, Location: location, Status: StatusPending, CreatedAt: now}

	// Archive the current world into a temporary file.
	archivePath, err := s.archiveWorld(ctx, serverID, id)
	if err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		s.SetProgress(serverID, BackupProgress{
			ID:        id,
			Operation: "backup",
			Stage:     "failed",
			Percent:   100,
			Message:   "Archive failed: " + err.Error(),
			Error:     err.Error(),
		})
		return backup, err
	}
	defer os.Remove(archivePath)

	stat, err := os.Stat(archivePath)
	if err != nil {
		s.SetStatus(ctx, id, StatusFailed)
		s.SetProgress(serverID, BackupProgress{
			ID:        id,
			Operation: "backup",
			Stage:     "failed",
			Percent:   100,
			Message:   "Backup failed: " + err.Error(),
			Error:     err.Error(),
		})
		return backup, err
	}

	archiveSize := stat.Size()
	s.SetProgress(serverID, BackupProgress{
		ID:         id,
		Operation:  "backup",
		Stage:      "saving",
		Percent:    75,
		Message:    "Saving backup archive...",
		BytesDone:  0,
		BytesTotal: archiveSize,
	})

	if target == "local" {
		localPath := s.localBackupPath(serverID, id)
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed to create backup dir: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, fmt.Errorf("create local backup dir: %w", err)
		}
		in, err := os.Open(archivePath)
		if err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed to open archive: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, err
		}
		out, err := os.Create(localPath)
		if err != nil {
			in.Close()
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed to write local backup: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, err
		}
		var copied int64
		lastUpdate := time.Now()
		cw := &countingWriter{
			w: out,
			onWrite: func(n int) {
				copied += int64(n)
				if time.Since(lastUpdate) > 100*time.Millisecond {
					lastUpdate = time.Now()
					pct := 75
					if archiveSize > 0 {
						pct = 75 + int((float64(copied)/float64(archiveSize))*23.0)
						if pct > 98 {
							pct = 98
						}
					}
					s.SetProgress(serverID, BackupProgress{
						ID:         id,
						Operation:  "backup",
						Stage:      "saving",
						Percent:    pct,
						Message:    fmt.Sprintf("Saving backup to disk (%s / %s)...", formatBytes(copied), formatBytes(archiveSize)),
						BytesDone:  copied,
						BytesTotal: archiveSize,
					})
				}
			},
		}
		_, err = io.Copy(cw, in)
		in.Close()
		cerr := out.Close()
		if err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed copying backup: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, err
		}
		if cerr != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed saving backup: " + cerr.Error(),
				Error:     cerr.Error(),
			})
			return backup, cerr
		}
	} else {
		f, err := os.Open(archivePath)
		if err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Failed reading archive: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, err
		}
		defer f.Close()

		var readBytes int64
		lastUpdate := time.Now()
		cr := &countingReader{
			r: f,
			onRead: func(n int) {
				readBytes += int64(n)
				if time.Since(lastUpdate) > 100*time.Millisecond {
					lastUpdate = time.Now()
					pct := 75
					if archiveSize > 0 {
						pct = 75 + int((float64(readBytes)/float64(archiveSize))*23.0)
						if pct > 98 {
							pct = 98
						}
					}
					s.SetProgress(serverID, BackupProgress{
						ID:         id,
						Operation:  "backup",
						Stage:      "saving",
						Percent:    pct,
						Message:    fmt.Sprintf("Uploading backup to cloud (%s / %s)...", formatBytes(readBytes), formatBytes(archiveSize)),
						BytesDone:  readBytes,
						BytesTotal: archiveSize,
					})
				}
			},
		}

		if err := s.client.putObject(ctx, location, cr); err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID:        id,
				Operation: "backup",
				Stage:     "failed",
				Percent:   100,
				Message:   "Cloud upload failed: " + err.Error(),
				Error:     err.Error(),
			})
			return backup, err
		}
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE backups SET size_bytes=?, status=? WHERE id=?`, stat.Size(), StatusCompleted, id); err != nil {
		s.SetProgress(serverID, BackupProgress{
			ID:        id,
			Operation: "backup",
			Stage:     "failed",
			Percent:   100,
			Message:   "Database update failed: " + err.Error(),
			Error:     err.Error(),
		})
		return backup, err
	}
	backup.SizeBytes = stat.Size()
	backup.Status = StatusCompleted

	s.SetProgress(serverID, BackupProgress{
		ID:         id,
		Operation:  "backup",
		Stage:      "completed",
		Percent:    100,
		Message:    "Backup created successfully",
		BytesDone:  stat.Size(),
		BytesTotal: stat.Size(),
	})

	if err := s.enforceRetention(ctx, serverID); err != nil {
		return backup, err
	}
	return backup, nil
}

// UploadBackup saves an uploaded archive (.tar.gz or .zip) as a backup record.
func (s *Store) UploadBackup(ctx context.Context, serverID, name string, r io.Reader, size int64, storage ...string) (*Backup, error) {
	target := ""
	if len(storage) > 0 && storage[0] != "" {
		target = strings.ToLower(strings.TrimSpace(storage[0]))
	} else if s.db != nil {
		var val string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'backup_storage_type'`).Scan(&val); err == nil && val != "" {
			target = strings.ToLower(strings.TrimSpace(val))
		}
	}
	if target == "" {
		if s.client != nil {
			target = "s3"
		} else {
			target = "local"
		}
	}

	if target == "s3" && s.client == nil {
		return nil, ErrNotConfigured
	}

	if name == "" {
		name = "uploaded-" + time.Now().UTC().Format("2006-01-02T15-04-05")
	}
	id := uuid.NewString()
	location := s.objectKey(id, serverID)
	if target == "local" {
		location = "local:" + location
	}
	now := time.Now().UTC().Format(time.RFC3339)

	s.SetProgress(serverID, BackupProgress{
		ID:         id,
		Operation:  "upload",
		Stage:      "saving",
		Percent:    10,
		Message:    "Saving uploaded backup...",
		BytesDone:  0,
		BytesTotal: size,
	})

	var writtenSize int64
	if target == "local" {
		localPath := s.localBackupPath(serverID, id)
		if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, fmt.Errorf("create local backup dir: %w", err)
		}
		out, err := os.Create(localPath)
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
		defer out.Close()

		n, err := io.Copy(out, r)
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
		writtenSize = n
	} else {
		tmpFile, err := os.CreateTemp("", "mcm_backup_upload_*.tar.gz")
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
		defer func() {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		}()
		n, err := io.Copy(tmpFile, r)
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
		writtenSize = n
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
		if err := s.client.putObject(ctx, location, tmpFile); err != nil {
			s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return nil, err
		}
	}

	if size > 0 && writtenSize == 0 {
		writtenSize = size
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (id, server_id, name, size_bytes, location, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, serverID, name, writtenSize, location, StatusCompleted, now); err != nil {
		s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return nil, fmt.Errorf("insert backup record: %w", err)
	}

	backup := &Backup{
		ID:        id,
		ServerID:  serverID,
		Name:      name,
		SizeBytes: writtenSize,
		Location:  location,
		Status:    StatusCompleted,
		CreatedAt: now,
	}

	s.SetProgress(serverID, BackupProgress{
		ID:         id,
		Operation:  "upload",
		Stage:      "completed",
		Percent:    100,
		Message:    "Backup uploaded successfully",
		BytesDone:  writtenSize,
		BytesTotal: writtenSize,
	})

	_ = s.enforceRetention(ctx, serverID)
	return backup, nil
}

// Restore downloads a backup archive and restores it into the server data
// directory.
func (s *Store) Restore(ctx context.Context, backupID string) error {
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}

	s.SetProgress(b.ServerID, BackupProgress{
		ID:        b.ID,
		Operation: "restore",
		Stage:     "downloading",
		Percent:   5,
		Message:   "Preparing backup for restoration...",
	})

	if strings.HasPrefix(b.Location, "local:") {
		archivePath := s.localBackupPath(b.ServerID, b.ID)
		if _, err := os.Stat(archivePath); err != nil {
			s.SetProgress(b.ServerID, BackupProgress{
				ID:        b.ID,
				Operation: "restore",
				Stage:     "failed",
				Percent:   100,
				Message:   "Local backup file not accessible",
				Error:     err.Error(),
			})
			return fmt.Errorf("local backup file not accessible: %w", err)
		}
		return s.extractWorld(ctx, b.ServerID, archivePath, b.ID)
	}

	if s.client == nil {
		s.SetProgress(b.ServerID, BackupProgress{
			ID:        b.ID,
			Operation: "restore",
			Stage:     "failed",
			Percent:   100,
			Message:   "S3 backup storage is not configured",
			Error:     ErrNotConfigured.Error(),
		})
		return ErrNotConfigured
	}

	s.SetProgress(b.ServerID, BackupProgress{
		ID:         b.ID,
		Operation:  "restore",
		Stage:      "downloading",
		Percent:    10,
		Message:    "Downloading cloud backup...",
		BytesDone:  0,
		BytesTotal: b.SizeBytes,
	})

	rc, err := s.client.getObject(ctx, b.Location)
	if err != nil {
		s.SetProgress(b.ServerID, BackupProgress{
			ID:        b.ID,
			Operation: "restore",
			Stage:     "failed",
			Percent:   100,
			Message:   "Failed to download cloud backup: " + err.Error(),
			Error:     err.Error(),
		})
		return err
	}
	defer rc.Close()

	archivePath := filepath.Join(os.TempDir(), "mcm-restore-"+backupID+".tar.gz")
	out, err := os.Create(archivePath)
	if err != nil {
		s.SetProgress(b.ServerID, BackupProgress{
			ID:        b.ID,
			Operation: "restore",
			Stage:     "failed",
			Percent:   100,
			Message:   "Failed creating temp file: " + err.Error(),
			Error:     err.Error(),
		})
		return err
	}

	var copied int64
	lastUpdate := time.Now()
	cw := &countingWriter{
		w: out,
		onWrite: func(n int) {
			copied += int64(n)
			if time.Since(lastUpdate) > 100*time.Millisecond {
				lastUpdate = time.Now()
				pct := 10
				if b.SizeBytes > 0 {
					pct = 10 + int((float64(copied)/float64(b.SizeBytes))*40.0)
					if pct > 50 {
						pct = 50
					}
				}
				s.SetProgress(b.ServerID, BackupProgress{
					ID:         b.ID,
					Operation:  "restore",
					Stage:      "downloading",
					Percent:    pct,
					Message:    fmt.Sprintf("Downloading cloud backup (%s / %s)...", formatBytes(copied), formatBytes(b.SizeBytes)),
					BytesDone:  copied,
					BytesTotal: b.SizeBytes,
				})
			}
		},
	}

	if _, err := io.Copy(cw, rc); err != nil {
		out.Close()
		os.Remove(archivePath)
		s.SetProgress(b.ServerID, BackupProgress{
			ID:        b.ID,
			Operation: "restore",
			Stage:     "failed",
			Percent:   100,
			Message:   "Cloud download failed: " + err.Error(),
			Error:     err.Error(),
		})
		return err
	}
	out.Close()
	defer os.Remove(archivePath)

	return s.extractWorld(ctx, b.ServerID, archivePath, b.ID)
}

// Delete removes a backup from storage and its database record.
func (s *Store) Delete(ctx context.Context, backupID string) error {
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}
	if strings.HasPrefix(b.Location, "local:") {
		_ = os.Remove(s.localBackupPath(b.ServerID, b.ID))
	} else if s.client != nil {
		if err := s.client.deleteObject(ctx, b.Location); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM backups WHERE id = ?`, backupID)
	return err
}

// OpenBackup opens the backup archive (local or S3) for reading, returning the
// stream, size in bytes, download filename, and any error.
func (s *Store) OpenBackup(ctx context.Context, backupID string) (io.ReadCloser, int64, string, error) {
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return nil, 0, "", err
	}
	filename := b.Name
	if !strings.HasSuffix(filename, ".tar.gz") {
		filename += ".tar.gz"
	}

	if strings.HasPrefix(b.Location, "local:") {
		path := s.localBackupPath(b.ServerID, b.ID)
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, "", err
		}
		stat, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, "", err
		}
		return f, stat.Size(), filename, nil
	}

	if s.client == nil {
		return nil, 0, "", ErrNotConfigured
	}
	rc, err := s.client.getObject(ctx, b.Location)
	if err != nil {
		return nil, 0, "", err
	}
	return rc, b.SizeBytes, filename, nil
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
func (s *Store) archiveWorld(ctx context.Context, serverID string, backupIDOpt ...string) (string, error) {
	backupID := ""
	if len(backupIDOpt) > 0 {
		backupID = backupIDOpt[0]
	}
	srcDir := s.serverDataDir(serverID)
	if fi, err := os.Stat(srcDir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("server data directory %s is not accessible", srcDir)
	}

	s.SetProgress(serverID, BackupProgress{
		ID:        backupID,
		Operation: "backup",
		Stage:     "scanning",
		Percent:   5,
		Message:   "Scanning server directory...",
	})

	var totalBytes int64
	var totalFiles int
	_ = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				totalBytes += info.Size()
				totalFiles++
			}
		}
		return nil
	})

	archivePath := filepath.Join(os.TempDir(), "mcm-backup-"+uuid.NewString()+".tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	var processedBytes int64
	lastUpdate := time.Now()

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
			cr := &countingReader{
				r: in,
				onRead: func(n int) {
					processedBytes += int64(n)
					if time.Since(lastUpdate) > 100*time.Millisecond {
						lastUpdate = time.Now()
						pct := 5
						if totalBytes > 0 {
							pct = 5 + int((float64(processedBytes)/float64(totalBytes))*70.0)
							if pct > 75 {
								pct = 75
							}
						}
						s.SetProgress(serverID, BackupProgress{
							ID:         backupID,
							Operation:  "backup",
							Stage:      "compressing",
							Percent:    pct,
							Message:    fmt.Sprintf("Compressing world files (%s / %s)...", formatBytes(processedBytes), formatBytes(totalBytes)),
							BytesDone:  processedBytes,
							BytesTotal: totalBytes,
						})
					}
				},
			}
			if _, err := io.Copy(tw, cr); err != nil {
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
func (s *Store) extractWorld(ctx context.Context, serverID, archivePath string, backupIDOpt ...string) error {
	backupID := ""
	if len(backupIDOpt) > 0 {
		backupID = backupIDOpt[0]
	}
	startPct := 10
	endPct := 98
	dstDir := s.serverDataDir(serverID)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return err
	}
	defer f.Close()

	stat, _ := f.Stat()
	archiveSize := int64(0)
	if stat != nil {
		archiveSize = stat.Size()
	}

	var readBytes int64
	lastUpdate := time.Now()
	cr := &countingReader{
		r: f,
		onRead: func(n int) {
			readBytes += int64(n)
			if time.Since(lastUpdate) > 100*time.Millisecond {
				lastUpdate = time.Now()
				pct := startPct
				if archiveSize > 0 {
					pct = startPct + int((float64(readBytes)/float64(archiveSize))*float64(endPct-startPct))
					if pct > endPct {
						pct = endPct
					}
				}
				s.SetProgress(serverID, BackupProgress{
					ID:         backupID,
					Operation:  "restore",
					Stage:      "extracting",
					Percent:    pct,
					Message:    fmt.Sprintf("Extracting world files (%s / %s)...", formatBytes(readBytes), formatBytes(archiveSize)),
					BytesDone:  readBytes,
					BytesTotal: archiveSize,
				})
			}
		},
	}

	gz, err := gzip.NewReader(cr)
	if err != nil {
		s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		select {
		case <-ctx.Done():
			s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: ctx.Err().Error(), Error: ctx.Err().Error()})
			return ctx.Err()
		default:
		}
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return err
		}
		name := filepath.FromSlash(hdr.Name)
		// Guard against path traversal.
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			err := fmt.Errorf("archive contains unsafe path %q", hdr.Name)
			s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
			return err
		}
		target := filepath.Join(dstDir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
				return err
			}
			out.Close()
		}
	}

	s.SetProgress(serverID, BackupProgress{
		ID:         backupID,
		Operation:  "restore",
		Stage:      "completed",
		Percent:    100,
		Message:    "World restored successfully",
		BytesDone:  archiveSize,
		BytesTotal: archiveSize,
	})
	return nil
}
