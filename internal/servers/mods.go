package servers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ModType identifies the artifact directory a server uses.
type ModType string

const (
	// ModTypeNone is returned for server types that load no external mods.
	ModTypeNone    ModType = ""
	ModTypeMods    ModType = "mods"
	ModTypePlugins ModType = "plugins"
)

// ErrInvalidModName is returned when a mod/plugin filename is unsafe or lacks a
// valid .jar extension.
var ErrInvalidModName = errors.New("invalid mod/plugin filename")

// ErrUnsupportedMods is returned when a server type has no mod/plugin directory.
var ErrUnsupportedMods = errors.New("server type does not support mods/plugins")

// Mod describes a single installed artifact file.
type Mod struct {
	Name    string `json:"name"` // display name without extension
	File    string `json:"file"` // on-disk filename (may carry .disabled)
	Enabled bool   `json:"enabled"`
}

// ModListResult is a listing of a server's mods or plugins.
type ModListResult struct {
	Type  string `json:"type"`
	Items []Mod  `json:"items"`
}

// modDirForType maps a server type to its artifact directory.
func modDirForType(serverType string) (ModType, error) {
	switch strings.ToLower(serverType) {
	case "paper", "spigot":
		return ModTypePlugins, nil
	case "fabric", "forge", "neoforge":
		return ModTypeMods, nil
	case "vanilla":
		return ModTypeNone, ErrUnsupportedMods
	default:
		return ModTypeNone, ErrUnsupportedMods
	}
}

// validModFileName reports whether name is a safe artifact filename: a non-empty
// basename ending in .jar with no path separators or traversal segments.
func validModFileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(name), ".jar")
}

// modDisplayBase returns the display name of a file, stripping a trailing
// .disabled marker and .jar extension.
func modDisplayBase(file string) string {
	base := file
	if strings.HasSuffix(base, ".disabled") {
		base = strings.TrimSuffix(base, ".disabled")
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return base
}

func (s *Store) modsPath(id, serverType string) (string, ModType, error) {
	t, err := modDirForType(serverType)
	if err != nil {
		return "", ModTypeNone, err
	}
	return filepath.Join(s.dataPath(id), string(t)), t, nil
}

// ListMods returns the mods or plugins installed for a server.
func (s *Store) ListMods(ctx context.Context, id string) (ModListResult, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return ModListResult{}, err
	}
	dir, t, err := s.modsPath(id, srv.ServerType)
	if err != nil {
		return ModListResult{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ModListResult{Type: string(t), Items: []Mod{}}, nil
		}
		return ModListResult{}, err
	}
	byName := map[string]*Mod{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		if !strings.HasSuffix(strings.ToLower(fname), ".jar") &&
			!strings.HasSuffix(strings.ToLower(fname), ".jar.disabled") {
			continue
		}
		base := modDisplayBase(fname)
		enabled := !strings.HasSuffix(fname, ".disabled")
		if m, ok := byName[base]; ok {
			// Prefer the enabled entry when both exist.
			if enabled {
				m.File = fname
				m.Enabled = true
			}
			continue
		}
		byName[base] = &Mod{Name: base, File: fname, Enabled: enabled}
	}
	items := make([]Mod, 0, len(byName))
	for _, m := range byName {
		items = append(items, *m)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return ModListResult{Type: string(t), Items: items}, nil
}

// UploadMod writes an uploaded artifact into the server's mod/plugin directory.
func (s *Store) UploadMod(ctx context.Context, id, filename string, r io.Reader) (Mod, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Mod{}, err
	}
	if !validModFileName(filename) {
		return Mod{}, ErrInvalidModName
	}
	dir, _, err := s.modsPath(id, srv.ServerType)
	if err != nil {
		return Mod{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Mod{}, err
	}
	target := filepath.Join(dir, filename)
	dst, err := os.Create(target)
	if err != nil {
		return Mod{}, err
	}
	_, werr := io.Copy(dst, r)
	cerr := dst.Close()
	if werr != nil {
		return Mod{}, werr
	}
	if cerr != nil {
		return Mod{}, cerr
	}
	return Mod{Name: modDisplayBase(filename), File: filename, Enabled: true}, nil
}

// SetModEnabled enables or disables a mod/plugin by renaming the underlying file
// between foo.jar and foo.jar.disabled.
func (s *Store) SetModEnabled(ctx context.Context, id, name string, enabled bool) (Mod, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Mod{}, err
	}
	dir, _, err := s.modsPath(id, srv.ServerType)
	if err != nil {
		return Mod{}, err
	}
	base, ok := resolveModDirEntry(dir, name)
	if !ok {
		return Mod{}, fmt.Errorf("mod not found: %s", name)
	}
	if enabled != !strings.HasSuffix(base, ".disabled") {
		// Nothing to change.
		return Mod{Name: modDisplayBase(base), File: base, Enabled: enabled}, nil
	}
	var target string
	if enabled {
		target = strings.TrimSuffix(base, ".disabled")
	} else {
		target = base + ".disabled"
	}
	if err := os.Rename(filepath.Join(dir, base), filepath.Join(dir, target)); err != nil {
		return Mod{}, err
	}
	return Mod{Name: modDisplayBase(target), File: target, Enabled: enabled}, nil
}

// DeleteMod removes an installed mod/plugin file.
func (s *Store) DeleteMod(ctx context.Context, id, name string) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	dir, _, err := s.modsPath(id, srv.ServerType)
	if err != nil {
		return err
	}
	base, ok := resolveModDirEntry(dir, name)
	if !ok {
		return fmt.Errorf("mod not found: %s", name)
	}
	return os.Remove(filepath.Join(dir, base))
}

// resolveModDirEntry finds an on-disk file whose display name matches name,
// returning the (possibly .disabled) filename. It only matches files that would
// be listed by ListMods.
func resolveModDirEntry(dir, name string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fname := e.Name()
		if !strings.HasSuffix(strings.ToLower(fname), ".jar") &&
			!strings.HasSuffix(strings.ToLower(fname), ".jar.disabled") {
			continue
		}
		if modDisplayBase(fname) == name {
			return fname, true
		}
	}
	return "", false
}
