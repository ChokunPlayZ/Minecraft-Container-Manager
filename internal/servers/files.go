package servers

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Errors returned by file operations. These map to friendly HTTP errors in the
// API layer; they never leak raw filesystem or Go error strings.
var (
	// ErrInvalidPath is returned when a requested path escapes the server's
	// data directory or is otherwise unsafe.
	ErrInvalidPath = errors.New("invalid path")
	// ErrPathNotFound is returned when a file or directory does not exist.
	ErrPathNotFound = errors.New("path not found")
	// ErrIsDirectory is returned when a file operation expected a file but got
	// a directory (or vice versa).
	ErrIsDirectory = errors.New("path is a directory")
	// ErrInvalidArchive is returned when a zip archive is malformed or unsafe.
	ErrInvalidArchive = errors.New("invalid archive")
	// ErrDownloadFailed is returned when a URL download fails.
	ErrDownloadFailed = errors.New("download failed")
)

// maxDownloadBytes caps how large a file downloaded from a URL may be (512 MB).
const maxDownloadBytes int64 = 512 << 20

// FileEntry describes a single entry in a server's data directory.
type FileEntry struct {
	Name       string    `json:"name"`
	IsDir      bool      `json:"is_directory"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

// FileListResult is a directory listing for a server data directory.
type FileListResult struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

// resolvePath resolves a server-relative path against the server's data
// directory, guaranteeing the result stays within it. Absolute paths and path
// traversal are rejected.
func (s *Store) resolvePath(id, rel string) (string, error) {
	root := s.dataPath(id)
	if rel == "" {
		return root, nil
	}
	clean := path.Clean(rel)
	// Reject anything that escapes toward the root or is absolute.
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", ErrInvalidPath
	}
	joined := filepath.Join(root, filepath.FromSlash(clean))
	// Belt-and-braces containment check.
	if !strings.HasPrefix(joined, root+string(os.PathSeparator)) && joined != root {
		return "", ErrInvalidPath
	}
	return joined, nil
}

// ListFiles returns the contents of a directory inside the server's data dir.
func (s *Store) ListFiles(id, rel string) (FileListResult, error) {
	dir, err := s.resolvePath(id, rel)
	if err != nil {
		return FileListResult{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return FileListResult{Path: rel, Entries: []FileEntry{}}, nil
		}
		return FileListResult{}, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			info = nil
		}
		fe := FileEntry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
		}
		if info != nil {
			fe.SizeBytes = info.Size()
			fe.ModifiedAt = info.ModTime()
		}
		out = append(out, fe)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return FileListResult{Path: rel, Entries: out}, nil
}

// resolveDownloadPath resolves a requested file for download, rejecting
// directories.
func (s *Store) resolveDownloadPath(id, rel string) (string, error) {
	f, err := s.resolvePath(id, rel)
	if err != nil {
		return "", err
	}
	info, statErr := os.Stat(f)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return "", ErrPathNotFound
		}
		return "", statErr
	}
	if info.IsDir() {
		return "", ErrIsDirectory
	}
	return f, nil
}

// UploadFile saves an uploaded file into a directory inside the server data dir.
func (s *Store) UploadFile(id, dir, name string, r io.Reader) (FileEntry, error) {
	if name == "" || name == "." || name == ".." {
		return FileEntry{}, ErrInvalidPath
	}
	if filepath.Base(name) != name {
		return FileEntry{}, ErrInvalidPath
	}
	target, err := s.resolvePath(id, path.Join(dir, name))
	if err != nil {
		return FileEntry{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return FileEntry{}, err
	}
	dst, err := os.Create(target)
	if err != nil {
		return FileEntry{}, err
	}
	_, werr := io.Copy(dst, r)
	cerr := dst.Close()
	if werr != nil {
		return FileEntry{}, werr
	}
	if cerr != nil {
		return FileEntry{}, cerr
	}
	return statEntry(target, name)
}

// zipWriter is a helper to write one file into a zip archive.
type zipWriter struct {
	zw *zip.Writer
}

func (z *zipWriter) addFile(base, fsPath string, info os.FileInfo) error {
	if info.IsDir() {
		_, err := z.zw.Create(base + "/")
		return err
	}
	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = base
	hdr.Method = zip.Deflate
	w, err := z.zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	f, err := os.Open(fsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// addTree recursively adds a file or directory rooted at fsRoot to the archive
// under the given base name.
func (z *zipWriter) addTree(base, fsRoot string) error {
	info, err := os.Stat(fsRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return z.addFile(base, fsRoot, info)
	}
	entry, err := os.ReadDir(fsRoot)
	if err != nil {
		return err
	}
	if closeErr := z.addFile(base, fsRoot, info); closeErr != nil {
		return closeErr
	}
	for _, e := range entry {
		child := filepath.Join(fsRoot, e.Name())
		childBase := path.Join(base, e.Name())
		if e.IsDir() {
			if err := z.addTree(childBase, child); err != nil {
				return err
			}
		} else {
			info, ierr := e.Info()
			if ierr != nil {
				return ierr
			}
			if err := z.addFile(childBase, child, info); err != nil {
				return err
			}
		}
	}
	return nil
}

// Archive creates a zip of a source file or directory into the server data dir,
// returning the new zip entry. The default archive name is the source basename.
func (s *Store) Archive(id, source, name string) (FileEntry, error) {
	src, err := s.resolvePath(id, source)
	if err != nil {
		return FileEntry{}, err
	}
	_, statErr := os.Stat(src)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return FileEntry{}, ErrPathNotFound
		}
		return FileEntry{}, statErr
	}
	if name == "" {
		name = filepath.Base(src)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".zip") {
		name += ".zip"
	}
	if filepath.Base(name) != name || name == "." || name == ".." {
		return FileEntry{}, ErrInvalidPath
	}
	target, err := s.resolvePath(id, name)
	if err != nil {
		return FileEntry{}, err
	}
	dst, err := os.Create(target)
	if err != nil {
		return FileEntry{}, err
	}
	zw := zip.NewWriter(dst)
	base := filepath.Base(src)
	z := &zipWriter{zw: zw}
	perr := z.addTree(base, src)
	if perr != nil {
		_ = zw.Close()
		_ = dst.Close()
		_ = os.Remove(target)
		return FileEntry{}, perr
	}
	if cerr := zw.Close(); cerr != nil {
		_ = dst.Close()
		_ = os.Remove(target)
		return FileEntry{}, cerr
	}
	if cerr := dst.Close(); cerr != nil {
		_ = os.Remove(target)
		return FileEntry{}, cerr
	}
	return statEntry(target, name)
}

// Unzip extracts a zip archive inside the server data dir into dest, with
// zip-slip protection. dest defaults to the archive's parent directory.
func (s *Store) Unzip(id, archive, dest string) (int, error) {
	arc, err := s.resolvePath(id, archive)
	if err != nil {
		return 0, err
	}
	info, statErr := os.Stat(arc)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return 0, ErrPathNotFound
		}
		return 0, statErr
	}
	if info.IsDir() {
		return 0, ErrIsDirectory
	}
	if dest == "" {
		dest = filepath.Dir(arc)
	} else {
		dest, err = s.resolvePath(id, dest)
		if err != nil {
			return 0, err
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	zr, err := zip.OpenReader(arc)
	if err != nil {
		return 0, ErrInvalidArchive
	}
	defer zr.Close()

	count := 0
	destClean := filepath.Clean(dest)
	for _, f := range zr.File {
		// Zip-slip protection: reject any entry whose cleaned target escapes
		// the destination directory.
		target := filepath.Join(destClean, filepath.FromSlash(f.Name))
		if target != destClean && !strings.HasPrefix(target, destClean+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return count, err
			}
			count++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return count, err
		}
		rc, rerr := f.Open()
		if rerr != nil {
			return count, ErrInvalidArchive
		}
		dst, cerr := os.Create(target)
		if cerr != nil {
			_ = rc.Close()
			return count, cerr
		}
		_, cerr = io.Copy(dst, rc)
		_ = rc.Close()
		_ = dst.Close()
		if cerr != nil {
			return count, cerr
		}
		count++
	}
	return count, nil
}

// DownloadFromURL downloads a file from urlStr into the server data dir. The
// filename is derived from the URL basename unless name is provided.
func (s *Store) DownloadFromURL(id, urlStr, dir, name string) (FileEntry, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return FileEntry{}, ErrDownloadFailed
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return FileEntry{}, ErrInvalidPath
	}
	if name == "" {
		name = path.Base(u.Path)
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return FileEntry{}, ErrInvalidPath
	}
	target, err := s.resolvePath(id, path.Join(dir, name))
	if err != nil {
		return FileEntry{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return FileEntry{}, err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(urlStr)
	if err != nil {
		return FileEntry{}, ErrDownloadFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return FileEntry{}, ErrDownloadFailed
	}

	dst, err := os.Create(target)
	if err != nil {
		return FileEntry{}, err
	}
	_, werr := io.Copy(dst, io.LimitReader(resp.Body, maxDownloadBytes+1))
	cerr := dst.Close()
	if werr != nil {
		_ = os.Remove(target)
		return FileEntry{}, werr
	}
	if cerr != nil {
		_ = os.Remove(target)
		return FileEntry{}, cerr
	}
	if fi, ferr := os.Stat(target); ferr == nil && fi.Size() > maxDownloadBytes {
		_ = os.Remove(target)
		return FileEntry{}, fmt.Errorf("%w: file too large", ErrDownloadFailed)
	}
	return statEntry(target, name)
}

// DeleteFile removes a file or directory recursively from the server data dir.
func (s *Store) DeleteFile(id, rel string) error {
	target, err := s.resolvePath(id, rel)
	if err != nil {
		return err
	}
	if target == s.dataPath(id) {
		return ErrInvalidPath
	}
	info, statErr := os.Stat(target)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return ErrPathNotFound
		}
		return statErr
	}
	if info.IsDir() {
		return os.RemoveAll(target)
	}
	return os.Remove(target)
}

// Mkdir creates a directory inside the server data dir.
func (s *Store) Mkdir(id, rel string) (FileEntry, error) {
	target, err := s.resolvePath(id, rel)
	if err != nil {
		return FileEntry{}, err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return FileEntry{}, err
	}
	return statEntry(target, filepath.Base(target))
}

// Rename moves a path to a new name within the server data dir.
func (s *Store) Rename(id, rel, name string) (FileEntry, error) {
	src, err := s.resolvePath(id, rel)
	if err != nil {
		return FileEntry{}, err
	}
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return FileEntry{}, ErrInvalidPath
	}
	_, statErr := os.Stat(src)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return FileEntry{}, ErrPathNotFound
		}
		return FileEntry{}, statErr
	}
	parentRel := filepath.ToSlash(filepath.Dir(rel))
	dst, err := s.resolvePath(id, path.Join(parentRel, name))
	if err != nil {
		return FileEntry{}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return FileEntry{}, err
	}
	return statEntry(dst, name)
}

// OpenFileForDownload returns an open handle to a file for streaming, plus the
// basename for Content-Disposition. The caller is responsible for closing the
// returned file.
func (s *Store) OpenFileForDownload(id, rel string) (*os.File, string, error) {
	f, err := s.resolveDownloadPath(id, rel)
	if err != nil {
		return nil, "", err
	}
	handle, err := os.Open(f)
	if err != nil {
		return nil, "", err
	}
	return handle, filepath.Base(f), nil
}

// statEntry produces a FileEntry for a path on disk.
func statEntry(p, name string) (FileEntry, error) {
	info, err := os.Stat(p)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Name:       name,
		IsDir:      info.IsDir(),
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	}, nil
}
