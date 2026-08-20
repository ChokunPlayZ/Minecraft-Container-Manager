package servers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalStore returns a Store whose data directory is dir. These properties
// helpers are pure filesystem operations keyed off dataPath, so no DB is needed.
func minimalStore(dir string) *Store {
	return &Store{dataDir: dir}
}

func TestGetPropertiesMissing(t *testing.T) {
	s := minimalStore(t.TempDir())
	props, err := s.GetProperties("some-id")
	if err != nil {
		t.Fatalf("GetProperties(missing) returned error: %v", err)
	}
	if props.Exists {
		t.Fatalf("Expected Exists=false for missing file")
	}
	if props.Content != "" {
		t.Fatalf("Expected empty content, got %q", props.Content)
	}
}

func TestSaveThenGetRoundTrip(t *testing.T) {
	s := minimalStore(t.TempDir())
	const content = "# Minecraft server properties\nserver-port=25565\nmotd=A cool server\ndifficulty=normal\n"

	saved, err := s.SaveProperties("abc", content)
	if err != nil {
		t.Fatalf("SaveProperties returned error: %v", err)
	}
	if !saved.Exists || saved.Content != content {
		t.Fatalf("saved = %+v, want exists with original content", saved)
	}

	got, err := s.GetProperties("abc")
	if err != nil {
		t.Fatalf("GetProperties returned error: %v", err)
	}
	if !got.Exists || got.Content != content {
		t.Fatalf("round-trip mismatch: got %q (exists=%v) want %q", got.Content, got.Exists, content)
	}

	// The raw file bytes should match exactly, preserving comments/formatting.
	raw, err := os.ReadFile(filepath.Join(s.dataPath("abc"), "server.properties"))
	if err != nil {
		t.Fatalf("read raw file: %v", err)
	}
	if string(raw) != content {
		t.Fatalf("raw file content mismatch")
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	s := minimalStore(dir)
	// dataPath is dir/servers/<id>; its parent dir/servers does not exist yet.
	if _, err := os.Stat(filepath.Join(dir, "servers")); !os.IsNotExist(err) {
		t.Fatalf("expected servers dir to not exist yet")
	}
	if _, err := s.SaveProperties("xyz", "server-port=25565\n"); err != nil {
		t.Fatalf("SaveProperties should create parent dir, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dataPath("xyz"), "server.properties")); err != nil {
		t.Fatalf("expected properties file after save: %v", err)
	}
}

func TestReadPropsSkipsCommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.properties")
	content := "# comment\n\nserver-port=25565\nmotd=Hello World\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	props, err := readProps(path)
	if err != nil {
		t.Fatal(err)
	}
	if props["server-port"] != "25565" {
		t.Fatalf("server-port = %q", props["server-port"])
	}
	if props["motd"] != "Hello World" {
		t.Fatalf("motd = %q", props["motd"])
	}
	if strings.Contains(props["motd"], "\n") {
		t.Fatalf("expected comments/blank lines excluded")
	}
}
