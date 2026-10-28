package servers

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
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
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/storage"
)

// ModpackFormat describes the packaging standard of a modpack.
type ModpackFormat string

const (
	ModpackFormatModrinth   ModpackFormat = "modrinth"
	ModpackFormatCurseForge ModpackFormat = "curseforge"
	ModpackFormatGeneric    ModpackFormat = "generic"
)

var (
	// ErrInvalidModpack is returned when an archive cannot be parsed as a valid modpack.
	ErrInvalidModpack = errors.New("invalid or unsupported modpack archive")
	// ErrModpackNotFound is returned when querying for a modpack that is not installed.
	ErrModpackNotFound = errors.New("no modpack installed")
	// ErrModpackLocked is returned when attempting to change or uninstall a modpack on a server created with a modpack.
	ErrModpackLocked = errors.New("server is locked to its initial modpack and cannot be changed")
)

// ModpackManifest describes inspected metadata from a modpack archive or online entry.
type ModpackManifest struct {
	Format            ModpackFormat `json:"format"`
	Name              string        `json:"name"`
	Version           string        `json:"version"`
	Summary           string        `json:"summary,omitempty"`
	Author            string        `json:"author,omitempty"`
	MinecraftVersion  string        `json:"minecraft_version"`
	Loader            string        `json:"loader"`
	LoaderVersion     string        `json:"loader_version"`
	TotalFiles        int           `json:"total_files"`
	ServerFiles       int           `json:"server_files"`
	ClientOnlyFiles   int           `json:"client_only_files"`
	UserRequiredFiles []string      `json:"user_required_files,omitempty"`
	IconURL           string        `json:"icon_url,omitempty"`
}

// InstalledModpack records the details of a modpack currently installed on a server.
type InstalledModpack struct {
	Name               string        `json:"name"`
	Version            string        `json:"version"`
	Summary            string        `json:"summary,omitempty"`
	Author             string        `json:"author,omitempty"`
	Format             ModpackFormat `json:"format"`
	MinecraftVersion   string        `json:"minecraft_version"`
	Loader             string        `json:"loader"`
	LoaderVersion      string        `json:"loader_version"`
	InstalledAt        string        `json:"installed_at"`
	Source             string        `json:"source"` // "upload", "modrinth", "curseforge", "url"
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectSlug        string        `json:"project_slug,omitempty"`
	InstalledFiles     []string      `json:"installed_files"`
	UserRequiredFiles  []string      `json:"user_required_files,omitempty"`
	IconURL            string        `json:"icon_url,omitempty"`
	CreatedWithModpack bool          `json:"created_with_modpack,omitempty"`
}

// InstallModpackOpts contains parameters controlling modpack installation.
type InstallModpackOpts struct {
	Source              string `json:"source"` // "upload", "url", "modrinth", "curseforge"
	URL                 string `json:"url,omitempty"`
	ProjectID           string `json:"project_id,omitempty"`
	ProjectSlug         string `json:"project_slug,omitempty"`
	VersionID           string `json:"version_id,omitempty"`
	AutoConfigureServer bool   `json:"auto_configure_server"`
	CurseForgeAPIKey    string `json:"curseforge_api_key,omitempty"`
	CreatedWithModpack  bool   `json:"created_with_modpack,omitempty"`
}

// Internal modrinth.index.json representation
type modrinthIndex struct {
	FormatVersion int                 `json:"formatVersion"`
	Game          string              `json:"game"`
	VersionID     string              `json:"versionId"`
	Name          string              `json:"name"`
	Summary       string              `json:"summary"`
	Files         []modrinthIndexFile `json:"files"`
	Dependencies  map[string]string   `json:"dependencies"`
}

type modrinthIndexFile struct {
	Path   string            `json:"path"`
	Hashes map[string]string `json:"hashes"`
	Env    struct {
		Client string `json:"client"`
		Server string `json:"server"`
	} `json:"env"`
	Downloads []string `json:"downloads"`
	FileSize  int64    `json:"fileSize"`
}

// Internal CurseForge manifest.json representation
type curseForgeManifest struct {
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	ManifestType    string `json:"manifestType"`
	ManifestVersion int    `json:"manifestVersion"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Author          string `json:"author"`
	Files           []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	} `json:"files"`
	Overrides string `json:"overrides"`
}

func modpackMetaFile(dataDir string) string {
	return filepath.Join(dataDir, ".mcm_modpack.json")
}

// InspectModpackArchive inspects a zip archive reader and extracts modpack metadata.
func InspectModpackArchive(r io.ReaderAt, size int64, fallbackName ...string) (*ModpackManifest, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
	}

	if err = validatePackZIP(zr); err != nil {
		return nil, err
	}

	// 1. Check for Modrinth .mrpack index
	for _, f := range zr.File {
		if f.Name == "modrinth.index.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			var idx modrinthIndex
			if err := json.NewDecoder(rc).Decode(&idx); err != nil {
				return nil, fmt.Errorf("%w: failed to decode modrinth.index.json", ErrInvalidModpack)
			}
			return inspectModrinthIndex(&idx), nil
		}
	}

	// 2. Check for CurseForge manifest.json
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			var man curseForgeManifest
			if err := json.NewDecoder(rc).Decode(&man); err != nil {
				return nil, fmt.Errorf("%w: failed to decode manifest.json", ErrInvalidModpack)
			}
			if man.ManifestType == "minecraftModpack" || len(man.Files) > 0 || len(man.Minecraft.ModLoaders) > 0 {
				return inspectCurseForgeManifest(&man), nil
			}
		}
	}

	// 3. Fallback: Generic zip modpack (contains mods/ directory or jar files)
	name := "Modpack"
	if len(fallbackName) > 0 && fallbackName[0] != "" {
		name = strings.TrimSuffix(filepath.Base(fallbackName[0]), filepath.Ext(fallbackName[0]))
		name = strings.ReplaceAll(name, "_", " ")
		name = strings.ReplaceAll(name, "-", " ")
	}

	jarCount := 0
	hasModsDir := false
	for _, f := range zr.File {
		clean := filepath.ToSlash(f.Name)
		if strings.HasSuffix(strings.ToLower(clean), ".jar") {
			jarCount++
		}
		if strings.HasPrefix(clean, "mods/") || strings.Contains(clean, "/mods/") {
			hasModsDir = true
		}
	}

	if jarCount == 0 && !hasModsDir {
		return nil, fmt.Errorf("%w: no mods or modpack index found in archive", ErrInvalidModpack)
	}

	return &ModpackManifest{
		Format:           ModpackFormatGeneric,
		Name:             name,
		Version:          "1.0",
		MinecraftVersion: "",
		Loader:           "fabric", // sensible default for generic server packs
		LoaderVersion:    "",
		TotalFiles:       jarCount,
		ServerFiles:      jarCount,
		ClientOnlyFiles:  0,
	}, nil
}

func inspectModrinthIndex(idx *modrinthIndex) *ModpackManifest {
	mcVersion := idx.Dependencies["minecraft"]
	loader := "fabric"
	loaderVersion := ""

	if v, ok := idx.Dependencies["fabric-loader"]; ok {
		loader = "fabric"
		loaderVersion = v
	} else if v, ok := idx.Dependencies["forge"]; ok {
		loader = "forge"
		loaderVersion = v
	} else if v, ok := idx.Dependencies["neoforge"]; ok {
		loader = "neoforge"
		loaderVersion = v
	} else if v, ok := idx.Dependencies["quilt-loader"]; ok {
		loader = "fabric"
		loaderVersion = v
	}

	serverFiles := 0
	clientOnly := 0
	var userRequiredFiles []string
	for _, f := range idx.Files {
		isClient := strings.ToLower(f.Env.Server) == "unsupported"
		if !isClient {
			if client, _ := IsClientOnlyFilename(f.Path); client {
				isClient = true
			}
		}
		if isClient {
			clientOnly++
		} else {
			serverFiles++
			if len(f.Downloads) == 0 {
				userRequiredFiles = append(userRequiredFiles, f.Path)
			}
		}
	}

	return &ModpackManifest{
		Format:            ModpackFormatModrinth,
		Name:              idx.Name,
		Version:           idx.VersionID,
		Summary:           idx.Summary,
		MinecraftVersion:  mcVersion,
		Loader:            loader,
		LoaderVersion:     loaderVersion,
		TotalFiles:        len(idx.Files),
		ServerFiles:       serverFiles,
		ClientOnlyFiles:   clientOnly,
		UserRequiredFiles: userRequiredFiles,
	}
}

func inspectCurseForgeManifest(man *curseForgeManifest) *ModpackManifest {
	loader := "forge"
	loaderVersion := ""

	for _, ml := range man.Minecraft.ModLoaders {
		id := strings.ToLower(ml.ID)
		if strings.HasPrefix(id, "fabric-") {
			loader = "fabric"
			loaderVersion = strings.TrimPrefix(id, "fabric-")
			break
		} else if strings.HasPrefix(id, "neoforge-") {
			loader = "neoforge"
			loaderVersion = strings.TrimPrefix(id, "neoforge-")
			break
		} else if strings.HasPrefix(id, "forge-") {
			loader = "forge"
			loaderVersion = strings.TrimPrefix(id, "forge-")
			break
		} else if strings.HasPrefix(id, "quilt-") {
			loader = "fabric"
			loaderVersion = strings.TrimPrefix(id, "quilt-")
			break
		}
	}

	return &ModpackManifest{
		Format:           ModpackFormatCurseForge,
		Name:             man.Name,
		Version:          man.Version,
		Author:           man.Author,
		MinecraftVersion: man.Minecraft.Version,
		Loader:           loader,
		LoaderVersion:    loaderVersion,
		TotalFiles:       len(man.Files),
		ServerFiles:      len(man.Files),
		ClientOnlyFiles:  0,
	}
}

// GetInstalledModpack returns the currently installed modpack for a server if any.
func (s *Store) GetInstalledModpack(ctx context.Context, serverID string) (*InstalledModpack, error) {
	root, err := s.openDataRoot(serverID)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := storage.OpenRegular(root, ".mcm_modpack.json")
	if os.IsNotExist(err) {
		return nil, ErrModpackNotFound
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrModpackNotFound
		}
		return nil, err
	}
	var res InstalledModpack
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// UninstallModpack deletes all files recorded for the installed modpack and removes metadata.
func (s *Store) UninstallModpack(ctx context.Context, serverID string) error {
	mu := s.lifecycleLock(serverID)
	mu.Lock()
	defer mu.Unlock()
	unlock := s.lockWorld(serverID, true)
	defer unlock()
	pack, err := s.GetInstalledModpack(ctx, serverID)
	if err != nil {
		return err
	}
	if pack.CreatedWithModpack {
		return ErrModpackLocked
	}
	if _, err = s.stopLocked(ctx, serverID); err != nil {
		return err
	}
	root, err := s.openDataRoot(serverID)
	if err != nil {
		return err
	}
	defer root.Close()
	stage := ".mcm-pack-stage-" + uuid.NewString()
	if err = root.Mkdir(stage, 0700); err != nil {
		return err
	}
	defer root.RemoveAll(stage)
	obsolete := append(append([]string{}, pack.InstalledFiles...), ".mcm_modpack.json")
	_, finish, err := storage.PublishTree(root, stage, obsolete)
	if err != nil {
		return err
	}
	if err = finish(); err != nil {
		return err
	}
	s.InvalidateModUpdatesCache(serverID)
	return nil
}

// InspectModpackFile opens a local archive file and returns its manifest.
func (s *Store) InspectModpackFile(filePath string) (*ModpackManifest, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	return InspectModpackArchive(f, fi.Size(), fi.Name())
}

// InstallModpack installs a modpack from an archive file onto the specified server.
func (s *Store) InstallModpack(ctx context.Context, serverID string, archivePath string, opts InstallModpackOpts) (installed *InstalledModpack, err error) {
	mu := s.lifecycleLock(serverID)
	mu.Lock()
	defer mu.Unlock()
	unlock := s.lockWorld(serverID, true)
	defer unlock()
	if _, err = s.stopLocked(ctx, serverID); err != nil {
		return nil, err
	}

	_ = s.setState(ctx, serverID, StateInstalling)

	stageOffset := 0
	if opts.Source != "upload" && opts.URL != "" {
		stageOffset = 1
	}
	stageTotal := stageOffset + 3

	defer func() {
		if err != nil {
			_ = s.setState(context.Background(), serverID, StateError)
			s.SetTaskProgress(serverID, TaskProgress{
				Operation:  "modpack_install",
				Stage:      "failed",
				StageTitle: "Installation Failed",
				StageIndex: stageOffset + 1,
				StageTotal: stageTotal,
				Error:      err.Error(),
				Message:    err.Error(),
			})
		}
	}()

	s.SetTaskProgress(serverID, TaskProgress{
		Operation:  "modpack_install",
		Stage:      "preparing",
		StageTitle: "Preparing Modpack",
		StageIndex: stageOffset + 1,
		StageTotal: stageTotal,
		Percent:    (stageOffset * 100) / stageTotal,
		Message:    "Inspecting modpack archive...",
	})

	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}

	manifest, err := InspectModpackArchive(f, fi.Size(), fi.Name())
	if err != nil {
		return nil, err
	}

	if manifest.Format == ModpackFormatGeneric {
		stageTotal = stageOffset + 2
	}

	stage1Index := stageOffset + 1
	stage1Base := (stage1Index - 1) * 100 / stageTotal
	stageWidth := 100 / stageTotal

	s.SetTaskProgress(serverID, TaskProgress{
		Operation:  "modpack_install",
		Stage:      "preparing",
		StageTitle: "Preparing Server",
		StageIndex: stage1Index,
		StageTotal: stageTotal,
		Percent:    stage1Base + stageWidth/2,
		Message:    fmt.Sprintf("Configuring server runtime (%s %s)", manifest.Loader, manifest.MinecraftVersion),
	})

	existing, existingErr := s.GetInstalledModpack(ctx, serverID)
	if existingErr != nil && !errors.Is(existingErr, ErrModpackNotFound) {
		return nil, existingErr
	}
	createdWithModpack := opts.CreatedWithModpack
	if existing != nil {
		if existing.CreatedWithModpack {
			createdWithModpack = true
		}
		// Verify this is an update of the same modpack, not a switch to a different modpack
		if existing.ProjectID != "" && opts.ProjectID != "" && existing.ProjectID != opts.ProjectID {
			return nil, fmt.Errorf("%w: server is locked to modpack %q (project %s)", ErrModpackLocked, existing.Name, existing.ProjectID)
		}
		if existing.ProjectSlug != "" && opts.ProjectSlug != "" && !strings.EqualFold(existing.ProjectSlug, opts.ProjectSlug) {
			return nil, fmt.Errorf("%w: server is locked to modpack %q (slug %s)", ErrModpackLocked, existing.Name, existing.ProjectSlug)
		}
		// If project identifiers are not set (e.g. file upload or generic url), check names
		if (existing.ProjectID == "" && opts.ProjectID == "") && (existing.ProjectSlug == "" && opts.ProjectSlug == "") {
			if manifest.Name != "" && existing.Name != "" {
				name1 := strings.ToLower(strings.TrimSpace(manifest.Name))
				name2 := strings.ToLower(strings.TrimSpace(existing.Name))
				if name1 != name2 && !strings.Contains(name1, name2) && !strings.Contains(name2, name1) {
					return nil, fmt.Errorf("%w: server is locked to modpack %q", ErrModpackLocked, existing.Name)
				}
			}
		}
	}

	liveRoot, err := s.openDataRoot(serverID)
	if err != nil {
		return nil, err
	}
	defer liveRoot.Close()
	stageName := ".mcm-pack-stage-" + uuid.NewString()
	if err = liveRoot.Mkdir(stageName, 0700); err != nil {
		return nil, err
	}
	defer liveRoot.RemoveAll(stageName)
	dataDir := filepath.Join(liveRoot.Name(), stageName)
	modsDir := filepath.Join(dataDir, "mods")
	if err = os.MkdirAll(modsDir, 0755); err != nil {
		return nil, err
	}
	// Keep catalog identity for user-owned artifacts while reconciling tracked pack files.
	if old, readErr := readModsMetaChecked(filepath.Join(liveRoot.Name(), "mods")); readErr == nil {
		if err = saveModsMeta(modsDir, old); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(readErr) {
		return nil, readErr
	}
	proposed := srv
	obsolete := []string{}
	if opts.AutoConfigureServer && manifest.Loader != "" && manifest.MinecraftVersion != "" {
		if s.jars == nil {
			return nil, fmt.Errorf("server software resolver unavailable")
		}
		jt, parseErr := jars.ParseJarType(manifest.Loader)
		if parseErr != nil {
			return nil, parseErr
		}
		resolved, resolveErr := s.jars.Validate(ctx, jt, manifest.MinecraftVersion, manifest.LoaderVersion)
		if resolveErr != nil {
			return nil, fmt.Errorf("validate modpack runtime: %w", resolveErr)
		}
		proposed.ServerType = string(jt)
		proposed.Version = resolved.Version
		proposed.Build = resolved.Build
		proposed.JavaVersion = jars.RecommendJavaVersion(resolved.Version)
		if err = s.jars.DownloadServerJar(ctx, jt, proposed.Version, proposed.Build, dataDir); err != nil {
			return nil, fmt.Errorf("download modpack runtime: %w", err)
		}
		obsolete, err = runtimePaths(liveRoot)
		if err != nil {
			return nil, err
		}
	}
	budget := storage.NewBudget(storage.MaxArchiveBytes)

	installedFiles := make([]string, 0)
	var fileMu sync.Mutex

	recordFile := func(rel string) {
		fileMu.Lock()
		installedFiles = append(installedFiles, filepath.ToSlash(rel))
		fileMu.Unlock()
	}

	// Seek f back to start for zip.NewReader
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(f, fi.Size())
	if err != nil {
		return nil, err
	}

	// Dedicated server overrides take precedence regardless of ZIP member order.
	sort.SliceStable(zr.File, func(i, j int) bool {
		return !strings.HasPrefix(zr.File[i].Name, "server-overrides/") && strings.HasPrefix(zr.File[j].Name, "server-overrides/")
	})

	extractStageIndex := stageOffset + 2
	extractBasePct := (extractStageIndex - 1) * 100 / stageTotal
	dlStageIndex := stageOffset + 3
	dlBasePct := (dlStageIndex - 1) * 100 / stageTotal

	var userRequiredMu sync.Mutex
	var userRequiredFiles []string

	switch manifest.Format {
	case ModpackFormatModrinth:
		// Extract index again
		var idx modrinthIndex
		for _, zf := range zr.File {
			if zf.Name == "modrinth.index.json" {
				rc, err := zf.Open()
				if err != nil {
					return nil, err
				}
				decodeErr := json.NewDecoder(io.LimitReader(rc, 8<<20)).Decode(&idx)
				if decodeErr != nil {
					rc.Close()
					return nil, decodeErr
				}
				_ = rc.Close()
				break
			}
		}

		// Count overrides for progress tracking
		totalOverrides := 0
		for _, zf := range zr.File {
			cleanName := filepath.ToSlash(zf.Name)
			if (strings.HasPrefix(cleanName, "overrides/") || strings.HasPrefix(cleanName, "server-overrides/")) && !zf.FileInfo().IsDir() {
				totalOverrides++
			}
		}

		// Extract overrides and server-overrides
		extractedOverrides := 0
		var lastExtractUpdate time.Time
		for _, zf := range zr.File {
			cleanName := filepath.ToSlash(zf.Name)
			var relPath string
			if strings.HasPrefix(cleanName, "overrides/") {
				relPath = strings.TrimPrefix(cleanName, "overrides/")
			} else if strings.HasPrefix(cleanName, "server-overrides/") {
				relPath = strings.TrimPrefix(cleanName, "server-overrides/")
			} else {
				continue
			}

			if relPath == "" || strings.HasSuffix(relPath, "/") {
				continue
			}

			targetPath := filepath.Join(dataDir, filepath.FromSlash(relPath))
			// Zip-slip containment
			if !strings.HasPrefix(targetPath, dataDir+string(os.PathSeparator)) {
				return nil, ErrInvalidPath
			}

			if zf.FileInfo().IsDir() {
				_ = os.MkdirAll(targetPath, 0o755)
				continue
			}

			// If it's a jar in mods/, check if it is client-only (Tier 1)
			if strings.HasPrefix(filepath.ToSlash(relPath), "mods/") && strings.HasSuffix(strings.ToLower(relPath), ".jar") {
				if isClient, _ := IsClientOnlyFilename(relPath); isClient {
					targetPath += ".disabled"
					relPath += ".disabled"
				}
			}

			if err := extractZipFileBounded(ctx, zf, targetPath, budget); err != nil {
				return nil, fmt.Errorf("extract %s: %w", zf.Name, err)
			} else {
				recordFile(relPath)
				// Deep inspection (Tier 2) on extracted mod jar
				if strings.HasPrefix(filepath.ToSlash(relPath), "mods/") && !strings.HasSuffix(targetPath, ".disabled") {
					if manifest, err := inspectModJar(targetPath); err == nil && manifest.IsClientOnly {
						disabledTarget := targetPath + ".disabled"
						if err := os.Rename(targetPath, disabledTarget); err == nil {
							recordFile(relPath + ".disabled")
						}
					}
				}
			}
			extractedOverrides++

			now := time.Now()
			if extractedOverrides == totalOverrides || now.Sub(lastExtractUpdate) >= 100*time.Millisecond {
				lastExtractUpdate = now
				pct := extractBasePct
				if totalOverrides > 0 {
					pct += int(float64(extractedOverrides) / float64(totalOverrides) * float64(stageWidth))
				}
				s.SetTaskProgress(serverID, TaskProgress{
					Operation:  "modpack_install",
					Stage:      "extracting",
					StageTitle: fmt.Sprintf("Extracting Files (%d/%d)", extractedOverrides, totalOverrides),
					StageIndex: extractStageIndex,
					StageTotal: stageTotal,
					Percent:    pct,
					Message:    filepath.Base(relPath),
					Current:    int64(extractedOverrides),
					Total:      int64(totalOverrides),
				})
			}
		}

		// Filter files for server compatibility
		eligibleFiles := make([]modrinthIndexFile, 0, len(idx.Files))
		for _, mf := range idx.Files {
			if strings.ToLower(mf.Env.Server) == "unsupported" {
				continue
			}
			if isClient, _ := IsClientOnlyFilename(mf.Path); isClient {
				continue
			}
			eligibleFiles = append(eligibleFiles, mf)
		}

		// Concurrent file downloader (6 workers)
		concurrency := 6
		if len(eligibleFiles) < concurrency {
			concurrency = len(eligibleFiles)
		}
		if concurrency == 0 {
			concurrency = 1
		}

		totalMods := len(eligibleFiles)
		var downloadedCount atomic.Int64
		var lastDlUpdateMu sync.Mutex
		var lastDlUpdate time.Time

		emitModProgress := func(curr int64, currentName string) {
			lastDlUpdateMu.Lock()
			now := time.Now()
			if curr < int64(totalMods) && now.Sub(lastDlUpdate) < 100*time.Millisecond {
				lastDlUpdateMu.Unlock()
				return
			}
			lastDlUpdate = now
			lastDlUpdateMu.Unlock()

			pct := dlBasePct
			if totalMods > 0 {
				pct += int(float64(curr) / float64(totalMods) * float64(stageWidth))
			}
			if pct > 99 {
				pct = 99
			}
			s.SetTaskProgress(serverID, TaskProgress{
				Operation:  "modpack_install",
				Stage:      "downloading_mods",
				StageTitle: fmt.Sprintf("Downloading Mods (%d/%d)", curr, totalMods),
				StageIndex: dlStageIndex,
				StageTotal: stageTotal,
				Percent:    pct,
				Message:    currentName,
				Current:    curr,
				Total:      int64(totalMods),
			})
		}

		emitModProgress(0, "Starting mod downloads...")

		fileCh := make(chan modrinthIndexFile, len(eligibleFiles))
		for _, ef := range eligibleFiles {
			fileCh <- ef
		}
		close(fileCh)

		for _, mf := range eligibleFiles {
			if len(mf.Downloads) == 0 {
				if strings.ToLower(mf.Env.Server) != "optional" {
					return nil, fmt.Errorf("required modpack file %s has no download", mf.Path)
				}
				userRequiredMu.Lock()
				userRequiredFiles = append(userRequiredFiles, mf.Path)
				userRequiredMu.Unlock()
			}
		}

		client := &http.Client{Timeout: 90 * time.Second}
		var wg sync.WaitGroup
		var downloadErr error
		var errMu sync.Mutex

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for mf := range fileCh {
					if len(mf.Downloads) == 0 {
						continue
					}
					targetRel := mf.Path
					targetAbs := filepath.Join(dataDir, filepath.FromSlash(targetRel))
					if clean, pathErr := storage.Clean(targetRel); pathErr != nil || clean == "." {
						errMu.Lock()
						downloadErr = ErrInvalidPath
						errMu.Unlock()
						return
					}
					if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
						errMu.Lock()
						downloadErr = err
						errMu.Unlock()
						return
					}

					dlURL := mf.Downloads[0]
					if err := downloadToFile(ctx, client, dlURL, targetAbs, budget); err != nil {
						if strings.ToLower(mf.Env.Server) != "optional" {
							errMu.Lock()
							downloadErr = err
							errMu.Unlock()
							return
						}
						// Non-fatal if one file fails, but record as requiring manual provision
						userRequiredMu.Lock()
						userRequiredFiles = append(userRequiredFiles, mf.Path)
						userRequiredMu.Unlock()
						continue
					}
					if strings.HasSuffix(strings.ToLower(targetAbs), ".jar") {
						if verifyErr := validateArtifact(ctx, targetAbs, mf.Hashes); verifyErr != nil {
							errMu.Lock()
							downloadErr = verifyErr
							errMu.Unlock()
							return
						}
					} else if verifyErr := verifyPackHashes(ctx, targetAbs, mf.Hashes); verifyErr != nil {
						errMu.Lock()
						downloadErr = verifyErr
						errMu.Unlock()
						return
					}
					recordFile(targetRel)

					// Write mod meta for catalog discovery
					if strings.HasPrefix(filepath.ToSlash(targetRel), "mods/") {
						fname := filepath.Base(targetRel)
						manifest, _ := inspectModJar(targetAbs)
						sha1Hash := mf.Hashes["sha1"]
						if sha1Hash == "" {
							sha1Hash = manifest.SHA1
						}
						if metaErr := writeModMeta(modsDir, fname, ModDownloadMeta{
							Provider:         "modrinth",
							InstalledVersion: manifest.Version,
							DownloadURL:      dlURL,
						}, manifest.ModID); metaErr != nil {
							errMu.Lock()
							downloadErr = metaErr
							errMu.Unlock()
							return
						}
					}

					curr := downloadedCount.Add(1)
					emitModProgress(curr, filepath.Base(targetRel))
				}
			}()
		}
		wg.Wait()
		if downloadErr != nil {
			return nil, fmt.Errorf("download modpack files: %w", downloadErr)
		}

	case ModpackFormatCurseForge:
		// Extract CurseForge manifest
		var man curseForgeManifest
		for _, zf := range zr.File {
			if zf.Name == "manifest.json" {
				rc, err := zf.Open()
				if err != nil {
					return nil, err
				}
				decodeErr := json.NewDecoder(io.LimitReader(rc, 8<<20)).Decode(&man)
				if decodeErr != nil {
					rc.Close()
					return nil, decodeErr
				}
				_ = rc.Close()
				break
			}
		}

		// Extract overrides directory
		overridesDir := man.Overrides
		if overridesDir == "" {
			overridesDir = "overrides"
		}
		overridesPrefix := overridesDir + "/"

		totalOverrides := 0
		for _, zf := range zr.File {
			cleanName := filepath.ToSlash(zf.Name)
			if strings.HasPrefix(cleanName, overridesPrefix) && !zf.FileInfo().IsDir() {
				totalOverrides++
			}
		}

		extractedOverrides := 0
		var lastExtractUpdate time.Time
		for _, zf := range zr.File {
			cleanName := filepath.ToSlash(zf.Name)
			if !strings.HasPrefix(cleanName, overridesPrefix) {
				continue
			}
			relPath := strings.TrimPrefix(cleanName, overridesPrefix)
			if relPath == "" || strings.HasSuffix(relPath, "/") {
				continue
			}

			targetPath := filepath.Join(dataDir, filepath.FromSlash(relPath))
			if !strings.HasPrefix(targetPath, dataDir+string(os.PathSeparator)) {
				return nil, ErrInvalidPath
			}

			if zf.FileInfo().IsDir() {
				_ = os.MkdirAll(targetPath, 0o755)
				continue
			}

			if err := extractZipFileBounded(ctx, zf, targetPath, budget); err != nil {
				return nil, fmt.Errorf("extract %s: %w", zf.Name, err)
			} else {
				recordFile(relPath)
			}
			extractedOverrides++

			now := time.Now()
			if extractedOverrides == totalOverrides || now.Sub(lastExtractUpdate) >= 100*time.Millisecond {
				lastExtractUpdate = now
				pct := extractBasePct
				if totalOverrides > 0 {
					pct += int(float64(extractedOverrides) / float64(totalOverrides) * float64(stageWidth))
				}
				s.SetTaskProgress(serverID, TaskProgress{
					Operation:  "modpack_install",
					Stage:      "extracting",
					StageTitle: fmt.Sprintf("Extracting Files (%d/%d)", extractedOverrides, totalOverrides),
					StageIndex: extractStageIndex,
					StageTotal: stageTotal,
					Percent:    pct,
					Message:    filepath.Base(relPath),
					Current:    int64(extractedOverrides),
					Total:      int64(totalOverrides),
				})
			}
		}

		// Download CurseForge mods
		concurrency := 4
		if len(man.Files) < concurrency {
			concurrency = len(man.Files)
		}
		if concurrency == 0 {
			concurrency = 1
		}

		totalCFMods := len(man.Files)
		var downloadedCF atomic.Int64
		var lastCFUpdateMu sync.Mutex
		var lastCFUpdate time.Time

		emitCFProgress := func(curr int64, currentName string) {
			lastCFUpdateMu.Lock()
			now := time.Now()
			if curr < int64(totalCFMods) && now.Sub(lastCFUpdate) < 100*time.Millisecond {
				lastCFUpdateMu.Unlock()
				return
			}
			lastCFUpdate = now
			lastCFUpdateMu.Unlock()

			pct := dlBasePct
			if totalCFMods > 0 {
				pct += int(float64(curr) / float64(totalCFMods) * float64(stageWidth))
			}
			if pct > 99 {
				pct = 99
			}
			s.SetTaskProgress(serverID, TaskProgress{
				Operation:  "modpack_install",
				Stage:      "downloading_mods",
				StageTitle: fmt.Sprintf("Downloading Mods (%d/%d)", curr, totalCFMods),
				StageIndex: dlStageIndex,
				StageTotal: stageTotal,
				Percent:    pct,
				Message:    currentName,
				Current:    curr,
				Total:      int64(totalCFMods),
			})
		}

		emitCFProgress(0, "Starting mod downloads...")

		cfCh := make(chan struct {
			ProjectID int
			FileID    int
			Required  bool
		}, len(man.Files))
		for _, cfFile := range man.Files {
			cfCh <- struct {
				ProjectID int
				FileID    int
				Required  bool
			}{ProjectID: cfFile.ProjectID, FileID: cfFile.FileID, Required: cfFile.Required}
		}
		close(cfCh)

		client := &http.Client{Timeout: 90 * time.Second}
		var wg sync.WaitGroup
		var cfErr error
		var cfErrMu sync.Mutex
		failCF := func(e error) {
			cfErrMu.Lock()
			if cfErr == nil {
				cfErr = e
			}
			cfErrMu.Unlock()
		}

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range cfCh {
					dlURL, fileName, hashes, err := resolveCurseForgeDownload(ctx, client, item.ProjectID, item.FileID, opts.CurseForgeAPIKey)
					if err != nil || dlURL == "" {
						if item.Required {
							failCF(fmt.Errorf("required CurseForge file %d unavailable: %v", item.FileID, err))
						}
						userRequiredMu.Lock()
						userRequiredFiles = append(userRequiredFiles, fmt.Sprintf("CurseForge File #%d (Project %d)", item.FileID, item.ProjectID))
						userRequiredMu.Unlock()
						curr := downloadedCF.Add(1)
						emitCFProgress(curr, fmt.Sprintf("File #%d (failed to resolve)", item.FileID))
						continue
					}
					if fileName == "" {
						fileName = fmt.Sprintf("cf_%d_%d.jar", item.ProjectID, item.FileID)
					}
					if !validModFileName(fileName) {
						failCF(ErrInvalidModName)
						return
					}
					// Tier 1 check on filename before downloading
					if isClient, _ := IsClientOnlyFilename(fileName); isClient {
						curr := downloadedCF.Add(1)
						emitCFProgress(curr, fileName+" (skipped client-only)")
						continue
					}

					targetAbs := filepath.Join(modsDir, fileName)
					if err := downloadToFile(ctx, client, dlURL, targetAbs, budget); err != nil {
						if item.Required {
							failCF(err)
						}
						userRequiredMu.Lock()
						userRequiredFiles = append(userRequiredFiles, fileName)
						userRequiredMu.Unlock()
						curr := downloadedCF.Add(1)
						emitCFProgress(curr, fileName+" (download failed)")
						continue
					}
					if err := validateArtifact(ctx, targetAbs, hashes); err != nil {
						failCF(err)
						return
					}
					rel := filepath.Join("mods", fileName)

					manifest, _ := inspectModJar(targetAbs)
					// Tier 2 check on downloaded jar
					if manifest.IsClientOnly {
						disabledTarget := targetAbs + ".disabled"
						if err := os.Rename(targetAbs, disabledTarget); err == nil {
							recordFile(rel + ".disabled")
							if metaErr := writeModMeta(modsDir, fileName+".disabled", ModDownloadMeta{
								ProjectID:        fmt.Sprint(item.ProjectID),
								Provider:         "curseforge",
								InstalledVersion: manifest.Version,
								DownloadURL:      dlURL,
							}, manifest.ModID); metaErr != nil {
								failCF(metaErr)
								return
							}
							curr := downloadedCF.Add(1)
							emitCFProgress(curr, fileName+" (client-only disabled)")
							continue
						}
					}

					recordFile(rel)
					if metaErr := writeModMeta(modsDir, fileName, ModDownloadMeta{
						ProjectID:        fmt.Sprint(item.ProjectID),
						Provider:         "curseforge",
						InstalledVersion: manifest.Version,
						DownloadURL:      dlURL,
					}, manifest.ModID); metaErr != nil {
						failCF(metaErr)
						return
					}

					curr := downloadedCF.Add(1)
					emitCFProgress(curr, fileName)
				}
			}()
		}
		wg.Wait()
		if cfErr != nil {
			return nil, cfErr
		}

	case ModpackFormatGeneric:
		// Extract zip directly. If all jars are at root, extract into mods/.
		// Otherwise extract with folder hierarchy intact.
		allJarsRoot := true
		totalFiles := 0
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			totalFiles++
			clean := filepath.ToSlash(zf.Name)
			if strings.Contains(clean, "/") && strings.HasSuffix(strings.ToLower(clean), ".jar") {
				allJarsRoot = false
			}
		}

		extractedFiles := 0
		var lastExtractUpdate time.Time
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			cleanName := filepath.ToSlash(zf.Name)

			var targetPath string
			var relPath string

			if allJarsRoot && strings.HasSuffix(strings.ToLower(cleanName), ".jar") {
				relPath = filepath.Join("mods", filepath.Base(cleanName))
				targetPath = filepath.Join(modsDir, filepath.Base(cleanName))
			} else {
				// Strip leading single root directory if present (e.g. MyPack/mods/... -> mods/...)
				relPath = stripCommonModpackPrefix(cleanName)
				targetPath = filepath.Join(dataDir, filepath.FromSlash(relPath))
			}

			if !strings.HasPrefix(targetPath, dataDir+string(os.PathSeparator)) {
				return nil, ErrInvalidPath
			}

			if err := extractZipFileBounded(ctx, zf, targetPath, budget); err != nil {
				return nil, fmt.Errorf("extract %s: %w", zf.Name, err)
			} else {
				recordFile(relPath)
				if strings.HasPrefix(filepath.ToSlash(relPath), "mods/") && strings.HasSuffix(strings.ToLower(relPath), ".jar") {
					fname := filepath.Base(relPath)
					manifest, _ := inspectModJar(targetPath)
					if manifest.IsClientOnly {
						disabledPath := targetPath + ".disabled"
						if err := os.Rename(targetPath, disabledPath); err == nil {
							fname += ".disabled"
							recordFile(relPath + ".disabled")
						}
					}
					if metaErr := writeModMeta(modsDir, fname, ModDownloadMeta{
						InstalledVersion: manifest.Version,
					}, manifest.ModID); metaErr != nil {
						return nil, metaErr
					}
				}
			}
			extractedFiles++

			now := time.Now()
			if extractedFiles == totalFiles || now.Sub(lastExtractUpdate) >= 100*time.Millisecond {
				lastExtractUpdate = now
				pct := extractBasePct
				if totalFiles > 0 {
					pct += int(float64(extractedFiles) / float64(totalFiles) * float64(stageWidth))
				}
				s.SetTaskProgress(serverID, TaskProgress{
					Operation:  "modpack_install",
					Stage:      "extracting",
					StageTitle: fmt.Sprintf("Extracting Files (%d/%d)", extractedFiles, totalFiles),
					StageIndex: extractStageIndex,
					StageTotal: stageTotal,
					Percent:    pct,
					Message:    filepath.Base(relPath),
					Current:    int64(extractedFiles),
					Total:      int64(totalFiles),
				})
			}
		}
	}

	installed = &InstalledModpack{
		Name:               manifest.Name,
		Version:            manifest.Version,
		Summary:            manifest.Summary,
		Author:             manifest.Author,
		Format:             manifest.Format,
		MinecraftVersion:   manifest.MinecraftVersion,
		Loader:             manifest.Loader,
		LoaderVersion:      manifest.LoaderVersion,
		InstalledAt:        time.Now().UTC().Format(time.RFC3339),
		Source:             opts.Source,
		ProjectID:          opts.ProjectID,
		ProjectSlug:        opts.ProjectSlug,
		InstalledFiles:     installedFiles,
		UserRequiredFiles:  userRequiredFiles,
		IconURL:            manifest.IconURL,
		CreatedWithModpack: createdWithModpack,
	}
	if installed.ProjectID == "" && existing != nil {
		installed.ProjectID = existing.ProjectID
	}
	if installed.ProjectSlug == "" && existing != nil {
		installed.ProjectSlug = existing.ProjectSlug
	}
	if installed.IconURL == "" && existing != nil {
		installed.IconURL = existing.IconURL
	}

	s.SetTaskProgress(serverID, TaskProgress{
		Operation:  "modpack_install",
		Stage:      "finalizing",
		StageTitle: "Finalizing Installation",
		StageIndex: stageTotal,
		StageTotal: stageTotal,
		Percent:    99,
		Message:    "Saving modpack metadata...",
	})

	// Verify every staged jar before any live file or runtime configuration changes.
	stageRoot, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}
	defer stageRoot.Close()
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jar") || strings.HasSuffix(entry.Name(), ".jar.disabled") {
			file := filepath.Join(modsDir, entry.Name())
			if err = validateArtifact(ctx, file, nil); err != nil {
				return nil, err
			}
			info, inspectErr := inspectModJar(file)
			if inspectErr != nil {
				return nil, inspectErr
			}
			if info.IsClientOnly && !strings.HasSuffix(entry.Name(), ".disabled") {
				if err = os.Rename(file, file+".disabled"); err != nil {
					return nil, err
				}
			}
		}
	}
	// Derive the manifest from files that actually exist after client-only renames.
	installed.InstalledFiles = nil
	for _, rel := range installedFiles {
		if _, statErr := stageRoot.Stat(rel); statErr == nil {
			installed.InstalledFiles = append(installed.InstalledFiles, rel)
		} else if _, statErr = stageRoot.Stat(rel + ".disabled"); statErr == nil {
			installed.InstalledFiles = append(installed.InstalledFiles, rel+".disabled")
		}
	}
	if existing != nil {
		current := map[string]bool{}
		for _, rel := range installed.InstalledFiles {
			current[rel] = true
		}
		for _, rel := range existing.InstalledFiles {
			if !current[rel] {
				obsolete = append(obsolete, rel)
			}
		}
	}
	data, err := json.MarshalIndent(installed, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err = storage.Write(ctx, stageRoot, ".mcm_modpack.json", strings.NewReader(string(data)), 8<<20, 0644); err != nil {
		return nil, err
	}
	undo, finish, err := storage.PublishTree(liveRoot, stageName, obsolete)
	if err != nil {
		return nil, err
	}
	published := false
	defer func() {
		if !published {
			if rollbackErr := undo(); rollbackErr != nil {
				err = errors.Join(err, rollbackErr)
			}
		} else {
			if cleanupErr := finish(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
			}
		}
	}()
	if proposed.ServerType != srv.ServerType || proposed.Version != srv.Version || proposed.Build != srv.Build {
		if srv.ContainerID != "" {
			if err = s.removeContainer(ctx, srv.ContainerID); err != nil {
				return nil, err
			}
			if err = s.clearContainerID(ctx, serverID); err != nil {
				return nil, err
			}
		}
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE servers SET server_type=?,version=?,build=?,java_version=?,updated_at=? WHERE id=?`, proposed.ServerType, proposed.Version, proposed.Build, proposed.JavaVersion, time.Now().UTC().Format(time.RFC3339), serverID); err != nil {
		return nil, err
	}
	published = true

	s.InvalidateModUpdatesCache(serverID)

	msg := fmt.Sprintf("Installed %s %s successfully", installed.Name, installed.Version)
	stageTitle := "Installation Complete"
	if len(userRequiredFiles) > 0 {
		stageTitle = "Installation Complete (Action Required)"
		msg = fmt.Sprintf("Installed %s. %d mod(s) could not be downloaded and must be provided manually.", installed.Name, len(userRequiredFiles))
	}

	s.SetTaskProgress(serverID, TaskProgress{
		Operation:  "modpack_install",
		Stage:      "completed",
		StageTitle: stageTitle,
		StageIndex: stageTotal,
		StageTotal: stageTotal,
		Percent:    100,
		Message:    msg,
	})

	_ = s.setState(ctx, serverID, StateStopped)

	return installed, nil
}

// stripCommonModpackPrefix strips an enclosing single top-level directory name
// if the zip was packaged as e.g. PackName/mods/foo.jar instead of mods/foo.jar.
func stripCommonModpackPrefix(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) > 1 {
		first := strings.ToLower(parts[0])
		if first == "mods" || first == "config" || first == "defaultconfigs" || first == "kubejs" || first == "scripts" || first == "resourcepacks" {
			return path
		}
		// If second component is a known minecraft dir, strip first
		second := strings.ToLower(parts[1])
		if second == "mods" || second == "config" || second == "defaultconfigs" || second == "kubejs" || second == "scripts" {
			return strings.Join(parts[1:], "/")
		}
	}
	return path
}

func extractZipFileBounded(ctx context.Context, zf *zip.File, destPath string, budget *storage.Budget) error {
	if !zf.Mode().IsRegular() {
		return ErrInvalidArchive
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	_, err = storage.Copy(ctx, budget.Writer(out), rc, storage.MaxArchiveBytes)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return err
}

func downloadToFile(ctx context.Context, client *http.Client, rawURL, destPath string, budgets ...*storage.Budget) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrInvalidPath
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mcm-panel/1.0 (https://github.com/mcm-panel/mcm)")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	tmpFile := destPath + fmt.Sprintf(".tmp_%d", time.Now().UnixNano())
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	var writer io.Writer = out
	if len(budgets) > 0 {
		writer = budgets[0].Writer(out)
	}
	_, werr := storage.Copy(ctx, writer, resp.Body, maxDownloadBytes)
	cerr := out.Close()
	if werr != nil {
		_ = os.Remove(tmpFile)
		return werr
	}
	if cerr != nil {
		_ = os.Remove(tmpFile)
		return cerr
	}

	return os.Rename(tmpFile, destPath)
}

// resolveCurseForgeDownload resolves download URL and filename for a CurseForge file.
func resolveCurseForgeDownload(ctx context.Context, client *http.Client, projectID, fileID int, apiKey string) (string, string, map[string]string, error) {
	if apiKey == "" {
		apiKey = os.Getenv("CURSEFORGE_API_KEY")
	}
	target := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d", projectID, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", nil, err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", nil, fmt.Errorf("CurseForge file metadata returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Data struct {
			FileName    string           `json:"fileName"`
			DownloadURL string           `json:"downloadUrl"`
			Hashes      []curseForgeHash `json:"hashes"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", "", nil, err
	}
	file := result.Data
	if !validModFileName(file.FileName) {
		return "", "", nil, ErrInvalidModName
	}
	hashes := curseForgeHashes(file.Hashes)
	if file.DownloadURL != "" {
		return file.DownloadURL, file.FileName, hashes, nil
	}
	download, err := resolveCurseForgeDownloadURLEndpoint(ctx, client, projectID, fileID, apiKey)
	if err != nil || download == "" {
		return "", "", nil, fmt.Errorf("CurseForge file %d requires manual download: %v", fileID, err)
	}
	return download, file.FileName, hashes, nil
}

func resolveCurseForgeDownloadURLEndpoint(ctx context.Context, client *http.Client, projectID, fileID int, apiKey string) (string, error) {
	url := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d/download-url", projectID, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var res struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Data, nil
}

// CalculateSHA1 returns hex-encoded sha1 checksum of a reader.
func CalculateSHA1(r io.Reader) (string, error) {
	h := sha1.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func validatePackZIP(zr *zip.Reader) error {
	if len(zr.File) > storage.MaxArchiveEntries {
		return ErrInvalidArchive
	}
	var total uint64
	for _, entry := range zr.File {
		name, err := storage.Clean(entry.Name)
		if err != nil || name == "." {
			return ErrInvalidArchive
		}
		if !entry.Mode().IsRegular() && !entry.FileInfo().IsDir() {
			return ErrInvalidArchive
		}
		if entry.UncompressedSize64 > uint64(storage.MaxArchiveBytes)-total {
			return ErrInvalidArchive
		}
		total += entry.UncompressedSize64
	}
	return nil
}
func verifyPackHashes(ctx context.Context, file string, hashes map[string]string) error {
	return verifyArtifactHashes(ctx, file, hashes)
}
