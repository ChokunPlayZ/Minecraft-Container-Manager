package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failedRead struct{}

func (failedRead) Read(p []byte) (int, error) {
	copy(p, "partial")
	return 7, errors.New("interrupted upload")
}

func TestWritePreservesPreviousBytesOnFailure(t *testing.T) {
	root, err := OpenServer(t.TempDir(), "server")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.WriteFile("config.txt", []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, reader := range []interface{ Read([]byte) (int, error) }{failedRead{}, strings.NewReader(strings.Repeat("x", 20))} {
		if _, err = Write(context.Background(), root, "config.txt", reader, 10, 0644); err == nil {
			t.Fatal("accepted failed/oversized upload")
		}
		content, _ := root.ReadFile("config.txt")
		if string(content) != "previous" {
			t.Fatal("previous bytes lost")
		}
	}
}
func TestRootRejectsUnsafeIDsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"..", "a/b", "a\\b", "%2e%2e", "."} {
		if root, err := OpenServer(dir, id); err == nil {
			root.Close()
			t.Fatalf("accepted %q", id)
		}
	}
	root, err := OpenServer(dir, "server")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	outside := t.TempDir()
	if err = os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = root.Symlink(outside, "link"); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenRegular(root, "link/secret"); err == nil {
		t.Fatal("read outside root")
	}
	if _, err = Write(context.Background(), root, "link/secret", strings.NewReader("changed"), 100, 0644); err == nil {
		t.Fatal("wrote outside root")
	}
	content, _ := os.ReadFile(filepath.Join(outside, "secret"))
	if string(content) != "private" {
		t.Fatal("outside data changed")
	}
}
func TestPublishRollsBackDirectoryAndFiles(t *testing.T) {
	root, err := OpenServer(t.TempDir(), "server")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	root.MkdirAll("libraries/old", 0755)
	root.WriteFile("libraries/old/library.jar", []byte("old"), 0644)
	root.MkdirAll("stage/libraries/new", 0755)
	root.WriteFile("stage/libraries/new/library.jar", []byte("new"), 0644)
	undo, finish, err := PublishTree(root, "stage", []string{"libraries"})
	if err != nil {
		t.Fatal(err)
	}
	if err = undo(); err != nil {
		t.Fatal(err)
	}
	defer finish()
	data, err := root.ReadFile("libraries/old/library.jar")
	if err != nil || string(data) != "old" {
		t.Fatalf("rollback lost old tree: %s %v", data, err)
	}
	if _, err = root.Stat("libraries/new/library.jar"); !os.IsNotExist(err) {
		t.Fatal("rollback kept new file")
	}
}
