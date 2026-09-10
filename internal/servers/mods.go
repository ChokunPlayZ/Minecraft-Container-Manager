package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
	Name        string `json:"name"`                   // display name without extension
	File        string `json:"file"`                   // on-disk filename (may carry .disabled)
	Enabled     bool   `json:"enabled"`                // whether file is enabled
	ModID       string `json:"mod_id,omitempty"`       // canonical mod/plugin id from jar manifest
	Title       string `json:"title,omitempty"`        // human friendly name from jar manifest
	Version     string `json:"version,omitempty"`      // version from jar manifest
	SHA1        string `json:"sha1,omitempty"`         // SHA1 file hash
	Description string `json:"description,omitempty"`  // description from jar manifest
	ProjectID   string `json:"project_id,omitempty"`   // catalog project ID if known
	ProjectSlug string `json:"project_slug,omitempty"` // catalog project slug if known
	Provider    string `json:"provider,omitempty"`     // catalog provider (e.g. "modrinth")
}

// ModDownloadMeta stores catalog metadata associated with an installed mod.
type ModDownloadMeta struct {
	ProjectID   string `json:"project_id,omitempty"`
	ProjectSlug string `json:"project_slug,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

func modsMetaFile(modsDir string) string {
	return filepath.Join(modsDir, ".mcm_mods_meta.json")
}

func readModsMeta(modsDir string) map[string]ModDownloadMeta {
	data, err := os.ReadFile(modsMetaFile(modsDir))
	if err != nil {
		return make(map[string]ModDownloadMeta)
	}
	var res map[string]ModDownloadMeta
	if err := json.Unmarshal(data, &res); err != nil {
		return make(map[string]ModDownloadMeta)
	}
	return res
}

func writeModMeta(modsDir, filename string, meta ModDownloadMeta) {
	if meta.ProjectID == "" && meta.ProjectSlug == "" && meta.Provider == "" {
		return
	}
	m := readModsMeta(modsDir)
	base := modDisplayBase(filename)
	m[filename] = meta
	m[base] = meta
	data, err := json.MarshalIndent(m, "", "  ")
	if err == nil {
		_ = os.WriteFile(modsMetaFile(modsDir), data, 0o644)
	}
}

func deleteModMeta(modsDir, filename string) {
	m := readModsMeta(modsDir)
	base := modDisplayBase(filename)
	delete(m, filename)
	delete(m, filename+".disabled")
	delete(m, base)
	data, err := json.MarshalIndent(m, "", "  ")
	if err == nil {
		_ = os.WriteFile(modsMetaFile(modsDir), data, 0o644)
	}
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
	metaMap := readModsMeta(dir)
	items := make([]Mod, 0, len(byName))
	for _, m := range byName {
		fullPath := filepath.Join(dir, m.File)
		if manifest, err := inspectModJar(fullPath); err == nil {
			m.ModID = manifest.ModID
			m.Title = manifest.Title
			m.Version = manifest.Version
			m.SHA1 = manifest.SHA1
			m.Description = manifest.Description
		}
		if meta, ok := metaMap[m.File]; ok {
			m.ProjectID = meta.ProjectID
			m.ProjectSlug = meta.ProjectSlug
			m.Provider = meta.Provider
		} else if meta, ok := metaMap[modDisplayBase(m.File)]; ok {
			m.ProjectID = meta.ProjectID
			m.ProjectSlug = meta.ProjectSlug
			m.Provider = meta.Provider
		}
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
	manifest, _ := inspectModJar(target)
	return Mod{
		Name:        modDisplayBase(filename),
		File:        filename,
		Enabled:     true,
		ModID:       manifest.ModID,
		Title:       manifest.Title,
		Version:     manifest.Version,
		SHA1:        manifest.SHA1,
		Description: manifest.Description,
	}, nil
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
		targetPath := filepath.Join(dir, base)
		manifest, _ := inspectModJar(targetPath)
		metaMap := readModsMeta(dir)
		m := Mod{
			Name:        modDisplayBase(base),
			File:        base,
			Enabled:     enabled,
			ModID:       manifest.ModID,
			Title:       manifest.Title,
			Version:     manifest.Version,
			SHA1:        manifest.SHA1,
			Description: manifest.Description,
		}
		if meta, ok := metaMap[base]; ok {
			m.ProjectID = meta.ProjectID
			m.ProjectSlug = meta.ProjectSlug
			m.Provider = meta.Provider
		} else if meta, ok := metaMap[modDisplayBase(base)]; ok {
			m.ProjectID = meta.ProjectID
			m.ProjectSlug = meta.ProjectSlug
			m.Provider = meta.Provider
		}
		return m, nil
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
	targetPath := filepath.Join(dir, target)
	manifest, _ := inspectModJar(targetPath)
	metaMap := readModsMeta(dir)
	m := Mod{
		Name:        modDisplayBase(target),
		File:        target,
		Enabled:     enabled,
		ModID:       manifest.ModID,
		Title:       manifest.Title,
		Version:     manifest.Version,
		SHA1:        manifest.SHA1,
		Description: manifest.Description,
	}
	if meta, ok := metaMap[target]; ok {
		m.ProjectID = meta.ProjectID
		m.ProjectSlug = meta.ProjectSlug
		m.Provider = meta.Provider
	} else if meta, ok := metaMap[modDisplayBase(target)]; ok {
		m.ProjectID = meta.ProjectID
		m.ProjectSlug = meta.ProjectSlug
		m.Provider = meta.Provider
	}
	return m, nil
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
	deleteModMeta(dir, base)
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
		if fname == name || modDisplayBase(fname) == name || strings.TrimSuffix(fname, ".disabled") == name {
			return fname, true
		}
	}
	return "", false
}

// DownloadMod fetches an artifact file from a remote URL (e.g. Modrinth CDN) and saves it
// into the server's mod/plugin directory.
func (s *Store) DownloadMod(ctx context.Context, id, filename, downloadURL string, meta ...ModDownloadMeta) (Mod, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Mod{}, err
	}
	if !validModFileName(filename) {
		return Mod{}, ErrInvalidModName
	}
	u, err := url.Parse(downloadURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Mod{}, ErrInvalidPath
	}
	dir, _, err := s.modsPath(id, srv.ServerType)
	if err != nil {
		return Mod{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Mod{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return Mod{}, err
	}
	req.Header.Set("User-Agent", "mcm-panel/1.0 (https://github.com/mcm-panel/mcm)")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return Mod{}, ErrDownloadFailed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Mod{}, fmt.Errorf("%w: status %d", ErrDownloadFailed, resp.StatusCode)
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf(".tmp_%d_%s", time.Now().UnixNano(), filename))
	dst, err := os.Create(tmpFile)
	if err != nil {
		return Mod{}, err
	}
	_, werr := io.Copy(dst, io.LimitReader(resp.Body, maxDownloadBytes+1))
	cerr := dst.Close()
	if werr != nil {
		_ = os.Remove(tmpFile)
		return Mod{}, werr
	}
	if cerr != nil {
		_ = os.Remove(tmpFile)
		return Mod{}, cerr
	}

	if fi, ferr := os.Stat(tmpFile); ferr == nil && fi.Size() > maxDownloadBytes {
		_ = os.Remove(tmpFile)
		return Mod{}, fmt.Errorf("%w: file too large", ErrDownloadFailed)
	}

	target := filepath.Join(dir, filename)
	// If a .disabled version exists for the same base name, remove it so there's no duplicate
	disabledTarget := target + ".disabled"
	_ = os.Remove(disabledTarget)

	if err := os.Rename(tmpFile, target); err != nil {
		_ = os.Remove(tmpFile)
		return Mod{}, err
	}

	if len(meta) > 0 {
		writeModMeta(dir, filename, meta[0])
	}
	manifest, _ := inspectModJar(target)
	result := Mod{
		Name:        modDisplayBase(filename),
		File:        filename,
		Enabled:     true,
		ModID:       manifest.ModID,
		Title:       manifest.Title,
		Version:     manifest.Version,
		SHA1:        manifest.SHA1,
		Description: manifest.Description,
	}
	if len(meta) > 0 {
		result.ProjectID = meta[0].ProjectID
		result.ProjectSlug = meta[0].ProjectSlug
		result.Provider = meta[0].Provider
	}
	return result, nil
}
