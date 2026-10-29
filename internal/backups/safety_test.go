package backups

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreFailurePreservesCompleteTree(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	writeTestWorld(t, s, "server", "original")
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "world/level.dat", Mode: 0644, Size: 7})
	tw.Write([]byte("changed"))
	tw.WriteHeader(&tar.Header{Name: "../../escape", Mode: 0644, Size: 3})
	tw.Write([]byte("bad"))
	tw.Close()
	gz.Close()
	archive := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	os.WriteFile(archive, buffer.Bytes(), 0644)
	if err := s.extractWorld(ctx, "server", archive); err == nil {
		t.Fatal("unsafe archive accepted")
	}
	data, err := os.ReadFile(filepath.Join(s.serverDataDir("server"), "world/level.dat"))
	if err != nil || string(data) != "original" {
		t.Fatalf("failed restore changed world: %s %v", data, err)
	}
}

type boundedSeekReader struct {
	data    []byte
	offset  int64
	maxRead int
}

func (r *boundedSeekReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRead {
		r.maxRead = len(p)
	}
	if len(p) > 64<<10 {
		return 0, errors.New("upload buffered the entire stream")
	}
	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}
func (r *boundedSeekReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.offset = offset
	case io.SeekCurrent:
		r.offset += offset
	case io.SeekEnd:
		r.offset = int64(len(r.data)) + offset
	}
	return r.offset, nil
}

type backupRoundTrip func(*http.Request) (*http.Response, error)

func (f backupRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestS3UploadHashesAndStreamsBoundedChunks(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), 2<<20)
	source := &boundedSeekReader{data: payload}
	expected := sha256.Sum256(payload)
	client := newS3Client(S3Config{Endpoint: "https://storage.test", Bucket: "backup", Region: "test", AccessKey: "key", SecretKey: "secret"})
	if client.httpClient.Timeout == 0 {
		t.Fatal("unbounded cloud timeout")
	}
	client.httpClient.Transport = backupRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.ContentLength != int64(len(payload)) || req.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(expected[:]) {
			t.Fatal("incorrect stream signature or length")
		}
		var body bytes.Buffer
		_, err := io.CopyBuffer(struct{ io.Writer }{&body}, req.Body, make([]byte, 32<<10))
		data := body.Bytes()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(data, payload) {
			t.Fatal("stream changed")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	for name, reader := range map[string]io.Reader{
		"seekable": source,
		"stream":   struct{ io.Reader }{bytes.NewReader(payload)},
	} {
		t.Run(name, func(t *testing.T) {
			progress := 0
			if err := client.putObject(context.Background(), "snapshot", reader, func(n int) { progress += n }); err != nil {
				t.Fatal(err)
			}
			if progress != len(payload) {
				t.Fatalf("progress counted %d bytes; want uploaded size %d", progress, len(payload))
			}
		})
	}
}

func TestSnapshotDereferencesOnlyContainedRuntimeLinks(t *testing.T) {
	s, _ := openTestStore(t)
	writeTestWorld(t, s, "server", "world")
	dir := s.serverDataDir("server")
	if err := os.WriteFile(filepath.Join(dir, "forge-runtime.jar"), []byte("runtime"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/data/forge-runtime.jar", filepath.Join(dir, "server.jar")); err != nil {
		t.Fatal(err)
	}
	backup, err := s.Backup(context.Background(), "server", "runtime", "local")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Restore(context.Background(), backup.ID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "server.jar"))
	if err != nil || string(data) != "runtime" {
		t.Fatalf("runtime link not captured: %s %v", data, err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("secret"), 0644)
	os.Symlink(outside, filepath.Join(dir, "escape"))
	if _, err = s.Backup(context.Background(), "server", "unsafe", "local"); err == nil {
		t.Fatal("snapshot followed outside symlink")
	}
}
