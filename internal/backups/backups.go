// Package backups implements local and S3-compatible world backup and restore
// for MCM. Local archives can live on separate media from server data.
package backups

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/mcm-panel/mcm/internal/storage"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/notifications"
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

// Store coordinates backup records in SQLite and local or S3 archive storage.
type Store struct {
	db             *sql.DB
	client         *s3Client
	dataDir        string
	backupDir      string
	mu             sync.RWMutex
	progress       map[string]*BackupProgress
	subscribers    map[string][]chan BackupProgress
	notifier       notifications.Notifier
	operationLocks sync.Map
	slots          chan struct{}
	coordinate     func(context.Context, string, bool, func(context.Context) error) error
}

// SetNotifier must be called before backup operations start.
func (s *Store) SetNotifier(n notifications.Notifier) { s.notifier = n }

// S3Config configures the remote object store.
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
}

// New constructs a backup Store. The optional backup directory defaults to
// dataDir/backups. S3 operations require an endpoint and bucket.
func New(handle *sql.DB, cfg S3Config, dataDir string, backupDir ...string) *Store {
	s := &Store{
		db:          handle,
		slots:       make(chan struct{}, 2),
		dataDir:     dataDir,
		backupDir:   filepath.Join(dataDir, "backups"),
		progress:    make(map[string]*BackupProgress),
		subscribers: make(map[string][]chan BackupProgress),
	}
	if len(backupDir) > 0 && backupDir[0] != "" {
		s.backupDir = filepath.Clean(backupDir[0])
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
	return filepath.Join(s.backupDir, serverID, backupID+".tar.gz")
}

// existingLocalBackupPath also finds archives created before separate backup
// media was configured. New writes always use backupDir.
func (s *Store) existingLocalBackupPath(serverID, backupID string) (string, error) {
	path := s.localBackupPath(serverID, backupID)
	_, err := os.Stat(path)
	legacy := filepath.Join(s.dataDir, "backups", serverID, backupID+".tar.gz")
	if errors.Is(err, os.ErrNotExist) && path != legacy {
		if _, legacyErr := os.Stat(legacy); legacyErr == nil || !errors.Is(legacyErr, os.ErrNotExist) {
			return legacy, legacyErr
		}
	}
	return path, err
}

// createTempArchive keeps staging on backup media, avoiding large archives on
// the system's temporary filesystem and allowing atomic local publication.
func (s *Store) createTempArchive() (*os.File, error) {
	dir := filepath.Join(s.backupDir, ".tmp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create backup staging directory: %w", err)
	}
	return os.CreateTemp(dir, "mcm-*.tar.gz")
}

func (s *Store) saveLocalArchive(serverID, backupID, archivePath string) error {
	path := s.localBackupPath(serverID, backupID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create local backup directory: %w", err)
	}
	if err := os.Rename(archivePath, path); err != nil {
		return fmt.Errorf("save local backup: %w", err)
	}
	return nil
}

// validateArchiveDestination prevents an archive from including itself, even
// when the configured backup directory reaches server data through a symlink.
func validateArchiveDestination(serverDir, archiveDir string) error {
	source, err := os.Stat(serverDir)
	if err != nil {
		return err
	}
	dir, err := filepath.EvalSymlinks(archiveDir)
	if err != nil {
		return err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}
	for {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if os.SameFile(source, info) {
			return errors.New("backup directory must be outside the server data directory")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}

func (s *Store) objectKey(backupID, serverID string) string {
	return fmt.Sprintf("backups/%s/%s.tar.gz", serverID, backupID)
}

// List returns all backups for a server, newest first.
func (s *Store) List(ctx context.Context, serverID string) ([]Backup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, server_id, name, COALESCE(size_bytes,0), COALESCE(location,''), status, created_at FROM backups WHERE server_id = ? ORDER BY created_at DESC, rowid DESC`, serverID)
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
	previous := s.progress[serverID]
	s.progress[serverID] = &p
	subs := append([]chan BackupProgress(nil), s.subscribers[serverID]...)
	s.mu.Unlock()
	if s.notifier != nil && p.Operation == "backup" && (p.Stage == "completed" || p.Stage == "failed") &&
		(previous == nil || previous.ID != p.ID || previous.Stage != p.Stage) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		var name string
		_ = s.db.QueryRowContext(ctx, `SELECT name FROM servers WHERE id=?`, serverID).Scan(&name)
		cancel()
		if name == "" {
			name = serverID
		}
		kind := notifications.BackupCompleted
		if p.Stage == "failed" {
			kind = notifications.BackupFailed
		}
		s.notifier.Notify(notifications.Event{Kind: kind, ServerID: serverID, ServerName: name, Message: p.Message})
	}

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
func (s *Store) Backup(ctx context.Context, serverID, name string, target ...string) (result *Backup, err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	release, err := s.lockOperation(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer release()
	err = s.withData(ctx, serverID, false, func(ctx context.Context) error {
		var e error
		result, e = s.backupLocked(ctx, serverID, name, target...)
		return e
	})
	return result, err
}
func (s *Store) backupLocked(ctx context.Context, serverID, name string, storageTarget ...string) (*Backup, error) {
	target := ""
	if len(storageTarget) > 0 && storageTarget[0] != "" {
		target = strings.ToLower(strings.TrimSpace(storageTarget[0]))
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

	if target != "local" && target != "s3" {
		return nil, fmt.Errorf("invalid backup storage %q", target)
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
		if err := s.saveLocalArchive(serverID, id, archivePath); err != nil {
			s.SetStatus(ctx, id, StatusFailed)
			s.SetProgress(serverID, BackupProgress{
				ID: id, Operation: "backup", Stage: "failed", Percent: 100,
				Message: "Failed saving local backup: " + err.Error(), Error: err.Error(),
			})
			return backup, err
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
		onRead := func(n int) {
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
		}

		if err := s.client.putObject(ctx, location, f, onRead); err != nil {
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
func (s *Store) UploadBackup(ctx context.Context, serverID, name string, r io.Reader, size int64, target ...string) (*Backup, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	release, err := s.lockOperation(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer release()
	if err = s.checkServer(ctx, serverID); err != nil {
		return nil, err
	}
	return s.uploadBackupLocked(ctx, serverID, name, r, size, target...)
}
func (s *Store) uploadBackupLocked(ctx context.Context, serverID, name string, r io.Reader, size int64, storageTarget ...string) (*Backup, error) {
	target := ""
	if len(storageTarget) > 0 && storageTarget[0] != "" {
		target = strings.ToLower(strings.TrimSpace(storageTarget[0]))
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

	if target != "local" && target != "s3" {
		return nil, fmt.Errorf("invalid backup storage %q", target)
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

	tmpFile, err := s.createTempArchive()
	if err != nil {
		s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return nil, err
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()
	writtenSize, err := storage.Copy(ctx, tmpFile, r, storage.MaxUploadBytes)
	if err == nil {
		_, err = tmpFile.Seek(0, io.SeekStart)
	}
	if err == nil {
		err = storage.ExtractTarGz(ctx, tmpFile, nil)
	}
	if err == nil {
		if target == "local" {
			if err = tmpFile.Close(); err == nil {
				err = s.saveLocalArchive(serverID, id, tmpFile.Name())
			}
		} else {
			if _, err = tmpFile.Seek(0, io.SeekStart); err == nil {
				err = s.client.putObject(ctx, location, tmpFile)
			}
		}
	}
	if err != nil {
		s.SetProgress(serverID, BackupProgress{ID: id, Operation: "upload", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return nil, err
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

	if err := s.enforceRetention(ctx, serverID); err != nil {
		return backup, err
	}
	return backup, nil
}

// Restore downloads a backup archive and restores it into the server data
// directory.
func (s *Store) Restore(ctx context.Context, backupID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}
	if b.Status != StatusCompleted {
		return fmt.Errorf("only completed backups can be restored")
	}
	release, err := s.lockOperation(ctx, b.ServerID)
	if err != nil {
		return err
	}
	defer release()
	return s.withData(ctx, b.ServerID, true, func(ctx context.Context) error { return s.restoreLocked(ctx, backupID) })
}
func (s *Store) restoreLocked(ctx context.Context, backupID string) error {
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
		archivePath, err := s.existingLocalBackupPath(b.ServerID, b.ID)
		if err != nil {
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

	out, err := s.createTempArchive()
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
	archivePath := out.Name()

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

	if _, err := storage.Copy(ctx, cw, rc, storage.MaxArchiveBytes+(64<<20)); err != nil {
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
	defer os.Remove(archivePath)
	if err := out.Close(); err != nil {
		s.SetProgress(b.ServerID, BackupProgress{ID: b.ID, Operation: "restore", Stage: "failed", Percent: 100, Message: err.Error(), Error: err.Error()})
		return err
	}

	return s.extractWorld(ctx, b.ServerID, archivePath, b.ID)
}

// Delete removes a backup from storage and its database record.
func (s *Store) Delete(ctx context.Context, backupID string) error {
	b, err := s.Get(ctx, backupID)
	if err != nil {
		return err
	}
	if strings.HasPrefix(b.Location, "local:") {
		path, err := s.existingLocalBackupPath(b.ServerID, b.ID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
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
		path, err := s.existingLocalBackupPath(b.ServerID, b.ID)
		if err != nil {
			return nil, 0, "", err
		}
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
	completed := list[:0]
	for _, b := range list {
		if b.Status == StatusCompleted {
			completed = append(completed, b)
		}
	}
	list = completed
	// List is newest-first; failed and pending records never displace recovery history.
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
func (s *Store) archiveWorld(ctx context.Context, serverID string, backupIDs ...string) (archivePath string, err error) {
	root, err := storage.OpenServer(s.dataDir, serverID)
	if err != nil {
		return "", err
	}
	defer root.Close()
	f, err := s.createTempArchive()
	if err != nil {
		return "", err
	}
	if err = validateArchiveDestination(root.Name(), filepath.Dir(f.Name())); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}

	defer func() {
		f.Close()
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	budget := storage.NewBudget(storage.MaxArchiveBytes)
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if filepath.Base(name) == "session.lock" {
			return nil
		}

		if d.IsDir() && (strings.HasPrefix(filepath.Base(name), ".mcm-runtime-stage-") || strings.HasPrefix(filepath.Base(name), ".mcm-pack-stage-") || strings.HasPrefix(filepath.Base(name), ".mcm-artifact-stage-") || strings.HasPrefix(filepath.Base(name), ".mcm-publish-recovery-")) {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		readName := name
		if info.Mode()&os.ModeSymlink != 0 {
			target, linkErr := root.Readlink(name)
			if linkErr != nil {
				return linkErr
			}
			// Older runtime scripts linked to the container's /data mount. Resolve
			// that exact mount prefix inside this server rather than on the panel host.
			if strings.HasPrefix(target, "/data/") {
				readName = strings.TrimPrefix(target, "/data/")
			}
			info, err = root.Stat(readName)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return storage.ErrUnsafe
			}
		} else if err = storage.Check(root, name); err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("cannot back up special file %s", name)
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if err = tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := storage.OpenContainedRegular(root, readName)
		if err != nil {
			return err
		}
		defer file.Close()
		n, err := storage.Copy(ctx, budget.Writer(tw), file, info.Size())
		if err == nil && n != info.Size() {
			err = fmt.Errorf("file changed during backup")
		}
		return err
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gz.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return f.Name(), err
}

// Extract into an isolated sibling tree before replacing the complete snapshot.
// The previous tree is retained as .mcm-recovery-* for explicit recovery.
func (s *Store) extractWorld(ctx context.Context, serverID, archivePath string, backupIDs ...string) (err error) {
	if !storage.ValidID(serverID) {
		return storage.ErrUnsafe
	}
	backupID := ""
	if len(backupIDs) > 0 {
		backupID = backupIDs[0]
	}
	defer func() {
		if err != nil {
			s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "failed", Error: err.Error(), Message: err.Error(), Percent: 100})
		}
	}()
	current, err := storage.OpenServer(s.dataDir, serverID)
	if err != nil {
		return err
	}
	current.Close()
	parent, err := os.OpenRoot(filepath.Join(s.dataDir, "servers"))
	if err != nil {
		return err
	}
	defer parent.Close()
	stageName := ".mcm-restore-" + uuid.NewString()
	if err = parent.Mkdir(stageName, 0700); err != nil {
		return err
	}
	defer parent.RemoveAll(stageName)
	stage, err := parent.OpenRoot(stageName)
	if err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		stage.Close()
		return err
	}
	s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "extracting", Percent: 20, Message: "Validating and staging backup"})
	err = storage.ExtractTarGz(ctx, f, stage)
	f.Close()
	stage.Close()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	recovery := ".mcm-recovery-" + serverID + "-" + uuid.NewString()
	if err = parent.Rename(serverID, recovery); err != nil {
		return err
	}
	if err = parent.Rename(stageName, serverID); err != nil {
		if rollbackErr := parent.Rename(recovery, serverID); rollbackErr != nil {
			return fmt.Errorf("restore failed: %v; previous data retained at %s: %w", err, recovery, rollbackErr)
		}
		return err
	}
	s.SetProgress(serverID, BackupProgress{ID: backupID, Operation: "restore", Stage: "completed", Percent: 100, Message: "Backup restored; previous data retained in " + recovery})
	return nil
}

// SetDataCoordinator connects snapshots to the server lifecycle before jobs start.
func (s *Store) SetDataCoordinator(fn func(context.Context, string, bool, func(context.Context) error) error) {
	s.coordinate = fn
}
func (s *Store) checkServer(ctx context.Context, id string) error {
	if !storage.ValidID(id) {
		return storage.ErrUnsafe
	}
	var found string
	return s.db.QueryRowContext(ctx, `SELECT id FROM servers WHERE id=?`, id).Scan(&found)
}
func (s *Store) withData(ctx context.Context, id string, restore bool, fn func(context.Context) error) error {
	if err := s.checkServer(ctx, id); err != nil {
		return err
	}
	if s.coordinate != nil {
		return s.coordinate(ctx, id, restore, fn)
	}
	// Standalone callers must supply stopped storage with no attached runtime.
	var state, containerID string
	if err := s.db.QueryRowContext(ctx, `SELECT state,COALESCE(container_id,'') FROM servers WHERE id=?`, id).Scan(&state, &containerID); err != nil {
		return err
	}
	if containerID != "" || (state != "stopped" && state != "error") {
		return fmt.Errorf("stop the server before backup or restore")
	}
	return fn(ctx)
}
func (s *Store) lockOperation(ctx context.Context, id string) (func(), error) {
	if !storage.ValidID(id) {
		return nil, storage.ErrUnsafe
	}
	value, _ := s.operationLocks.LoadOrStore(id, make(chan struct{}, 1))
	lock := value.(chan struct{})
	select {
	case lock <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		<-lock
		return nil, ctx.Err()
	}
	return func() { <-s.slots; <-lock }, nil
}
