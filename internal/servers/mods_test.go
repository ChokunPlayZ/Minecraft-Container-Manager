package servers

import (
	"context"
	"os"
	"path/filepath"
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
		{"youer", ModTypeMods, false},
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
	s, id := newModTestStore(t, "fabric")
	dir := filepath.Join(s.dataPath(id), "mods")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "foo.jar"), []byte("jar"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, false, true, true} {
		mod, err := s.SetModEnabled(context.Background(), id, "foo", enabled)
		if err != nil {
			t.Fatal(err)
		}
		want := "foo.jar"
		if !enabled {
			want += ".disabled"
		}
		if mod.Enabled != enabled || mod.File != want {
			t.Fatalf("incorrect toggle: %+v", mod)
		}
		if _, err = os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatal(err)
		}
		other := "foo.jar"
		if enabled {
			other += ".disabled"
		}
		if _, err = os.Stat(filepath.Join(dir, other)); !os.IsNotExist(err) {
			t.Fatal("duplicate toggle artifact")
		}
	}
}

func TestResolveModDirEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Chunky-1.4.28.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LuckPerms-5.4.jar.disabled"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Match by display base
	if got, ok := resolveModDirEntry(dir, "Chunky-1.4.28"); !ok || got != "Chunky-1.4.28.jar" {
		t.Fatalf("expected Chunky-1.4.28.jar, got %q (ok=%v)", got, ok)
	}
	// Match by full file name
	if got, ok := resolveModDirEntry(dir, "Chunky-1.4.28.jar"); !ok || got != "Chunky-1.4.28.jar" {
		t.Fatalf("expected Chunky-1.4.28.jar, got %q (ok=%v)", got, ok)
	}

	// Match disabled by display base
	if got, ok := resolveModDirEntry(dir, "LuckPerms-5.4"); !ok || got != "LuckPerms-5.4.jar.disabled" {
		t.Fatalf("expected LuckPerms-5.4.jar.disabled, got %q (ok=%v)", got, ok)
	}
	// Match disabled by full filename
	if got, ok := resolveModDirEntry(dir, "LuckPerms-5.4.jar.disabled"); !ok || got != "LuckPerms-5.4.jar.disabled" {
		t.Fatalf("expected LuckPerms-5.4.jar.disabled, got %q (ok=%v)", got, ok)
	}
	// Match disabled by jar filename without .disabled
	if got, ok := resolveModDirEntry(dir, "LuckPerms-5.4.jar"); !ok || got != "LuckPerms-5.4.jar.disabled" {
		t.Fatalf("expected LuckPerms-5.4.jar.disabled, got %q (ok=%v)", got, ok)
	}
}
