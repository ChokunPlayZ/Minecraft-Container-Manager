package servers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModDirForType(t *testing.T) {
	cases := []struct {
		serverType string
		want       ModType
		wantErr    bool
	}{
		{"paper", ModTypePlugins, false},
		{"spigot", ModTypePlugins, false},
		{"fabric", ModTypeMods, false},
		{"forge", ModTypeMods, false},
		{"neoforge", ModTypeMods, false},
		{"vanilla", ModTypeNone, true},
		{"unknown", ModTypeNone, true},
	}
	for _, c := range cases {
		got, err := modDirForType(c.serverType)
		if c.wantErr && err == nil {
			t.Fatalf("%s: expected error", c.serverType)
		}
		if !c.wantErr && err != nil {
			t.Fatalf("%s: unexpected error %v", c.serverType, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.serverType, got, c.want)
		}
	}
}

func TestValidModFileName(t *testing.T) {
	valid := []string{"foo.jar", "My_Mod-1.0.jar", "a.jar"}
	invalid := []string{"", ".", "..", "foo", "foo.txt", "../evil.jar", "/abs/evil.jar", "a\\evil.jar", "dir/evil.jar"}
	for _, v := range valid {
		if !validModFileName(v) {
			t.Fatalf("expected %q valid", v)
		}
	}
	for _, v := range invalid {
		if validModFileName(v) {
			t.Fatalf("expected %q invalid", v)
		}
	}
}

func TestModDisplayBase(t *testing.T) {
	cases := map[string]string{
		"foo.jar":            "foo",
		"foo.jar.disabled":   "foo",
		"My_Mod-1.0.jar":     "My_Mod-1.0",
		"x.jar.disabled.jar": "x.jar.disabled",
	}
	for in, want := range cases {
		if got := modDisplayBase(in); got != want {
			t.Fatalf("modDisplayBase(%q) = %q want %q", in, got, want)
		}
	}
}

func TestEnableDisableRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Disable: foo.jar -> foo.jar.disabled
	target := disable(t, dir, "foo")
	if target != "foo.jar.disabled" {
		t.Fatalf("disable returned %q", target)
	}
	if _, err := os.Stat(filepath.Join(dir, "foo.jar.disabled")); err != nil {
		t.Fatalf("disabled file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "foo.jar")); !os.IsNotExist(err) {
		t.Fatalf("enabled file should be gone")
	}

	// Enable: foo.jar.disabled -> foo.jar
	target = enable(t, dir, "foo")
	if target != "foo.jar" {
		t.Fatalf("enable returned %q", target)
	}
	if _, err := os.Stat(filepath.Join(dir, "foo.jar")); err != nil {
		t.Fatalf("enabled file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "foo.jar.disabled")); !os.IsNotExist(err) {
		t.Fatalf("disabled file should be gone")
	}
}

func disable(t *testing.T, dir, name string) string {
	t.Helper()
	base, ok := resolveModDirEntry(dir, name)
	if !ok {
		t.Fatalf("resolve %q failed", name)
	}
	target := base
	if !strings.HasSuffix(target, ".disabled") {
		target = base + ".disabled"
	}
	if err := os.Rename(filepath.Join(dir, base), filepath.Join(dir, target)); err != nil {
		t.Fatal(err)
	}
	return target
}

func enable(t *testing.T, dir, name string) string {
	t.Helper()
	base, ok := resolveModDirEntry(dir, name)
	if !ok {
		t.Fatalf("resolve %q failed", name)
	}
	target := strings.TrimSuffix(base, ".disabled")
	if err := os.Rename(filepath.Join(dir, base), filepath.Join(dir, target)); err != nil {
		t.Fatal(err)
	}
	return target
}
