package servers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeLogFixture(t *testing.T, s *Store, name string, data []byte) string {
	t.Helper()
	file := filepath.Join(s.dataPath("id"), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func gzipLog(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarLogs(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := tar.NewWriter(&buf)
	for _, name := range []string{"./logs/first.log", "logs/second.log.gz", "../unsafe.log", "/absolute.log", "world/level.dat"} {
		data := []byte("[Server thread/INFO]: " + name + "\n")
		if strings.HasSuffix(name, ".gz") {
			data = gzipLog(t, data)
		}
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), ModTime: time.Unix(100, 0)}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteHeader(&tar.Header{Name: "link.log", Typeflag: tar.TypeSymlink, Linkname: "logs/first.log"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestListServerLogs(t *testing.T) {
	s := minimalStore(t.TempDir())
	ctx := context.Background()
	files, err := s.ListServerLogs(ctx, "id")
	if err != nil || files == nil || len(files) != 0 {
		t.Fatalf("missing logs = %v, %v", files, err)
	}
	latest := writeLogFixture(t, s, "logs/latest.log", []byte("live"))
	if err := os.Chtimes(latest, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"logs/2026-10-26-1.log.gz", "logs/old/saved.tar.gz", "crash-reports/crash.txt", "server.properties", "logs/ignored.json"} {
		writeLogFixture(t, s, name, []byte("fixture"))
	}
	if err := os.Symlink(latest, filepath.Join(s.dataPath("id"), "logs", "link.log")); err != nil {
		t.Fatal(err)
	}
	files, err = s.ListServerLogs(ctx, "id")
	if err != nil || len(files) != 4 || files[0].Path != "logs/latest.log" {
		t.Fatalf("listing = %+v, %v", files, err)
	}
	for _, file := range files {
		if file.Archive != isLogArchive(file.Path) {
			t.Fatalf("wrong archive flag: %+v", file)
		}
	}
}

func TestReadServerLogsWithoutExtraction(t *testing.T) {
	s := minimalStore(t.TempDir())
	ctx := context.Background()
	const content = "[Server thread/INFO]: Ready\n[Server thread/WARN]: Can't keep up!\n"
	writeLogFixture(t, s, "logs/latest.log", []byte(content))
	writeLogFixture(t, s, "logs/2026-10-25-1.log.gz", gzipLog(t, []byte(content)))
	for _, rel := range []string{"logs/latest.log", "logs/2026-10-25-1.log.gz"} {
		preview, err := s.ReadServerLog(ctx, "id", rel, "")
		if err != nil || preview.Content != content || preview.Truncated {
			t.Fatalf("read %s = %+v, %v", rel, preview, err)
		}
	}
	tarData := tarLogs(t)
	for _, name := range []string{"logs/old.tar", "logs/old.tar.gz", "logs/old.tgz"} {
		data := tarData
		if name != "logs/old.tar" {
			data = gzipLog(t, data)
		}
		writeLogFixture(t, s, name, data)
		files, err := s.ListLogArchive(ctx, "id", name)
		if err != nil || len(files) != 2 {
			t.Fatalf("archive members = %+v, %v", files, err)
		}
		for _, member := range []string{"logs/first.log", "logs/second.log.gz"} {
			preview, err := s.ReadServerLog(ctx, "id", name, member)
			if err != nil || !strings.Contains(preview.Content, "INFO") || preview.Truncated {
				t.Fatalf("archive preview = %+v, %v", preview, err)
			}
		}
		if _, err := s.ReadServerLog(ctx, "id", name, "missing.log"); !errors.Is(err, ErrPathNotFound) {
			t.Fatalf("missing member = %v", err)
		}
	}
	// All archive members remain virtual; only fixture files exist on disk.
	entries, err := os.ReadDir(filepath.Join(s.dataPath("id"), "logs"))
	if err != nil || len(entries) != 5 {
		t.Fatalf("archive reads wrote files: %v, %v", entries, err)
	}
}

func TestServerLogPreviewsAreBounded(t *testing.T) {
	s := minimalStore(t.TempDir())
	data := bytes.Repeat([]byte("x"), maxLogPreviewBytes+100)
	for _, name := range []string{"logs/large.log", "logs/large.log.gz"} {
		payload := data
		if strings.HasSuffix(name, ".gz") {
			payload = gzipLog(t, payload)
		}
		writeLogFixture(t, s, name, payload)
		preview, err := s.ReadServerLog(context.Background(), "id", name, "")
		if err != nil || !preview.Truncated || len(preview.Content) != maxLogPreviewBytes || preview.LimitBytes != maxLogPreviewBytes {
			t.Fatalf("preview limit: size=%d, truncated=%v, err=%v", len(preview.Content), preview.Truncated, err)
		}
	}
}

func TestServerLogsRejectUnsafePathsAndBadArchives(t *testing.T) {
	s := minimalStore(t.TempDir())
	ctx := context.Background()
	writeLogFixture(t, s, "logs/latest.log", []byte("log"))
	for _, name := range []string{"../outside.log", "/logs/latest.log", "server.properties", "logs/../latest.log", "logs/ignored.json"} {
		if _, err := s.ReadServerLog(ctx, "id", name, ""); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("path %q = %v", name, err)
		}
	}
	for _, id := range []string{"..", ".", "../id", "/id"} {
		if _, err := s.ListServerLogs(ctx, id); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("id %q = %v", id, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.log")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.dataPath("id"), "logs", "escape.log")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadServerLog(ctx, "id", "logs/escape.log", ""); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("symlink read = %v", err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(s.dataPath("id"), "logs", "escape")); err != nil {
		t.Fatal(err)
	}
	if preview, err := s.ReadServerLog(ctx, "id", "logs/escape/outside.log", ""); err == nil || strings.Contains(preview.Content, "secret") {
		t.Fatalf("parent symlink escaped root: %+v, %v", preview, err)
	}
	writeLogFixture(t, s, "logs/bad.log.gz", []byte("not gzip"))
	if _, err := s.ReadServerLog(ctx, "id", "logs/bad.log.gz", ""); !errors.Is(err, ErrInvalidLogArchive) {
		t.Fatalf("invalid gzip = %v", err)
	}
	writeLogFixture(t, s, "logs/bad.tar.gz", gzipLog(t, []byte("not tar")))
	if _, err := s.ListLogArchive(ctx, "id", "logs/bad.tar.gz"); !errors.Is(err, ErrInvalidLogArchive) {
		t.Fatalf("invalid tar = %v", err)
	}
	if _, err := s.ReadServerLog(ctx, "id", "logs/missing.log", ""); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing log = %v", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ReadServerLog(ctx, "id", "logs/latest.log", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v", err)
	}
}
