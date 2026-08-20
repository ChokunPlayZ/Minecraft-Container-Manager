package servers

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathTraversal(t *testing.T) {
	s := minimalStore(t.TempDir())
	bad := []string{
		"../",
		"../secret",
		"a/../../secret",
		"/etc/passwd",
		"/../etc/passwd",
		"..",
	}
	for _, rel := range bad {
		if _, err := s.resolvePath("srv", rel); err == nil {
			t.Fatalf("resolvePath(%q) expected error, got none", rel)
		}
	}
}

func TestResolvePathValid(t *testing.T) {
	dir := t.TempDir()
	s := minimalStore(dir)
	root := filepath.Join(dir, "servers", "srv")
	cases := []struct {
		rel string
		out string
	}{
		{"", root},
		{".", root},
		{"server.jar", filepath.Join(root, "server.jar")},
		{"worlds/nether/region", filepath.Join(root, "worlds", "nether", "region")},
		{"a/./b", filepath.Join(root, "a", "b")},
	}
	for _, c := range cases {
		got, err := s.resolvePath("srv", c.rel)
		if err != nil {
			t.Fatalf("resolvePath(%q) error: %v", c.rel, err)
		}
		if got != c.out {
			t.Fatalf("resolvePath(%q) = %q want %q", c.rel, got, c.out)
		}
	}
}

func TestListFilesEmptyAndMissing(t *testing.T) {
	s := minimalStore(t.TempDir())
	res, err := s.ListFiles("id", "worlds")
	if err != nil {
		t.Fatalf("ListFiles(missing) error: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("expected empty listing, got %d entries", len(res.Entries))
	}
}

func TestArchiveNestedDirAndUnzipRoundTrip(t *testing.T) {
	s := minimalStore(t.TempDir())
	root := filepath.Join(s.dataPath("id"), "worlds", "survival")
	if err := os.MkdirAll(filepath.Join(root, "region"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "level.dat"), []byte("leveldata"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "region", "r.0.0.mca"), []byte("regiondata"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	entry, err := s.Archive("id", "worlds/survival", "backup")
	if err != nil {
		t.Fatalf("Archive error: %v", err)
	}
	if entry.Name != "backup.zip" || !strings.HasSuffix(entry.Name, ".zip") {
		t.Fatalf("unexpected archive name: %q", entry.Name)
	}

	// Unzip into a fresh directory.
	count, err := s.Unzip("id", "backup.zip", "restored")
	if err != nil {
		t.Fatalf("Unzip error: %v", err)
	}
	if count < 3 {
		t.Fatalf("expected at least 3 entries extracted, got %d", count)
	}
	data, err := os.ReadFile(filepath.Join(s.dataPath("id"), "restored", "survival", "level.dat"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "leveldata" {
		t.Fatalf("extracted content mismatch: %q", data)
	}
}

func TestArchiveSingleFile(t *testing.T) {
	s := minimalStore(t.TempDir())
	if err := os.MkdirAll(s.dataPath("id"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.dataPath("id"), "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	entry, err := s.Archive("id", "readme.txt", "")
	if err != nil {
		t.Fatalf("Archive error: %v", err)
	}
	if entry.Name != "readme.txt.zip" {
		t.Fatalf("expected default name readme.txt.zip, got %q", entry.Name)
	}
}

func TestUnzipRejectsZipSlip(t *testing.T) {
	s := minimalStore(t.TempDir())
	root := s.dataPath("id")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	zpath := filepath.Join(root, "evil.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// This entry would escape the data directory if unzipped naively.
	w, _ := zw.Create("../../outside.txt")
	_, _ = w.Write([]byte("escaped"))
	_ = zw.Close()
	if err := os.WriteFile(zpath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	count, err := s.Unzip("id", "evil.zip", "dest")
	if err != nil {
		t.Fatalf("Unzip error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 extracted (all slip entries rejected), got %d", count)
	}
	if _, statErr := os.Stat(filepath.Join(s.dataPath("id"), "dest", "outside.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("zip-slip entry was written inside dest")
	}
	// Sanity: nothing was written outside the data dir.
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "outside.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("zip-slip entry escaped the data directory")
	}
}

func TestMkdirRenameDelete(t *testing.T) {
	s := minimalStore(t.TempDir())
	if _, err := s.Mkdir("id", "plugins"); err != nil {
		t.Fatalf("Mkdir error: %v", err)
	}
	if _, err := s.Mkdir("id", "plugins/sub"); err != nil {
		t.Fatalf("nested Mkdir error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.dataPath("id"), "plugins", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	renamed, err := s.Rename("id", "plugins/x.txt", "y.txt")
	if err != nil {
		t.Fatalf("Rename error: %v", err)
	}
	if renamed.Name != "y.txt" {
		t.Fatalf("renamed name = %q want y.txt", renamed.Name)
	}
	if _, err := os.Stat(filepath.Join(s.dataPath("id"), "plugins", "y.txt")); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if err := s.DeleteFile("id", "plugins"); err != nil {
		t.Fatalf("DeleteFile error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dataPath("id"), "plugins")); !os.IsNotExist(err) {
		t.Fatalf("directory should have been removed")
	}
}

func TestUnzipInvalidArchive(t *testing.T) {
	s := minimalStore(t.TempDir())
	if err := os.MkdirAll(s.dataPath("id"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.dataPath("id"), "bad.zip"), []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := s.Unzip("id", "bad.zip", ""); err == nil {
		t.Fatalf("expected error for invalid archive")
	}
}
