package servers

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/google/uuid"
)

var (
	ErrWorldBusy        = errors.New("stop the server before changing or exporting worlds")
	ErrWorldConflict    = errors.New("a world or folder with this name already exists")
	ErrWorldUnsupported = errors.New("world management requires a Java game server")
)

type WorldValidationError struct{ Message string }

func (e *WorldValidationError) Error() string { return e.Message }
func worldInvalid(message string) error       { return &WorldValidationError{message} }

const worldConfigFile = ".mcm-world.json"
const MaxWorldUploadBytes int64 = 2 << 30
const maxWorldExtractBytes int64 = 8 << 30

var worldNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// WorldInput describes generation settings for a world which has not been generated.
type WorldInput struct {
	Name               string `json:"name"`
	Seed               string `json:"seed"`
	LevelType          string `json:"level_type"`
	GenerateStructures bool   `json:"generate_structures"`
}
type World struct {
	Name               string           `json:"name"`
	Active             bool             `json:"active"`
	Generated          bool             `json:"generated"`
	Seed               *string          `json:"seed"`
	LevelType          string           `json:"level_type"`
	GenerateStructures bool             `json:"generate_structures"`
	SizeBytes          int64            `json:"size_bytes"`
	ModifiedAt         time.Time        `json:"modified_at"`
	Version            string           `json:"version,omitempty"`
	MetadataError      bool             `json:"metadata_error"`
	Dimensions         []WorldDimension `json:"dimensions"`
}
type WorldDimension struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Regions int    `json:"regions"`
}
type WorldList struct {
	Worlds      []World `json:"worlds"`
	ActiveWorld string  `json:"active_world"`
	Editable    bool    `json:"editable"`
}

// World writes and server startup share a per-server lock. An import on one
// server does not block the rest of the fleet.
func (s *Store) lockWorld(id string, write bool) func() {
	value, _ := s.worldLocks.LoadOrStore(id, &sync.RWMutex{})
	mu := value.(*sync.RWMutex)
	if write {
		mu.Lock()
		return mu.Unlock
	}
	mu.RLock()
	return mu.RUnlock
}

func validWorldName(name string) error {
	if !worldNamePattern.MatchString(name) {
		return worldInvalid("Use 1–64 letters, numbers, dots, underscores or hyphens, starting with a letter or number.")
	}
	if strings.HasSuffix(name, "_nether") || strings.HasSuffix(name, "_the_end") {
		return worldInvalid("World names cannot end in _nether or _the_end; those names are reserved for dimensions.")
	}
	return nil
}
func validateWorldInput(in *WorldInput) error {
	if err := validWorldName(in.Name); err != nil {
		return err
	}
	in.Seed = strings.TrimSpace(in.Seed)
	if len(in.Seed) > 128 || strings.ContainsAny(in.Seed, "\r\n\x00") {
		return worldInvalid("Seeds must be at most 128 characters and contain no line breaks.")
	}
	if in.LevelType == "" {
		in.LevelType = "minecraft:normal"
	}
	switch in.LevelType {
	case "minecraft:normal", "minecraft:flat", "minecraft:large_biomes", "minecraft:amplified":
	default:
		return worldInvalid("Choose a supported world generator.")
	}
	return nil
}

// worldPath rejects symlinks at every component, including absent destinations.
func (s *Store) worldPath(id, rel string) (string, error) {
	if rel == "" {
		return s.dataPath(id), nil
	}
	if path.IsAbs(rel) || strings.Contains(rel, "\\") || path.Clean(rel) != rel || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", ErrInvalidPath
	}
	current := s.dataPath(id)
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", ErrInvalidPath
		}
	}
	return current, nil
}
func (s *Store) worldServer(ctx context.Context, id string, stopped bool) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return srv, err
	}
	switch srv.ServerType {
	case "velocity", "waterfall", "bungeecord", "geysermc", "limbo", "nanolimbo":
		return srv, ErrWorldUnsupported
	}
	if !stopped {
		return srv, nil
	}
	if srv.State != StateStopped && srv.State != StateError {
		return srv, ErrWorldBusy
	}
	if srv.ContainerID != "" {
		if isDockerNil(s.docker) {
			return srv, ErrWorldBusy
		}
		inspect, err := s.docker.Inspect(ctx, srv.ContainerID)
		if err != nil {
			return srv, ErrWorldBusy
		}
		switch inspect.Status {
		case "exited", "dead", "created":
		default:
			return srv, ErrWorldBusy
		}
	}
	if task := s.GetTaskProgress(id); task != nil && task.Stage != "completed" && task.Stage != "failed" {
		return srv, ErrWorldBusy
	}
	return srv, nil
}
func (s *Store) worldProperties(id string) (map[string]string, error) {
	p, err := s.worldPath(id, "server.properties")
	if err != nil {
		return nil, err
	}
	props, err := readProps(p)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return props, err
}
func activeWorld(props map[string]string) string {
	if props["level-name"] != "" {
		return props["level-name"]
	}
	return "world"
}
func (s *Store) worldRoots(id, name string) ([]string, error) {
	if err := validWorldName(name); err != nil {
		return nil, err
	}
	roots := []string{}
	for _, rel := range []string{name, name + "_nether", name + "_the_end"} {
		p, err := s.worldPath(id, rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, ErrWorldConflict
		}
		roots = append(roots, rel)
	}
	if len(roots) == 0 || roots[0] != name {
		return nil, ErrPathNotFound
	}
	return roots, nil
}
func (s *Store) world(id, name string, props map[string]string) (World, error) {
	roots, err := s.worldRoots(id, name)
	if err != nil {
		return World{}, err
	}
	w := World{Name: name, Active: activeWorld(props) == name, LevelType: "minecraft:normal", GenerateStructures: true, Dimensions: []WorldDimension{}}
	configured := false
	configPath, err := s.worldPath(id, name+"/"+worldConfigFile)
	if err != nil {
		return w, err
	}
	if data, err := os.ReadFile(configPath); err == nil {
		var in WorldInput
		if json.Unmarshal(data, &in) == nil && validateWorldInput(&in) == nil {
			configured = true
			w.Seed = &in.Seed
			w.LevelType = in.LevelType
			w.GenerateStructures = in.GenerateStructures
		}
	}
	dat, err := s.worldPath(id, name+"/level.dat")
	if err != nil {
		return w, err
	}
	if info, err := os.Stat(dat); err == nil && !info.IsDir() {
		w.Generated = true
		meta, err := readWorldMetadata(dat)
		if err != nil || !meta.WorldData {
			w.MetadataError = true
			w.Seed = nil
		} else {
			w.Seed = meta.Seed
			w.Version = meta.Version
		}
	}
	if !w.Generated && !configured {
		return w, ErrPathNotFound
	}
	if w.Generated && w.Seed == nil {
		// Recent Java versions keep world generation options in saved data.
		for _, rel := range []string{"data/minecraft/world_gen_settings.dat", "data/world_gen_settings.dat"} {
			p, pathErr := s.worldPath(id, name+"/"+rel)
			if pathErr != nil {
				return w, pathErr
			}
			if meta, readErr := readWorldMetadata(p); readErr == nil && meta.Seed != nil {
				w.Seed = meta.Seed
				break
			}
		}
	}
	// Walk once for size and actual dimension directories, including modded dimensions.
	dimensions := map[string]bool{}
	for _, root := range roots {
		dir, _ := s.worldPath(id, root)
		err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				return ErrInvalidPath
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(w.ModifiedAt) {
				w.ModifiedAt = info.ModTime()
			}
			if !d.IsDir() {
				w.SizeBytes += info.Size()
				return nil
			}
			if d.Name() == "region" {
				rel, _ := filepath.Rel(s.dataPath(id), filepath.Dir(p))
				rel = filepath.ToSlash(rel)
				dimensions[rel] = true
			}
			return nil
		})
		if err != nil {
			return w, err
		}
	}
	// The Overworld is selectable even before any region file is generated.
	dimensions[name] = true
	keys := make([]string, 0, len(dimensions))
	for k := range dimensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		label := strings.TrimPrefix(key, name+"/")
		switch key {
		case name:
			label = "Overworld"
		case name + "/DIM-1", name + "_nether/DIM-1", name + "_nether":
			label = "Nether"
		case name + "/DIM1", name + "_the_end/DIM1", name + "_the_end":
			label = "The End"
		}
		entries, err := os.ReadDir(filepath.Join(s.dataPath(id), filepath.FromSlash(key), "region"))
		if err != nil && !os.IsNotExist(err) {
			return w, err
		}
		count := 0
		for _, e := range entries {
			if regionPattern.MatchString(e.Name()) && !e.IsDir() {
				count++
			}
		}
		w.Dimensions = append(w.Dimensions, WorldDimension{ID: key, Name: label, Regions: count})
	}
	return w, nil
}
func (s *Store) ListWorlds(ctx context.Context, id string) (WorldList, error) {
	unlockWorld := s.lockWorld(id, false)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, false); err != nil {
		return WorldList{}, err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return WorldList{}, err
	}
	out := WorldList{Worlds: []World{}, ActiveWorld: activeWorld(props)}
	_, err = s.worldServer(ctx, id, true)
	out.Editable = err == nil
	entries, err := os.ReadDir(s.dataPath(id))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if !e.IsDir() || validWorldName(e.Name()) != nil {
			continue
		}
		base := filepath.Join(s.dataPath(id), e.Name())
		if _, a := os.Lstat(filepath.Join(base, "level.dat")); a != nil {
			if _, b := os.Lstat(filepath.Join(base, worldConfigFile)); b != nil {
				continue
			}
		}
		w, err := s.world(id, e.Name(), props)
		if err != nil {
			return out, err
		}
		out.Worlds = append(out.Worlds, w)
	}
	sort.Slice(out.Worlds, func(i, j int) bool {
		if out.Worlds[i].Active != out.Worlds[j].Active {
			return out.Worlds[i].Active
		}
		return out.Worlds[i].Name < out.Worlds[j].Name
	})
	return out, nil
}
func (s *Store) SaveWorld(ctx context.Context, id string, in WorldInput, create bool) (World, error) {
	if err := validateWorldInput(&in); err != nil {
		return World{}, err
	}
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return World{}, err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return World{}, err
	}
	dir, err := s.worldPath(id, in.Name)
	if err != nil {
		return World{}, err
	}
	if create {
		if err = os.MkdirAll(s.dataPath(id), 0755); err != nil {
			return World{}, err
		}
		for _, name := range []string{in.Name, in.Name + "_nether", in.Name + "_the_end"} {
			if _, err := os.Lstat(filepath.Join(s.dataPath(id), name)); !os.IsNotExist(err) {
				return World{}, ErrWorldConflict
			}
		}
		if err = os.Mkdir(dir, 0755); err != nil {
			return World{}, err
		}
	} else {
		w, err := s.world(id, in.Name, props)
		if err != nil {
			return w, err
		}
		if w.Generated {
			return w, worldInvalid("Existing terrain keeps its saved seed. Create a new world to use a different seed.")
		}
	}
	data, _ := json.Marshal(in)
	if err = atomicWorldFile(filepath.Join(dir, worldConfigFile), data); err != nil {
		if create {
			_ = os.Remove(dir)
		}
		return World{}, err
	}
	if activeWorld(props) == in.Name {
		if err = s.setActiveWorld(id, in); err != nil {
			return World{}, err
		}
	}
	return s.world(id, in.Name, props)
}

// Minecraft reads Java properties using escapes, including non-ASCII seeds.
func escapeWorldSeed(seed string) string {
	var out strings.Builder
	for _, r := range seed {
		if r == '\\' {
			out.WriteString(`\\`)
		} else if r < 32 || r > 126 {
			for _, unit := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(&out, `\u%04x`, unit)
			}
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func atomicWorldFile(filename string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(filename), ".mcm-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(0644)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filename)
}

// Update only generation keys, preserving comments and all unrelated properties.
func (s *Store) setActiveWorld(id string, in WorldInput) error {
	s.fileEditMu.Lock()
	defer s.fileEditMu.Unlock()
	filename, err := s.worldPath(id, "server.properties")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	updates := map[string]string{"level-name": in.Name, "level-seed": escapeWorldSeed(in.Seed), "level-type": in.LevelType, "generate-structures": fmt.Sprint(in.GenerateStructures)}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		if value, exists := updates[key]; ok && exists {
			lines[i] = key + "=" + value
			seen[key] = true
		}
	}
	for _, key := range []string{"level-name", "level-seed", "level-type", "generate-structures"} {
		if !seen[key] {
			lines = append(lines, key+"="+updates[key])
		}
	}
	return atomicWorldFile(filename, []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n"))
}
func (s *Store) ActivateWorld(ctx context.Context, id, name string) (World, error) {
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return World{}, err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return World{}, err
	}
	w, err := s.world(id, name, props)
	if err != nil {
		return w, err
	}
	seed := ""
	if w.Seed != nil {
		seed = *w.Seed
	}
	if err = s.setActiveWorld(id, WorldInput{Name: name, Seed: seed, LevelType: w.LevelType, GenerateStructures: w.GenerateStructures}); err != nil {
		return w, err
	}
	w.Active = true
	return w, nil
}

// Deletion retains all files in the server's recovery directory.
func (s *Store) DeleteWorld(ctx context.Context, id, name string) (string, error) {
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return "", err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return "", err
	}
	if activeWorld(props) == name {
		return "", worldInvalid("Switch to another world before deleting the active world.")
	}
	if _, err = s.world(id, name, props); err != nil {
		return "", err
	}
	roots, err := s.worldRoots(id, name)
	if err != nil {
		return "", err
	}
	recovery := ".mcm-world-trash/" + uuid.NewString()
	dst, err := s.worldPath(id, recovery)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(dst, 0755); err != nil {
		return "", err
	}
	moved := []string{}
	for _, root := range roots {
		if err = os.Rename(filepath.Join(s.dataPath(id), root), filepath.Join(dst, root)); err != nil {
			for _, r := range moved {
				_ = os.Rename(filepath.Join(dst, r), filepath.Join(s.dataPath(id), r))
			}
			return "", err
		}
		moved = append(moved, root)
	}
	return recovery, nil
}

// ExportWorld builds a complete ZIP before HTTP headers are sent. The caller removes it.
func (s *Store) ExportWorld(ctx context.Context, id, name string) (string, error) {
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return "", err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return "", err
	}
	w, err := s.world(id, name, props)
	if err != nil {
		return "", err
	}
	if !w.Generated {
		return "", worldInvalid("Start this world once before exporting it.")
	}
	roots, err := s.worldRoots(id, name)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "mcm-world-*.zip")
	if err != nil {
		return "", err
	}
	defer f.Close()
	z := zip.NewWriter(f)
	for _, root := range roots {
		err = filepath.WalkDir(filepath.Join(s.dataPath(id), root), func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return ErrInvalidPath
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() == "session.lock" || strings.HasPrefix(d.Name(), ".mcm-") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return ErrInvalidPath
			}
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(s.dataPath(id), p)
			h.Name = filepath.ToSlash(rel)
			h.Method = zip.Deflate
			out, err := z.CreateHeader(h)
			if err != nil {
				return err
			}
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(out, in)
			return err
		})
		if err != nil {
			break
		}
	}
	if closeErr := z.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func worldArchiveName(name string) string {
	for strings.HasPrefix(name, "./") {
		name = strings.TrimPrefix(name, "./")
	}
	return strings.TrimSuffix(name, "/")
}

// ImportWorld accepts a world folder, optionally wrapped in other directories,
// and matching Bukkit dimension folders. Extraction happens in staging first.
func (s *Store) ImportWorld(ctx context.Context, id, name, filename string) (World, error) {
	if err := validWorldName(name); err != nil {
		return World{}, err
	}
	unlockWorld := s.lockWorld(id, true)
	defer unlockWorld()
	if _, err := s.worldServer(ctx, id, true); err != nil {
		return World{}, err
	}
	props, err := s.worldProperties(id)
	if err != nil {
		return World{}, err
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return World{}, ErrInvalidArchive
	}
	defer z.Close()
	if len(z.File) > 100000 {
		return World{}, worldInvalid("World archives may contain at most 100,000 entries.")
	}
	var total uint64
	candidates := []string{}
	for _, f := range z.File {
		n := worldArchiveName(f.Name)
		if n == "" && f.FileInfo().IsDir() {
			continue
		}
		if n == "" || strings.ContainsAny(n, "\\\x00") || path.IsAbs(n) || path.Clean(n) != n || n == ".." || strings.HasPrefix(n, "../") || f.Mode()&os.ModeSymlink != 0 || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return World{}, ErrInvalidArchive
		}
		if f.UncompressedSize64 > uint64(maxWorldExtractBytes)-total {
			return World{}, worldInvalid("Unpacked worlds must be 8 GiB or smaller.")
		}
		total += f.UncompressedSize64
		if path.Base(n) == "level.dat" && !strings.HasPrefix(n, "__MACOSX/") {
			candidates = append(candidates, path.Dir(n))
		}
	}
	bases := []string{}
	for _, candidate := range candidates {
		companion := false
		for _, other := range candidates {
			if candidate == other+"_nether" || candidate == other+"_the_end" {
				companion = true
			}
		}
		if !companion {
			bases = append(bases, candidate)
		}
	}
	if len(bases) != 1 {
		return World{}, worldInvalid("Upload one Java world with a level.dat file. Include matching Nether and End folders if needed.")
	}
	base := bases[0]
	if base == "." {
		base = ""
	}
	if err = os.MkdirAll(s.dataPath(id), 0755); err != nil {
		return World{}, err
	}
	for _, root := range []string{name, name + "_nether", name + "_the_end"} {
		p, err := s.worldPath(id, root)
		if err != nil {
			return World{}, err
		}
		if _, err = os.Lstat(p); !os.IsNotExist(err) {
			return World{}, ErrWorldConflict
		}
	}
	staging, err := os.MkdirTemp(s.dataPath(id), ".mcm-import-*")
	if err != nil {
		return World{}, err
	}
	defer os.RemoveAll(staging)
	var written int64
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return World{}, err
		}
		n := worldArchiveName(f.Name)
		if n == "" {
			continue
		}
		dest := ""
		if base == "" {
			dest = path.Join(name, n)
		} else {
			for _, suffix := range []string{"", "_nether", "_the_end"} {
				prefix := base + suffix
				if n == prefix || strings.HasPrefix(n, prefix+"/") {
					dest = name + suffix + strings.TrimPrefix(n, prefix)
					break
				}
			}
		}
		if dest == "" || strings.HasPrefix(n, "__MACOSX/") || path.Base(dest) == "session.lock" || strings.HasPrefix(path.Base(dest), ".mcm-") {
			continue
		}
		target := filepath.Join(staging, filepath.FromSlash(dest))
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0755); err != nil {
				return World{}, err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return World{}, err
		}
		in, err := f.Open()
		if err != nil {
			return World{}, ErrInvalidArchive
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			in.Close()
			return World{}, ErrInvalidArchive
		}
		count, copyErr := io.Copy(out, io.LimitReader(in, maxWorldExtractBytes-written+1))
		written += count
		in.Close()
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || written > maxWorldExtractBytes {
			return World{}, ErrInvalidArchive
		}
	}
	if metadata, readErr := readWorldMetadata(filepath.Join(staging, name, "level.dat")); readErr != nil || !metadata.WorldData {
		return World{}, worldInvalid("The archive does not contain readable Java level.dat metadata.")
	}
	moved := []string{}
	for _, root := range []string{name, name + "_nether", name + "_the_end"} {
		if _, err = os.Stat(filepath.Join(staging, root)); os.IsNotExist(err) {
			continue
		}
		if err = os.Rename(filepath.Join(staging, root), filepath.Join(s.dataPath(id), root)); err != nil {
			for _, r := range moved {
				_ = os.Rename(filepath.Join(s.dataPath(id), r), filepath.Join(staging, r))
			}
			return World{}, err
		}
		moved = append(moved, root)
	}
	return s.world(id, name, props)
}
