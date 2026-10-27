package servers

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestWriteAndReadFileContentRoundTrip(t *testing.T) {
	s := minimalStore(t.TempDir())
	const rel = "config/ops.yml"
	const content = "ops:\n  - uuid: abc\n    name: steve\n    level: 4\n"

	entry, err := s.WriteFileContent("id", rel, content)
	if err != nil {
		t.Fatalf("WriteFileContent error: %v", err)
	}
	if entry.Name != "ops.yml" || entry.IsDir {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	// The write must create missing parent directories.
	if _, err := os.Stat(filepath.Join(s.dataPath("id"), "config", "ops.yml")); err != nil {
		t.Fatalf("written file missing: %v", err)
	}

	got, err := s.ReadFileContent("id", rel)
	if err != nil {
		t.Fatalf("ReadFileContent error: %v", err)
	}
	if got != content {
		t.Fatalf("content mismatch:\n got %q\nwant %q", got, content)
	}
}

func TestFileEditorRejectsChangedContent(t *testing.T) {
	s := minimalStore(t.TempDir())
	const rel = "config/settings.yml"
	if _, err := s.WriteFileContent("id", rel, "pvp: true\n"); err != nil {
		t.Fatal(err)
	}
	opened, err := s.ReadFileSnapshot("id", rel)
	if err != nil || !opened.Exists || opened.Revision == "" {
		t.Fatalf("snapshot = %+v, err = %v", opened, err)
	}
	file := filepath.Join(s.dataPath("id"), filepath.FromSlash(rel))
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	// Content changes must be detected even if size and modification time match.
	const external = "pvp: nope\n"
	if err := os.WriteFile(file, []byte(external), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteFileContent("id", rel, "my edits", opened.Revision)
	var conflict *FileConflictError
	if !errors.As(err, &conflict) || conflict.Current.Content != external || !conflict.Current.Exists {
		t.Fatalf("expected current file in conflict, got %v", err)
	}
	got, err := s.ReadFileContent("id", rel)
	if err != nil || got != external {
		t.Fatalf("conflict overwrote file: %q, %v", got, err)
	}
	// Replacing the reviewed version is allowed, and returns the next revision.
	saved, err := s.WriteFileContent("id", rel, "my edits", conflict.Current.Revision)
	if err != nil || saved.Revision != fileRevision("my edits") {
		t.Fatalf("reviewed save: %+v, %v", saved, err)
	}
	if _, err := s.WriteFileContent("id", rel, "next edits", saved.Revision); err != nil {
		t.Fatalf("subsequent save: %v", err)
	}
}

func TestFileEditorCreationAndDeletionConflicts(t *testing.T) {
	s := minimalStore(t.TempDir())
	entry, err := s.WriteFileContent("id", "empty.txt", "", "")
	if err != nil || entry.Revision == "" {
		t.Fatalf("create empty file: %+v, %v", entry, err)
	}
	_, err = s.WriteFileContent("id", "empty.txt", "new file", "")
	var conflict *FileConflictError
	if !errors.As(err, &conflict) || !conflict.Current.Exists || conflict.Current.Content != "" {
		t.Fatalf("expected collision with existing empty file: %v", err)
	}
	file := filepath.Join(s.dataPath("id"), "empty.txt")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	_, err = s.WriteFileContent("id", "empty.txt", "restore", entry.Revision)
	if !errors.As(err, &conflict) || conflict.Current.Exists || conflict.Current.Revision != "" {
		t.Fatalf("expected deleted file conflict: %v", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file was recreated without review: %v", err)
	}
	if _, err := s.WriteFileContent("id", "empty.txt", "restore", conflict.Current.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestFileEditorSerializesConditionalSaves(t *testing.T) {
	s := minimalStore(t.TempDir())
	entry, err := s.WriteFileContent("id", "config.txt", "original")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, text := range []string{"first edit", "second edit"} {
		wg.Add(1)
		go func(content string) {
			defer wg.Done()
			_, err := s.WriteFileContent("id", "config.txt", content, entry.Revision)
			results <- err
		}(text)
	}
	wg.Wait()
	close(results)
	saved, conflicted := 0, 0
	for err := range results {
		var conflict *FileConflictError
		switch {
		case err == nil:
			saved++
		case errors.As(err, &conflict):
			conflicted++
		default:
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicted != 1 {
		t.Fatalf("saved=%d conflicted=%d, want one each", saved, conflicted)
	}
}

func TestReadFileContentRejectsDirectoryAndTooLarge(t *testing.T) {
	s := minimalStore(t.TempDir())
	if err := os.MkdirAll(filepath.Join(s.dataPath("id"), "worlds"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := s.ReadFileContent("id", "worlds"); err == nil {
		t.Fatalf("expected error reading a directory")
	}

	big := filepath.Join(s.dataPath("id"), "huge.bin")
	if err := os.WriteFile(big, bytes.Repeat([]byte("x"), int(maxEditBytes)+1), 0o644); err != nil {
		t.Fatalf("write huge file: %v", err)
	}
	if _, err := s.ReadFileContent("id", "huge.bin"); err == nil {
		t.Fatalf("expected error for oversized file")
	}
}

func TestWriteFileContentRejectsTraversal(t *testing.T) {
	s := minimalStore(t.TempDir())
	bad := []string{
		"",
		".",
		"..",
		"../escape.txt",
		"a/../../escape.txt",
		"/etc/passwd",
	}
	for _, rel := range bad {
		if _, err := s.WriteFileContent("id", rel, "x"); err == nil {
			t.Fatalf("WriteFileContent(%q) expected error, got none", rel)
		}
	}
}

func TestUploadFileSavesIntoDir(t *testing.T) {
	s := minimalStore(t.TempDir())
	entry, err := s.UploadFile("id", "plugins", "my-plugin.jar", strings.NewReader("jarbytes"))
	if err != nil {
		t.Fatalf("UploadFile error: %v", err)
	}
	if entry.Name != "my-plugin.jar" || entry.IsDir {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	data, err := os.ReadFile(filepath.Join(s.dataPath("id"), "plugins", "my-plugin.jar"))
	if err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
	if string(data) != "jarbytes" {
		t.Fatalf("uploaded content mismatch: %q", data)
	}
}

func TestUploadFileRejectsTraversal(t *testing.T) {
	s := minimalStore(t.TempDir())
	bad := []string{"", ".", "..", "../escape.jar", "a/../../escape.jar", "plugins/weird"}
	for _, name := range bad {
		if _, err := s.UploadFile("id", "plugins", name, strings.NewReader("x")); err == nil {
			t.Fatalf("UploadFile(%q) expected error, got none", name)
		}
	}
}
