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
	"strings"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/jars"
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
	Format           ModpackFormat `json:"format"`
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	Summary          string        `json:"summary,omitempty"`
	Author           string        `json:"author,omitempty"`
	MinecraftVersion string        `json:"minecraft_version"`
	Loader           string        `json:"loader"`
	LoaderVersion    string        `json:"loader_version"`
	TotalFiles       int           `json:"total_files"`
	ServerFiles      int           `json:"server_files"`
	ClientOnlyFiles  int           `json:"client_only_files"`
	IconURL          string        `json:"icon_url,omitempty"`
}

// InstalledModpack records the details of a modpack currently installed on a server.
type InstalledModpack struct {
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	Summary          string        `json:"summary,omitempty"`
	Author           string        `json:"author,omitempty"`
	Format           ModpackFormat `json:"format"`
	MinecraftVersion string        `json:"minecraft_version"`
	Loader           string        `json:"loader"`
	LoaderVersion    string        `json:"loader_version"`
	InstalledAt      string        `json:"installed_at"`
	Source           string        `json:"source"` // "upload", "modrinth", "curseforge", "url"
	ProjectID        string        `json:"project_id,omitempty"`
	ProjectSlug      string        `json:"project_slug,omitempty"`
	InstalledFiles     []string      `json:"installed_files"`
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
	for _, f := range idx.Files {
		// If explicitly marked as unsupported on server, exclude it
		if strings.ToLower(f.Env.Server) == "unsupported" {
			clientOnly++
		} else {
			serverFiles++
		}
	}

	return &ModpackManifest{
		Format:           ModpackFormatModrinth,
		Name:             idx.Name,
		Version:          idx.VersionID,
		Summary:          idx.Summary,
		MinecraftVersion: mcVersion,
		Loader:           loader,
		LoaderVersion:    loaderVersion,
		TotalFiles:       len(idx.Files),
		ServerFiles:      serverFiles,
		ClientOnlyFiles:  clientOnly,
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
	dataDir := s.dataPath(serverID)
	data, err := os.ReadFile(modpackMetaFile(dataDir))
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
	modpack, err := s.GetInstalledModpack(ctx, serverID)
	if err != nil {
		return err
	}
	if modpack.CreatedWithModpack {
		return ErrModpackLocked
	}

	dataDir := s.dataPath(serverID)
	modsDir := filepath.Join(dataDir, "mods")

	for _, rel := range modpack.InstalledFiles {
		if rel == "" {
			continue
		}
		p := filepath.Join(dataDir, filepath.FromSlash(rel))
		if strings.HasPrefix(p, dataDir+string(os.PathSeparator)) {
			_ = os.Remove(p)
			if strings.HasPrefix(rel, "mods/") {
				fname := filepath.Base(rel)
				deleteModMeta(modsDir, fname)
			}
		}
	}

	_ = os.Remove(modpackMetaFile(dataDir))
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
func (s *Store) InstallModpack(ctx context.Context, serverID string, archivePath string, opts InstallModpackOpts) (*InstalledModpack, error) {
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

	existing, _ := s.GetInstalledModpack(ctx, serverID)
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

	dataDir := s.dataPath(serverID)
	modsDir := filepath.Join(dataDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create mods directory: %w", err)
	}

	// If AutoConfigureServer is enabled and manifest declares loader & version,
	// synchronize server configuration to ensure container starts compatible runtime.
	if opts.AutoConfigureServer && manifest.Loader != "" && manifest.MinecraftVersion != "" {
		targetType := manifest.Loader
		targetVersion := manifest.MinecraftVersion
		targetBuild := manifest.LoaderVersion

		if s.jars != nil {
			srvType := jars.JarType(targetType)
			upd := UpdateInput{
				ServerType: &srvType,
				Version:    &targetVersion,
				Build:      &targetBuild,
			}
			if _, err := s.Update(ctx, serverID, upd); err == nil {
				// Detach old container if it exists so next start provision uses new runtime
				if srv.ContainerID != "" && s.docker != nil {
					_ = s.docker.Remove(ctx, srv.ContainerID)
					_ = s.clearContainerID(ctx, serverID)
				}
			} else {
				// Direct update fallback if Validate fails due to upstream metadata differences
				now := time.Now().UTC().Format(time.RFC3339)
				_, _ = s.db.ExecContext(ctx, "UPDATE servers SET server_type=?, version=?, build=?, updated_at=? WHERE id=?",
					targetType, targetVersion, targetBuild, now, serverID)
				if srv.ContainerID != "" && s.docker != nil {
					_ = s.docker.Remove(ctx, srv.ContainerID)
					_ = s.clearContainerID(ctx, serverID)
				}
			}
		} else {
			// Direct DB update when jars resolver is not wired (e.g. lightweight test)
			now := time.Now().UTC().Format(time.RFC3339)
			_, _ = s.db.ExecContext(ctx, "UPDATE servers SET server_type=?, version=?, build=?, updated_at=? WHERE id=?",
				targetType, targetVersion, targetBuild, now, serverID)
		}
	}

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
				_ = json.NewDecoder(rc).Decode(&idx)
				_ = rc.Close()
				break
			}
		}

		// Extract overrides and server-overrides
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
				continue
			}

			if zf.FileInfo().IsDir() {
				_ = os.MkdirAll(targetPath, 0o755)
				continue
			}

			if err := extractZipFile(zf, targetPath); err == nil {
				recordFile(relPath)
			}
		}

		// Filter files for server compatibility
		eligibleFiles := make([]modrinthIndexFile, 0, len(idx.Files))
		for _, mf := range idx.Files {
			if strings.ToLower(mf.Env.Server) == "unsupported" {
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

		fileCh := make(chan modrinthIndexFile, len(eligibleFiles))
		for _, ef := range eligibleFiles {
			fileCh <- ef
		}
		close(fileCh)

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
					if !strings.HasPrefix(targetAbs, dataDir+string(os.PathSeparator)) {
						continue
					}
					if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
						errMu.Lock()
						downloadErr = err
						errMu.Unlock()
						return
					}

					dlURL := mf.Downloads[0]
					if err := downloadToFile(ctx, client, dlURL, targetAbs); err != nil {
						// Non-fatal if one optional file fails, but log
						continue
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
						writeModMeta(modsDir, fname, ModDownloadMeta{
							Provider:         "modrinth",
							InstalledVersion: manifest.Version,
							DownloadURL:      dlURL,
						}, manifest.ModID)
					}
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
				_ = json.NewDecoder(rc).Decode(&man)
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
				continue
			}

			if zf.FileInfo().IsDir() {
				_ = os.MkdirAll(targetPath, 0o755)
				continue
			}

			if err := extractZipFile(zf, targetPath); err == nil {
				recordFile(relPath)
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

		cfCh := make(chan struct {
			ProjectID int
			FileID    int
		}, len(man.Files))
		for _, cfFile := range man.Files {
			cfCh <- struct {
				ProjectID int
				FileID    int
			}{ProjectID: cfFile.ProjectID, FileID: cfFile.FileID}
		}
		close(cfCh)

		client := &http.Client{Timeout: 90 * time.Second}
		var wg sync.WaitGroup

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range cfCh {
					dlURL, fileName, err := resolveCurseForgeDownload(ctx, client, item.ProjectID, item.FileID, opts.CurseForgeAPIKey)
					if err != nil || dlURL == "" {
						continue
					}
					if fileName == "" {
						fileName = fmt.Sprintf("cf_%d_%d.jar", item.ProjectID, item.FileID)
					}
					targetAbs := filepath.Join(modsDir, fileName)
					if err := downloadToFile(ctx, client, dlURL, targetAbs); err != nil {
						continue
					}
					rel := filepath.Join("mods", fileName)
					recordFile(rel)

					manifest, _ := inspectModJar(targetAbs)
					writeModMeta(modsDir, fileName, ModDownloadMeta{
						ProjectID:        fmt.Sprint(item.ProjectID),
						Provider:         "curseforge",
						InstalledVersion: manifest.Version,
						DownloadURL:      dlURL,
					}, manifest.ModID)
				}
			}()
		}
		wg.Wait()

	case ModpackFormatGeneric:
		// Extract zip directly. If all jars are at root, extract into mods/.
		// Otherwise extract with folder hierarchy intact.
		allJarsRoot := true
		for _, zf := range zr.File {
			if zf.FileInfo().IsDir() {
				continue
			}
			clean := filepath.ToSlash(zf.Name)
			if strings.Contains(clean, "/") && strings.HasSuffix(strings.ToLower(clean), ".jar") {
				allJarsRoot = false
				break
			}
		}

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
				continue
			}

			if err := extractZipFile(zf, targetPath); err == nil {
				recordFile(relPath)
				if strings.HasPrefix(filepath.ToSlash(relPath), "mods/") && strings.HasSuffix(strings.ToLower(relPath), ".jar") {
					fname := filepath.Base(relPath)
					manifest, _ := inspectModJar(targetPath)
					writeModMeta(modsDir, fname, ModDownloadMeta{
						InstalledVersion: manifest.Version,
					}, manifest.ModID)
				}
			}
		}
	}

	installed := &InstalledModpack{
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

	// Persist installed modpack record
	if data, err := json.MarshalIndent(installed, "", "  "); err == nil {
		_ = os.WriteFile(modpackMetaFile(dataDir), data, 0o644)
	}

	s.InvalidateModUpdatesCache(serverID)
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

func extractZipFile(zf *zip.File, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

func downloadToFile(ctx context.Context, client *http.Client, rawURL, destPath string) error {
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
	_, werr := io.Copy(out, resp.Body)
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
func resolveCurseForgeDownload(ctx context.Context, client *http.Client, projectID, fileID int, apiKey string) (string, string, error) {
	if apiKey == "" {
		apiKey = os.Getenv("CURSEFORGE_API_KEY")
	}

	// 1. Query /v1/mods/{projectID}/files/{fileID}
	url := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d", projectID, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "mcm-panel/1.0 (https://github.com/mcm-panel/mcm)")

	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var res struct {
			Data struct {
				FileName    string `json:"fileName"`
				DownloadURL string `json:"downloadUrl"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			if res.Data.DownloadURL != "" {
				return res.Data.DownloadURL, res.Data.FileName, nil
			}
			// If downloadUrl is empty in payload, query download-url endpoint
			dlURL, err := resolveCurseForgeDownloadURLEndpoint(ctx, client, projectID, fileID, apiKey)
			if err == nil && dlURL != "" {
				return dlURL, res.Data.FileName, nil
			}
			// Fallback: construct edge URL if fileId > 0
			if res.Data.FileName != "" {
				edgeURL := fmt.Sprintf("https://edge.forgecdn.net/files/%d/%d/%s", fileID/1000, fileID%1000, res.Data.FileName)
				return edgeURL, res.Data.FileName, nil
			}
		}
	} else if resp != nil {
		resp.Body.Close()
	}

	// Direct edge fallback
	edgeURL := fmt.Sprintf("https://edge.forgecdn.net/files/%d/%d/mod_%d_%d.jar", fileID/1000, fileID%1000, projectID, fileID)
	return edgeURL, fmt.Sprintf("mod_%d_%d.jar", projectID, fileID), nil
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
