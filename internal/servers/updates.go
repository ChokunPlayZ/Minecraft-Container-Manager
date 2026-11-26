package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/curseforge"
	"github.com/mcm-panel/mcm/internal/proxy"
)

func cleanVersionString(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "release-")
	v = strings.TrimPrefix(v, "build-")
	return v
}

// AvailableModJar describes a single installable build of a mod or plugin.
type AvailableModJar struct {
	Hashes        map[string]string `json:"hashes,omitempty"`
	VersionID     string            `json:"version_id"`
	Provider      string            `json:"provider,omitempty"`
	ProjectID     string            `json:"project_id,omitempty"`
	ProjectSlug   string            `json:"project_slug,omitempty"`
	VersionName   string            `json:"version_name"`
	VersionNumber string            `json:"version_number"`
	Filename      string            `json:"filename"`
	DownloadURL   string            `json:"download_url"`
	SizeBytes     int64             `json:"size_bytes,omitempty"`
	ReleaseType   string            `json:"release_type,omitempty"`
	GameVersions  []string          `json:"game_versions,omitempty"`
	Loaders       []string          `json:"loaders,omitempty"`
	DatePublished string            `json:"date_published,omitempty"`
	IsCurrent     bool              `json:"is_current,omitempty"`
}

// ModUpdateInfo describes an update available for an installed mod.
type ModUpdateInfo struct {
	Hashes            map[string]string `json:"hashes,omitempty"`
	ModName           string            `json:"mod_name"`
	ModFile           string            `json:"mod_file"`
	Provider          string            `json:"provider"`
	ProjectID         string            `json:"project_id,omitempty"`
	ProjectSlug       string            `json:"project_slug,omitempty"`
	Title             string            `json:"title"`
	CurrentVersion    string            `json:"current_version,omitempty"`
	LatestVersion     string            `json:"latest_version"`
	LatestJar         string            `json:"latest_jar"`
	LatestDownloadURL string            `json:"latest_download_url,omitempty"`
	CatalogVersionID  string            `json:"catalog_version_id,omitempty"`
	LatestReleaseType string            `json:"latest_release_type,omitempty"`
	LatestReleaseDate string            `json:"latest_release_date,omitempty"`
	Changelog         string            `json:"changelog,omitempty"`
	AvailableJars     []AvailableModJar `json:"available_jars,omitempty"`
}

// ServerModUpdatesResponse is returned by CheckModUpdates.
type ServerModUpdatesResponse struct {
	Updates     map[string]ModUpdateInfo `json:"updates"`
	LastChecked time.Time                `json:"last_checked,omitzero"`
	TotalMods   int                      `json:"total_mods"`
	UpdateCount int                      `json:"update_count"`
	Errors      []string                 `json:"errors,omitempty"`
}

// Modrinth API structs
type modrinthVersionFile struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
}

type modrinthVersion struct {
	ID            string                `json:"id"`
	ProjectID     string                `json:"project_id"`
	Name          string                `json:"name"`
	VersionNumber string                `json:"version_number"`
	GameVersions  []string              `json:"game_versions"`
	VersionType   string                `json:"version_type"`
	Loaders       []string              `json:"loaders"`
	DatePublished string                `json:"date_published"`
	Files         []modrinthVersionFile `json:"files"`
	Changelog     string                `json:"changelog"`
}

// Hangar API structs
type hangarVersionDownload struct {
	FileInfo *struct {
		Name      string `json:"name"`
		SizeBytes int64  `json:"sizeBytes"`
		SHA256    string `json:"sha256Hash"`
	} `json:"fileInfo"`
	DownloadURL string `json:"downloadUrl"`
}

type hangarVersion struct {
	Channel              struct{ Name string }            `json:"channel"`
	PlatformDependencies map[string][]string              `json:"platformDependencies"`
	Name                 string                           `json:"name"`
	CreatedAt            string                           `json:"createdAt"`
	Downloads            map[string]hangarVersionDownload `json:"downloads"`
}

// Spiget API structs
type spigetVersion struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ReleaseDate int64  `json:"releaseDate"`
}

// CurseForge API structs
type curseForgeHash struct {
	Value string `json:"value"`
	Algo  int    `json:"algo"`
}
type curseForgeFile struct {
	Hashes       []curseForgeHash `json:"hashes"`
	ID           int              `json:"id"`
	DisplayName  string           `json:"displayName"`
	FileName     string           `json:"fileName"`
	FileDate     string           `json:"fileDate"`
	FileLength   int64            `json:"fileLength"`
	DownloadURL  string           `json:"downloadUrl"`
	GameVersions []string         `json:"gameVersions"`
	ReleaseType  int              `json:"releaseType"`
}

type curseForgeFilesResponse struct {
	Data       []curseForgeFile         `json:"data"`
	Pagination struct{ TotalCount int } `json:"pagination"`
}

func serverLoadersForType(serverType string) []string {
	switch strings.ToLower(serverType) {
	case "paper", "spigot", "purpur", "pufferfish", "leaf":
		return []string{"paper", "purpur", "spigot", "bukkit"}
	case "folia":
		return []string{"folia", "paper", "purpur", "spigot", "bukkit"}
	case "fabric":
		return []string{"fabric", "quilt"}
	case "quilt":
		return []string{"quilt", "fabric"}
	case "forge", "crucible":
		return []string{"forge"}
	case "neoforge":
		return []string{"neoforge"}
	case "sponge":
		return []string{"sponge"}
	case "ketting", "mohist", "youer":
		return []string{"forge", "neoforge", "paper", "spigot", "bukkit"}
	case "waterfall", "bungeecord":
		return []string{"bungeecord", "waterfall"}
	case "limbo", "nanolimbo":
		return []string{"limbo"}
	case "geysermc":
		return []string{"geyser"}
	default:
		return nil
	}
}

// Ensure proxy service is initialized on the store.
func (s *Store) getProxy() *proxy.Service {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if s.proxy == nil {
		s.proxy = proxy.NewService()
	}
	return s.proxy
}

// SetProxy allows injecting a shared proxy service into Store.
func (s *Store) SetProxy(p *proxy.Service) {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	s.proxy = p
}

// ModCompatibilityStatus categorizes a mod's readiness for a target Minecraft version.
type ModCompatibilityStatus string

const (
	ModCompatUpdateAvailable   ModCompatibilityStatus = "update_available"
	ModCompatAlreadyCompatible ModCompatibilityStatus = "already_compatible"
	ModCompatNotAvailable      ModCompatibilityStatus = "not_available"
	ModCompatUntracked         ModCompatibilityStatus = "untracked"
)

// ModTargetCompatibility describes the compatibility status of an installed mod for a target MC version.
type ModTargetCompatibility struct {
	Hashes            map[string]string      `json:"hashes,omitempty"`
	ModName           string                 `json:"mod_name"`
	ModFile           string                 `json:"mod_file"`
	Title             string                 `json:"title"`
	Provider          string                 `json:"provider,omitempty"`
	ProjectID         string                 `json:"project_id,omitempty"`
	ProjectSlug       string                 `json:"project_slug,omitempty"`
	CurrentVersion    string                 `json:"current_version,omitempty"`
	Status            ModCompatibilityStatus `json:"status"`
	CompatibleVersion string                 `json:"compatible_version,omitempty"`
	CompatibleJar     string                 `json:"compatible_jar,omitempty"`
	DownloadURL       string                 `json:"download_url,omitempty"`
	CatalogVersionID  string                 `json:"catalog_version_id,omitempty"`
	ReleaseType       string                 `json:"release_type,omitempty"`
	ReleaseDate       string                 `json:"release_date,omitempty"`
	Changelog         string                 `json:"changelog,omitempty"`
	Notes             string                 `json:"notes,omitempty"`
}

// VersionUpdateReport is the comprehensive report produced by CheckTargetVersionCompatibility.
type VersionUpdateReport struct {
	CurrentVersion    string                            `json:"current_version"`
	TargetVersion     string                            `json:"target_version"`
	ServerType        string                            `json:"server_type"`
	TotalMods         int                               `json:"total_mods"`
	CompatibleCount   int                               `json:"compatible_count"`
	UpdateCount       int                               `json:"update_count"`
	NotAvailableCount int                               `json:"not_available_count"`
	UntrackedCount    int                               `json:"untracked_count"`
	ReadyToUpdate     bool                              `json:"ready_to_update"`
	Mods              map[string]ModTargetCompatibility `json:"mods"`
	LastChecked       time.Time                         `json:"last_checked"`
}

// BatchUpdateModItem reports the result of updating a single mod in a batch operation.
type BatchUpdateModItem struct {
	ModName  string `json:"mod_name"`
	Original string `json:"original"`
	NewJar   string `json:"new_jar"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// BatchUpdateModsResult summarizes the results of batch updating mods.
type BatchUpdateModsResult struct {
	TargetVersion string               `json:"target_version"`
	Total         int                  `json:"total"`
	Updated       int                  `json:"updated"`
	Failed        int                  `json:"failed"`
	Items         []BatchUpdateModItem `json:"items"`
}

// InvalidateModUpdatesCache purges the cached update state for a server.
func (s *Store) InvalidateModUpdatesCache(serverID string) {
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE server_update_checks SET mods = 'null' WHERE server_id = ?`, serverID)
	}
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	if s.updatesCache != nil {
		delete(s.updatesCache, serverID)
	}
	if s.targetUpdatesCache != nil {
		prefix := serverID + ":"
		for k := range s.targetUpdatesCache {
			if strings.HasPrefix(k, prefix) || k == serverID {
				delete(s.targetUpdatesCache, k)
			}
		}
	}
}

func (s *Store) setUpdatesCache(ctx context.Context, srv Server, resp *ServerModUpdatesResponse) error {
	if s.db != nil && len(resp.Errors) == 0 {
		raw, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		// Manual checks must create saved results even before the first fleet scan.
		// Keep software results only when they describe the same configuration.
		res, err := s.db.ExecContext(ctx, `INSERT INTO server_update_checks (server_id,server_type,version,build,mods,checked_at)
			SELECT id,server_type,version,COALESCE(build,''),?,? FROM servers WHERE id=? AND server_type=? AND version=? AND COALESCE(build,'')=?
			ON CONFLICT(server_id) DO UPDATE SET
				software=CASE WHEN server_update_checks.server_type=excluded.server_type AND server_update_checks.version=excluded.version AND server_update_checks.build=excluded.build THEN server_update_checks.software ELSE 'null' END,
				error=CASE WHEN server_update_checks.server_type=excluded.server_type AND server_update_checks.version=excluded.version AND server_update_checks.build=excluded.build THEN server_update_checks.error ELSE '' END,
				server_type=excluded.server_type,version=excluded.version,build=excluded.build,mods=excluded.mods,checked_at=excluded.checked_at`,
			string(raw), resp.LastChecked.UTC().Format(time.RFC3339), srv.ID, srv.ServerType, srv.Version, srv.Build)
		if err != nil {
			return err
		}
		if count, err := res.RowsAffected(); err != nil {
			return err
		} else if count == 0 {
			// The server changed while its catalogs were being checked.
			return nil
		}
	}
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	if s.updatesCache == nil {
		s.updatesCache = make(map[string]*ServerModUpdatesResponse)
	}
	s.updatesCache[srv.ID] = resp
	return nil
}

func (s *Store) setTargetUpdatesCache(cacheKey string, resp *VersionUpdateReport) {
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	if s.targetUpdatesCache == nil {
		s.targetUpdatesCache = make(map[string]*VersionUpdateReport)
	}
	s.targetUpdatesCache[cacheKey] = resp
}

// CheckModUpdates compares all installed mods against upstream catalogs
// (Modrinth hash batch check, Hangar, Spiget, CurseForge) using server-side
// caching and rate-limited requests.
func (s *Store) CheckModUpdates(ctx context.Context, serverID string, force bool) (*ServerModUpdatesResponse, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	cfKey, err := curseforge.APIKey(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("read CurseForge settings: %w", err)
	}

	if _, err := modDirForType(srv.ServerType); err != nil {
		return &ServerModUpdatesResponse{
			Updates:     make(map[string]ModUpdateInfo),
			LastChecked: time.Now(),
			TotalMods:   0,
			UpdateCount: 0,
		}, nil
	}

	// 1. Check in-memory cache if not forced
	if !force {
		s.updatesMu.RLock()
		if s.updatesCache != nil {
			if cached, ok := s.updatesCache[serverID]; ok {
				if time.Since(cached.LastChecked) < 10*time.Minute {
					s.updatesMu.RUnlock()
					return cached, nil
				}
			}
		}
		s.updatesMu.RUnlock()
	}

	// 2. Coalesce concurrent requests for the same server
	s.updatesFlightMu.Lock()
	if s.updatesInFlight == nil {
		s.updatesInFlight = make(map[string]*updatesFlightCall)
	}
	if call, ok := s.updatesInFlight[serverID]; ok {
		s.updatesFlightMu.Unlock()
		call.wg.Wait()
		return call.resp, call.err
	}
	call := &updatesFlightCall{}
	call.wg.Add(1)
	s.updatesInFlight[serverID] = call
	s.updatesFlightMu.Unlock()

	var result *ServerModUpdatesResponse
	var checkErr error
	defer func() {
		call.resp = result
		call.err = checkErr
		s.updatesFlightMu.Lock()
		delete(s.updatesInFlight, serverID)
		s.updatesFlightMu.Unlock()
		call.wg.Done()
	}()

	// 3. Fetch list of installed mods
	listRes, err := s.ListMods(ctx, serverID)
	if err != nil {
		checkErr = err
		return nil, checkErr
	}
	installedMods := listRes.Items
	if len(installedMods) == 0 {
		result = &ServerModUpdatesResponse{
			Updates:     make(map[string]ModUpdateInfo),
			LastChecked: time.Now(),
			TotalMods:   0,
			UpdateCount: 0,
		}
		if checkErr = s.setUpdatesCache(ctx, srv, result); checkErr != nil {
			return nil, checkErr
		}
		return result, nil
	}

	updates := make(map[string]ModUpdateInfo)
	px := s.getProxy()
	gameVersion := strings.TrimSpace(srv.Version)
	installedVersions := modrinthInstalledVersions(ctx, px, installedMods, force)
	for i, mod := range installedMods {
		installedMods[i] = withInstalledModrinthVersion(mod, installedVersions[mod.SHA1])
	}
	hashUpdates, upstreamErrors := modrinthUpdatesForChannels(ctx, px, installedMods, serverLoadersForType(srv.ServerType), gameVersion, force)
	for _, mod := range installedMods {
		catalog := modCatalog{Provider: "modrinth", ProjectSlug: mod.ProjectSlug, ProjectID: mod.ProjectID}
		build, found := hashUpdates[mod.File]
		// A project listing can recover a legacy channel when the hash lookup
		// did not identify the installed jar.
		if !found || (mod.ReleaseType == "" && modrinthProject(mod) != "") {
			resolved, err := fetchModCatalog(ctx, px, mod, srv.ServerType, gameVersion, cfKey, force, false)
			if err != nil {
				if !found {
					upstreamErrors = append(upstreamErrors, err.Error())
				}
			} else if choice, ok := selectModBuild(resolved.Builds, installedModChannel(mod, resolved.Builds)); ok {
				catalog, build, found = resolved, choice, true
			} else if len(resolved.Builds) > 0 {
				found = false
			}
		}
		if !found || build.IsCurrent || build.DownloadURL == "" || build.Filename == "" {
			continue
		}
		if build.ProjectID != "" {
			catalog.ProjectID = build.ProjectID
		}
		title := mod.Title
		if title == "" {
			title = mod.Name
		}
		updates[mod.Name] = ModUpdateInfo{
			ModName: mod.Name, ModFile: mod.File, Provider: catalog.Provider,
			ProjectID: catalog.ProjectID, ProjectSlug: catalog.ProjectSlug, Title: title,
			CurrentVersion: mod.Version, LatestVersion: build.VersionNumber, LatestJar: build.Filename,
			LatestDownloadURL: build.DownloadURL, Hashes: build.Hashes, LatestReleaseType: build.ReleaseType,
			LatestReleaseDate: build.DatePublished, Changelog: build.Changelog, CatalogVersionID: build.VersionID,
		}
	}

	result = &ServerModUpdatesResponse{
		Updates:     updates,
		LastChecked: time.Now(),
		TotalMods:   len(installedMods),
		UpdateCount: len(updates),
		Errors:      upstreamErrors,
	}
	if checkErr = s.setUpdatesCache(ctx, srv, result); checkErr != nil {
		return nil, checkErr
	}
	return result, nil
}

// GetModAvailableJars fetches available jar builds from upstream for a specific installed mod.
func (s *Store) GetModAvailableJars(ctx context.Context, serverID, modName string) ([]AvailableModJar, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	cfKey, err := curseforge.APIKey(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("read CurseForge settings: %w", err)
	}

	mod, err := s.GetMod(ctx, serverID, modName)
	if err != nil {
		return nil, err
	}
	px := s.getProxy()
	installed := modrinthInstalledVersions(ctx, px, []Mod{mod}, false)
	mod = withInstalledModrinthVersion(mod, installed[mod.SHA1])
	catalog, err := fetchModCatalog(ctx, px, mod, srv.ServerType, strings.TrimSpace(srv.Version), cfKey, false, true)
	if err != nil {
		return nil, err
	}
	jars := make([]AvailableModJar, 0, len(catalog.Builds))
	for _, build := range catalog.Builds {
		jar := build.AvailableModJar
		jar.Provider, jar.ProjectID, jar.ProjectSlug = catalog.Provider, catalog.ProjectID, catalog.ProjectSlug
		if build.ProjectID != "" {
			jar.ProjectID = build.ProjectID
		}
		jars = append(jars, jar)
	}
	return jars, nil
}

// CheckTargetVersionCompatibility inspects all installed mods against upstream catalogs
// to determine if each mod is compatible with or has an update available for targetVersion.
func (s *Store) CheckTargetVersionCompatibility(ctx context.Context, serverID, targetVersion string, force bool) (*VersionUpdateReport, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	cfKey, err := curseforge.APIKey(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("read CurseForge settings: %w", err)
	}

	if _, err := modDirForType(srv.ServerType); err != nil {
		return &VersionUpdateReport{
			CurrentVersion:    srv.Version,
			TargetVersion:     targetVersion,
			ServerType:        srv.ServerType,
			TotalMods:         0,
			CompatibleCount:   0,
			UpdateCount:       0,
			NotAvailableCount: 0,
			UntrackedCount:    0,
			ReadyToUpdate:     true,
			Mods:              make(map[string]ModTargetCompatibility),
			LastChecked:       time.Now(),
		}, nil
	}

	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		targetVersion = strings.TrimSpace(srv.Version)
	}

	cacheKey := fmt.Sprintf("%s:%s", serverID, targetVersion)

	// 1. Check in-memory cache if not forced
	if !force {
		s.updatesMu.RLock()
		if s.targetUpdatesCache != nil {
			if cached, ok := s.targetUpdatesCache[cacheKey]; ok {
				if time.Since(cached.LastChecked) < 10*time.Minute {
					s.updatesMu.RUnlock()
					return cached, nil
				}
			}
		}
		s.updatesMu.RUnlock()
	}

	// 2. Coalesce concurrent requests
	s.targetUpdatesFlightMu.Lock()
	if s.targetUpdatesInFlight == nil {
		s.targetUpdatesInFlight = make(map[string]*targetUpdatesFlightCall)
	}
	if call, ok := s.targetUpdatesInFlight[cacheKey]; ok {
		s.targetUpdatesFlightMu.Unlock()
		call.wg.Wait()
		return call.resp, call.err
	}
	call := &targetUpdatesFlightCall{}
	call.wg.Add(1)
	s.targetUpdatesInFlight[cacheKey] = call
	s.targetUpdatesFlightMu.Unlock()

	var result *VersionUpdateReport
	var checkErr error
	defer func() {
		call.resp = result
		call.err = checkErr
		s.targetUpdatesFlightMu.Lock()
		delete(s.targetUpdatesInFlight, cacheKey)
		s.targetUpdatesFlightMu.Unlock()
		call.wg.Done()
	}()

	// 3. Fetch list of installed mods
	listRes, err := s.ListMods(ctx, serverID)
	if err != nil {
		checkErr = err
		return nil, checkErr
	}
	installedMods := listRes.Items
	if len(installedMods) == 0 {
		result = &VersionUpdateReport{
			CurrentVersion:    srv.Version,
			TargetVersion:     targetVersion,
			ServerType:        srv.ServerType,
			TotalMods:         0,
			CompatibleCount:   0,
			UpdateCount:       0,
			NotAvailableCount: 0,
			UntrackedCount:    0,
			ReadyToUpdate:     true,
			Mods:              make(map[string]ModTargetCompatibility),
			LastChecked:       time.Now(),
		}
		s.setTargetUpdatesCache(cacheKey, result)
		return result, nil
	}

	modsResult := make(map[string]ModTargetCompatibility)
	px := s.getProxy()
	installedVersions := modrinthInstalledVersions(ctx, px, installedMods, force)
	for i, mod := range installedMods {
		installedMods[i] = withInstalledModrinthVersion(mod, installedVersions[mod.SHA1])
	}
	hashUpdates, _ := modrinthUpdatesForChannels(ctx, px, installedMods, serverLoadersForType(srv.ServerType), targetVersion, force)
	for _, mod := range installedMods {
		title := mod.Title
		if title == "" {
			title = mod.Name
		}
		item := ModTargetCompatibility{
			ModName: mod.Name, ModFile: mod.File, Title: title, Provider: mod.Provider,
			ProjectID: mod.ProjectID, ProjectSlug: mod.ProjectSlug, CurrentVersion: mod.Version, Status: ModCompatUntracked,
		}
		build, found := hashUpdates[mod.File]
		if found {
			item.Provider = "modrinth"
		}
		if !found || (mod.ReleaseType == "" && modrinthProject(mod) != "") {
			catalog, err := fetchModCatalog(ctx, px, mod, srv.ServerType, targetVersion, cfKey, force, false)
			if err == nil && catalog.Provider != "" {
				item.Provider, item.ProjectID, item.ProjectSlug = catalog.Provider, catalog.ProjectID, catalog.ProjectSlug
				channel := installedModChannel(mod, catalog.Builds)
				if mod.ReleaseType == "" && len(catalog.Builds) > 0 && targetVersion != srv.Version && mod.Provider != "spiget" {
					current, err := fetchModCatalog(ctx, px, mod, srv.ServerType, srv.Version, cfKey, false, false)
					if err == nil {
						channel = installedModChannel(mod, append(current.Builds, catalog.Builds...))
					}
				}
				if choice, ok := selectModBuild(catalog.Builds, channel); ok || len(catalog.Builds) > 0 {
					build, found = choice, ok
				}
			}
		}
		if item.Provider != "" {
			item.Status = ModCompatNotAvailable
		}
		if found {
			item.Status = ModCompatUpdateAvailable
			if build.IsCurrent {
				item.Status = ModCompatAlreadyCompatible
			}
			if build.ProjectID != "" {
				item.ProjectID = build.ProjectID
			}
			item.CompatibleVersion, item.CompatibleJar = build.VersionNumber, build.Filename
			item.DownloadURL, item.Hashes = build.DownloadURL, build.Hashes
			item.ReleaseType, item.ReleaseDate = build.ReleaseType, build.DatePublished
			item.CatalogVersionID, item.Changelog = build.VersionID, build.Changelog
		}
		modsResult[mod.Name] = item
	}

	// 6. Aggregate report metrics
	var compatCount, updateCount, notAvailCount, untrackedCount int
	for _, m := range modsResult {
		switch m.Status {
		case ModCompatAlreadyCompatible:
			compatCount++
		case ModCompatUpdateAvailable:
			compatCount++
			updateCount++
		case ModCompatNotAvailable:
			notAvailCount++
		case ModCompatUntracked:
			untrackedCount++
		}
	}

	readyToUpdate := notAvailCount == 0 && len(installedMods) > 0

	result = &VersionUpdateReport{
		CurrentVersion:    srv.Version,
		TargetVersion:     targetVersion,
		ServerType:        srv.ServerType,
		TotalMods:         len(installedMods),
		CompatibleCount:   compatCount,
		UpdateCount:       updateCount,
		NotAvailableCount: notAvailCount,
		UntrackedCount:    untrackedCount,
		ReadyToUpdate:     readyToUpdate,
		Mods:              modsResult,
		LastChecked:       time.Now(),
	}

	s.setTargetUpdatesCache(cacheKey, result)
	return result, nil
}

// BatchUpdateTargetMods applies compatible mod updates for targetVersion.
// If modNames is empty, all mods with status "update_available" will be updated.
func (s *Store) BatchUpdateTargetMods(ctx context.Context, serverID, targetVersion string, modNames []string) (*BatchUpdateModsResult, error) {
	report, err := s.CheckTargetVersionCompatibility(ctx, serverID, targetVersion, false)
	if err != nil {
		return nil, err
	}

	var toUpdate []ModTargetCompatibility
	targetMap := make(map[string]bool)
	for _, name := range modNames {
		targetMap[strings.ToLower(name)] = true
	}

	for _, m := range report.Mods {
		if m.Status != ModCompatUpdateAvailable || m.DownloadURL == "" || m.CompatibleJar == "" {
			continue
		}
		if len(modNames) == 0 || targetMap[strings.ToLower(m.ModName)] || targetMap[strings.ToLower(m.ModFile)] {
			toUpdate = append(toUpdate, m)
		}
	}

	sort.Slice(toUpdate, func(i, j int) bool {
		ti := toUpdate[i].Title
		if ti == "" {
			ti = toUpdate[i].ModName
		}
		tj := toUpdate[j].Title
		if tj == "" {
			tj = toUpdate[j].ModName
		}
		return ti < tj
	})

	res := &BatchUpdateModsResult{
		TargetVersion: targetVersion,
		Total:         len(toUpdate),
		Items:         make([]BatchUpdateModItem, 0, len(toUpdate)),
	}

	for i, item := range toUpdate {
		percent := 0
		if len(toUpdate) > 0 {
			percent = (i * 100) / len(toUpdate)
		}
		title := item.Title
		if title == "" {
			title = item.ModName
		}
		s.SetTaskProgress(serverID, TaskProgress{
			ServerID:   serverID,
			Operation:  "mod_upgrade_batch",
			Stage:      "downloading_mods",
			StageTitle: fmt.Sprintf("Updating Mods (%d/%d)", i+1, len(toUpdate)),
			Percent:    percent,
			Current:    int64(i + 1),
			Total:      int64(len(toUpdate)),
			Message:    fmt.Sprintf("Updating %s (%d/%d)...", title, i+1, len(toUpdate)),
		})

		meta := ModDownloadMeta{
			Hashes:           item.Hashes,
			ProjectID:        item.ProjectID,
			ProjectSlug:      item.ProjectSlug,
			Provider:         item.Provider,
			CatalogVersionID: item.CatalogVersionID,
			ReleaseType:      item.ReleaseType,
		}

		// Download new jar first to prevent losing the existing jar if download fails
		_, downErr := s.ReplaceModFromURL(ctx, serverID, item.CompatibleJar, item.DownloadURL, item.ModFile, false, meta)
		if downErr != nil {
			res.Failed++
			res.Items = append(res.Items, BatchUpdateModItem{
				ModName:  item.ModName,
				Original: item.ModFile,
				NewJar:   item.CompatibleJar,
				Success:  false,
				Error:    downErr.Error(),
			})
		} else {
			res.Updated++
			res.Items = append(res.Items, BatchUpdateModItem{
				ModName:  item.ModName,
				Original: item.ModFile,
				NewJar:   item.CompatibleJar,
				Success:  true,
			})
		}
	}

	s.ClearTaskProgress(serverID)
	s.InvalidateModUpdatesCache(serverID)

	return res, nil
}

// BatchUpdateMods updates installed mods to their latest versions for the current server version.
// If modNames is empty, all mods with available updates in CheckModUpdates will be updated.
func (s *Store) BatchUpdateMods(ctx context.Context, serverID string, modNames []string) (*BatchUpdateModsResult, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}

	updatesRes, err := s.CheckModUpdates(ctx, serverID, false)
	if err != nil {
		return nil, err
	}

	targetMap := make(map[string]bool)
	for _, name := range modNames {
		targetMap[strings.ToLower(name)] = true
	}

	var toUpdate []ModUpdateInfo
	for _, u := range updatesRes.Updates {
		if u.LatestDownloadURL == "" || u.LatestJar == "" {
			continue
		}
		if len(modNames) == 0 || targetMap[strings.ToLower(u.ModName)] || targetMap[strings.ToLower(u.ModFile)] {
			toUpdate = append(toUpdate, u)
		}
	}

	sort.Slice(toUpdate, func(i, j int) bool {
		ti := toUpdate[i].Title
		if ti == "" {
			ti = toUpdate[i].ModName
		}
		tj := toUpdate[j].Title
		if tj == "" {
			tj = toUpdate[j].ModName
		}
		return ti < tj
	})

	res := &BatchUpdateModsResult{
		TargetVersion: srv.Version,
		Total:         len(toUpdate),
		Items:         make([]BatchUpdateModItem, 0, len(toUpdate)),
	}

	for i, item := range toUpdate {
		percent := 0
		if len(toUpdate) > 0 {
			percent = (i * 100) / len(toUpdate)
		}
		title := item.Title
		if title == "" {
			title = item.ModName
		}
		s.SetTaskProgress(serverID, TaskProgress{
			ServerID:   serverID,
			Operation:  "mod_update_batch",
			Stage:      "downloading_mods",
			StageTitle: fmt.Sprintf("Updating Mods (%d/%d)", i+1, len(toUpdate)),
			Percent:    percent,
			Current:    int64(i + 1),
			Total:      int64(len(toUpdate)),
			Message:    fmt.Sprintf("Updating %s (%d/%d)...", title, i+1, len(toUpdate)),
		})

		meta := ModDownloadMeta{
			Hashes:           item.Hashes,
			ProjectID:        item.ProjectID,
			ProjectSlug:      item.ProjectSlug,
			Provider:         item.Provider,
			CatalogVersionID: item.CatalogVersionID,
			ReleaseType:      item.LatestReleaseType,
		}

		// Download new jar first to prevent losing the existing jar if download fails
		_, downErr := s.ReplaceModFromURL(ctx, serverID, item.LatestJar, item.LatestDownloadURL, item.ModFile, false, meta)
		if downErr != nil {
			res.Failed++
			res.Items = append(res.Items, BatchUpdateModItem{
				ModName:  item.ModName,
				Original: item.ModFile,
				NewJar:   item.LatestJar,
				Success:  false,
				Error:    downErr.Error(),
			})
		} else {
			res.Updated++
			res.Items = append(res.Items, BatchUpdateModItem{
				ModName:  item.ModName,
				Original: item.ModFile,
				NewJar:   item.LatestJar,
				Success:  true,
			})
		}
	}

	s.ClearTaskProgress(serverID)
	s.InvalidateModUpdatesCache(serverID)

	return res, nil
}

func curseForgeHashes(hashes []curseForgeHash) map[string]string {
	result := map[string]string{}
	for _, hash := range hashes {
		switch hash.Algo {
		case 1:
			result["sha1"] = hash.Value
		case 2:
			result["md5"] = hash.Value
		}
	}
	return result
}
