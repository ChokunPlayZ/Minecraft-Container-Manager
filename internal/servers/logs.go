package servers

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	maxLogPreviewBytes = 2 << 20
	maxLogScanBytes    = 256 << 20
	maxLogEntries      = 10000
)

var (
	ErrInvalidLogArchive = errors.New("invalid log archive")
	ErrLogScanLimit      = errors.New("log scan limit exceeded")
)

// LogFile is a saved log or an archive containing saved logs. Paths are relative
// to the server data directory, or to the archive for an archive member.
type LogFile struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
	Archive    bool      `json:"archive"`
}

type LogContent struct {
	Path       string `json:"path"`
	Entry      string `json:"entry,omitempty"`
	Content    string `json:"content"`
	Truncated  bool   `json:"truncated"`
	LimitBytes int    `json:"limit_bytes"`
}

func isLogArchive(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".tar")
}

func isLogText(name string) bool {
	name = strings.TrimSuffix(strings.ToLower(name), ".gz")
	return strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".txt")
}

func (s *Store) openLogRoot(id string) (*os.Root, error) {
	if id == "." || !fs.ValidPath(id) || strings.Contains(id, "/") {
		return nil, ErrInvalidPath
	}
	return os.OpenRoot(s.dataPath(id))
}

func sortLogFiles(files []LogFile) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path == "logs/latest.log" || files[j].Path == "logs/latest.log" {
			return files[i].Path == "logs/latest.log"
		}
		if !files[i].ModifiedAt.Equal(files[j].ModifiedAt) {
			return files[i].ModifiedAt.After(files[j].ModifiedAt)
		}
		return files[i].Path > files[j].Path
	})
}

// ListServerLogs discovers saved logs without decompressing any archives.
func (s *Store) ListServerLogs(ctx context.Context, id string) ([]LogFile, error) {
	files := make([]LogFile, 0)
	root, err := s.openLogRoot(id)
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	visited := 0
	for _, dir := range []string{"logs", "crash-reports"} {
		err = fs.WalkDir(root.FS(), dir, func(name string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			visited++
			if visited > maxLogEntries {
				return ErrLogScanLimit
			}
			if !entry.Type().IsRegular() || (!isLogText(name) && !isLogArchive(name)) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files = append(files, LogFile{Path: name, Name: entry.Name(), SizeBytes: info.Size(), ModifiedAt: info.ModTime(), Archive: isLogArchive(name)})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sortLogFiles(files)
	return files, nil
}

func (s *Store) openServerLog(id, rel string) (*os.File, error) {
	if !fs.ValidPath(rel) || (!strings.HasPrefix(rel, "logs/") && !strings.HasPrefix(rel, "crash-reports/")) || (!isLogText(rel) && !isLogArchive(rel)) {
		return nil, ErrInvalidPath
	}
	root, err := s.openLogRoot(id)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalidPath
	}
	return root.Open(rel)
}

// logReader checks cancellation between reads, including while tar.Next skips
// over large members. A decompressed archive has a separate total scan budget.
type logReader struct {
	ctx context.Context
	r   io.Reader
}

func (r logReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func walkLogArchive(ctx context.Context, file *os.File, visit func(*tar.Header, io.Reader) (bool, error)) error {
	var reader io.Reader = file
	if !strings.HasSuffix(strings.ToLower(file.Name()), ".tar") {
		gz, err := gzip.NewReader(logReader{ctx, file})
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidLogArchive, err)
		}
		defer gz.Close()
		reader = gz
	}
	limited := &io.LimitedReader{R: logReader{ctx, reader}, N: maxLogScanBytes + 1}
	tr := tar.NewReader(limited)
	for count := 0; ; count++ {
		header, err := tr.Next()
		if limited.N <= 1 || count >= maxLogEntries {
			return ErrLogScanLimit
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%w: %v", ErrInvalidLogArchive, err)
		}
		// Never follow links or expose absolute/traversing archive member names.
		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag != tar.TypeReg || !fs.ValidPath(name) || !isLogText(name) {
			continue
		}
		header.Name = name
		done, err := visit(header, tr)
		if err != nil || done {
			return err
		}
	}
}

func (s *Store) ListLogArchive(ctx context.Context, id, rel string) ([]LogFile, error) {
	if !isLogArchive(rel) {
		return nil, ErrInvalidPath
	}
	file, err := s.openServerLog(id, rel)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	files := make([]LogFile, 0)
	err = walkLogArchive(ctx, file, func(h *tar.Header, _ io.Reader) (bool, error) {
		files = append(files, LogFile{Path: h.Name, Name: path.Base(h.Name), SizeBytes: h.Size, ModifiedAt: h.ModTime})
		return false, nil
	})
	sortLogFiles(files)
	return files, err
}

func readLogPreview(ctx context.Context, reader io.Reader, name string) (string, bool, error) {
	if strings.HasSuffix(strings.ToLower(name), ".gz") {
		gz, err := gzip.NewReader(logReader{ctx, reader})
		if err != nil {
			return "", false, fmt.Errorf("%w: %v", ErrInvalidLogArchive, err)
		}
		defer gz.Close()
		reader = gz
	}
	data, err := io.ReadAll(io.LimitReader(logReader{ctx, reader}, maxLogPreviewBytes+1))
	if err != nil {
		if strings.HasSuffix(strings.ToLower(name), ".gz") && ctx.Err() == nil {
			err = fmt.Errorf("%w: %v", ErrInvalidLogArchive, err)
		}
		return "", false, err
	}
	truncated := len(data) > maxLogPreviewBytes
	if truncated {
		data = data[:maxLogPreviewBytes]
	}
	return strings.ToValidUTF8(string(data), "�"), truncated, nil
}

// ReadServerLog streams decompression into a bounded preview. It never writes
// extracted files into the server directory or a temporary directory.
func (s *Store) ReadServerLog(ctx context.Context, id, rel, entry string) (LogContent, error) {
	result := LogContent{Path: rel, Entry: entry, LimitBytes: maxLogPreviewBytes}
	file, err := s.openServerLog(id, rel)
	if err != nil {
		return result, err
	}
	defer file.Close()
	if isLogArchive(rel) {
		if !fs.ValidPath(entry) || !isLogText(entry) {
			return result, ErrInvalidPath
		}
		found := false
		err = walkLogArchive(ctx, file, func(h *tar.Header, reader io.Reader) (bool, error) {
			if h.Name != entry {
				return false, nil
			}
			found = true
			result.Content, result.Truncated, err = readLogPreview(ctx, reader, h.Name)
			return true, err
		})
		if err == nil && !found {
			err = ErrPathNotFound
		}
	} else {
		if entry != "" {
			return result, ErrInvalidPath
		}
		result.Content, result.Truncated, err = readLogPreview(ctx, file, rel)
	}
	return result, err
}
