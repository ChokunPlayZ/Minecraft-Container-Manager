// Package storage provides bounded, rooted access and atomic publication for server data.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var ErrUnsafe = errors.New("unsafe storage path or file")
var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

const MaxArchiveBytes int64 = 8 << 30
const MaxArchiveEntries = 100000
const MaxUploadBytes int64 = 2 << 30

func ValidID(id string) bool { return idPattern.MatchString(id) }
func Clean(name string) (string, error) {
	if name == "" {
		name = "."
	}
	name = path.Clean(name)
	if name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) || strings.ContainsAny(name, "\\\x00") {
		return "", ErrUnsafe
	}
	return name, nil
}

// OpenServer anchors the handle beneath trusted panel storage. Server directory
// entries must be actual directories, never links to another server or location.
func OpenServer(dataDir, id string) (*os.Root, error) {
	if !ValidID(id) {
		return nil, ErrUnsafe
	}
	panel, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}
	defer panel.Close()
	if err = panel.MkdirAll("servers", 0755); err != nil {
		return nil, err
	}
	info, err := panel.Lstat("servers")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafe
	}
	servers, err := panel.OpenRoot("servers")
	if err != nil {
		return nil, err
	}
	defer servers.Close()
	if err = servers.MkdirAll(id, 0755); err != nil {
		return nil, err
	}
	info, err = servers.Lstat(id)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafe
	}
	return servers.OpenRoot(id)
}

func Check(root *os.Root, name string) error {
	name, err := Clean(name)
	if err != nil {
		return err
	}
	current := ""
	for _, part := range strings.Split(name, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return ErrUnsafe
		}
	}
	return nil
}

func OpenRegular(root *os.Root, name string) (*os.File, error) {
	if err := Check(root, name); err != nil {
		return nil, err
	}
	f, err := openNonblocking(root, name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrUnsafe
	}
	return f, nil
}

// Write publishes only complete files; a failed copy never truncates an existing file.
func Write(ctx context.Context, root *os.Root, name string, src io.Reader, limit int64, mode fs.FileMode) (int64, error) {
	name, err := Clean(name)
	if err != nil || name == "." {
		return 0, ErrUnsafe
	}
	if err := Check(root, name); err != nil {
		return 0, err
	}
	if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
		return 0, err
	}
	temp := path.Join(path.Dir(name), ".mcm-write-"+uuid.NewString())
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return 0, err
	}
	defer root.Remove(temp)
	n, err := io.Copy(f, io.LimitReader(&contextReader{ctx, src}, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("file exceeds %d byte limit", limit)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return n, err
	}
	if err = Check(root, name); err != nil {
		return n, err
	}
	return n, root.Rename(temp, name)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func Copy(ctx context.Context, w io.Writer, r io.Reader, limit int64) (int64, error) {
	n, err := io.Copy(w, io.LimitReader(&contextReader{ctx, r}, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("archive exceeds byte limit")
	}
	return n, err
}
func Relative(root *os.Root, absolute string) string {
	rel, _ := filepath.Rel(root.Name(), absolute)
	return filepath.ToSlash(rel)
}

// OpenContainedRegular follows file links only inside the root, retaining the
// nonblocking open and fstat checks that reject devices and FIFOs.
func OpenContainedRegular(root *os.Root, name string) (*os.File, error) {
	if _, err := Clean(name); err != nil {
		return nil, err
	}
	f, err := openNonblockingLinks(root, name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrUnsafe
	}
	return f, nil
}
