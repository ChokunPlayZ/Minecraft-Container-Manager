package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy"
)

type contextKey string

const CurseForgeAPIKeyContextKey contextKey = "curseforge_api_key"

// WithCurseForgeAPIKey returns a new context carrying the CurseForge API key.
func WithCurseForgeAPIKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, CurseForgeAPIKeyContextKey, key)
}

func getCurseForgeKey(ctx context.Context) string {
	if val, ok := ctx.Value(CurseForgeAPIKeyContextKey).(string); ok && val != "" {
		return val
	}
	return os.Getenv("CURSEFORGE_API_KEY")
}

func cleanVersionString(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "release-")
	v = strings.TrimPrefix(v, "build-")
	return v
}

// AvailableModJar describes a single installable build of a mod or plugin.
type AvailableModJar struct {
	VersionID     string   `json:"version_id"`
	VersionName   string   `json:"version_name"`
	VersionNumber string   `json:"version_number"`
	Filename      string   `json:"filename"`
	DownloadURL   string   `json:"download_url"`
	SizeBytes     int64    `json:"size_bytes,omitempty"`
	ReleaseType   string   `json:"release_type,omitempty"`
	GameVersions  []string `json:"game_versions,omitempty"`
	Loaders       []string `json:"loaders,omitempty"`
	DatePublished string   `json:"date_published,omitempty"`
	IsCurrent     bool     `json:"is_current,omitempty"`
}

// ModUpdateInfo describes an update available for an installed mod.
type ModUpdateInfo struct {
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
	LatestReleaseType string            `json:"latest_release_type,omitempty"`
	LatestReleaseDate string            `json:"latest_release_date,omitempty"`
	Changelog         string            `json:"changelog,omitempty"`
	AvailableJars     []AvailableModJar `json:"available_jars,omitempty"`
}

// ServerModUpdatesResponse is returned by CheckModUpdates.
type ServerModUpdatesResponse struct {
	Updates     map[string]ModUpdateInfo `json:"updates"`
	LastChecked time.Time                `json:"last_checked"`
	TotalMods   int                      `json:"total_mods"`
	UpdateCount int                      `json:"update_count"`
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
	} `json:"fileInfo"`
	DownloadURL string `json:"downloadUrl"`
}

type hangarVersion struct {
	Name      string                           `json:"name"`
	CreatedAt string                           `json:"createdAt"`
	Downloads map[string]hangarVersionDownload `json:"downloads"`
}

// Spiget API structs
type spigetVersion struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ReleaseDate int64  `json:"releaseDate"`
}

// CurseForge API structs
type curseForgeFile struct {
	ID           int      `json:"id"`
	DisplayName  string   `json:"displayName"`
	FileName     string   `json:"fileName"`
	FileDate     string   `json:"fileDate"`
	FileLength   int64    `json:"fileLength"`
	DownloadURL  string   `json:"downloadUrl"`
	GameVersions []string `json:"gameVersions"`
	ReleaseType  int      `json:"releaseType"`
}

type curseForgeFilesResponse struct {
	Data []curseForgeFile `json:"data"`
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
	case "ketting", "mohist":
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

// InvalidateModUpdatesCache purges the cached update state for a server.
func (s *Store) InvalidateModUpdatesCache(serverID string) {
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	if s.updatesCache != nil {
		delete(s.updatesCache, serverID)
	}
}

func (s *Store) setUpdatesCache(serverID string, resp *ServerModUpdatesResponse) {
	s.updatesMu.Lock()
	defer s.updatesMu.Unlock()
	if s.updatesCache == nil {
		s.updatesCache = make(map[string]*ServerModUpdatesResponse)
	}
	s.updatesCache[serverID] = resp
}

// CheckModUpdates compares all installed mods against upstream catalogs
// (Modrinth hash batch check, Hangar, Spiget, CurseForge) using server-side
// caching and rate-limited requests.
func (s *Store) CheckModUpdates(ctx context.Context, serverID string, force bool) (*ServerModUpdatesResponse, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
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
		s.setUpdatesCache(serverID, result)
		return result, nil
	}

	updates := make(map[string]ModUpdateInfo)
	serverLoaders := serverLoadersForType(srv.ServerType)
	serverVersion := strings.TrimSpace(srv.Version)
	px := s.getProxy()

	// 3. Batch check via Modrinth for all mods with SHA1 hashes
	var modsWithSha1 []Mod
	var hashes []string
	for _, m := range installedMods {
		if m.SHA1 != "" && (m.Provider == "modrinth" || m.Provider == "") {
			modsWithSha1 = append(modsWithSha1, m)
			hashes = append(hashes, m.SHA1)
		}
	}

	if len(hashes) > 0 {
		payload := map[string]any{
			"hashes":    hashes,
			"algorithm": "sha1",
		}
		if len(serverLoaders) > 0 {
			payload["loaders"] = serverLoaders
		}
		if serverVersion != "" {
			payload["game_versions"] = []string{serverVersion}
		}

		bodyBytes, _ := json.Marshal(payload)
		resp, err := px.Do(ctx, http.MethodPost, "https://api.modrinth.com/v2/version_files/update", bodyBytes, nil, force)
		if err == nil && resp.StatusCode == http.StatusOK {
			var modrinthUpdates map[string]modrinthVersion
			if json.Unmarshal(resp.Body, &modrinthUpdates) == nil {
				for _, mod := range modsWithSha1 {
					ver, exists := modrinthUpdates[mod.SHA1]
					if !exists || len(ver.Files) == 0 {
						continue
					}

					var primaryFile *modrinthVersionFile
					for i := range ver.Files {
						if ver.Files[i].Primary {
							primaryFile = &ver.Files[i]
							break
						}
					}
					if primaryFile == nil {
						primaryFile = &ver.Files[0]
					}

					isSameHash := primaryFile.Hashes["sha1"] != "" &&
						strings.EqualFold(primaryFile.Hashes["sha1"], mod.SHA1)
					isSameFile := strings.EqualFold(primaryFile.Filename, mod.File) ||
						strings.EqualFold(primaryFile.Filename+".disabled", mod.File) ||
						strings.EqualFold(primaryFile.Filename, strings.TrimSuffix(mod.File, ".disabled"))

					if isSameHash && isSameFile {
						continue
					}

					// Persist metadata if it was not tracked before
					if modDir, _, err := s.modsPath(serverID, srv.ServerType); err == nil && (mod.Provider == "" || mod.ProjectID == "") {
						writeModMeta(modDir, mod.File, ModDownloadMeta{
							Provider:    "modrinth",
							ProjectID:   ver.ProjectID,
							ProjectSlug: mod.ProjectSlug,
						}, mod.ModID)
					}

					updates[mod.Name] = ModUpdateInfo{
						ModName:           mod.Name,
						ModFile:           mod.File,
						Provider:          "modrinth",
						ProjectID:         ver.ProjectID,
						ProjectSlug:       mod.ProjectSlug,
						Title:             mod.Title,
						CurrentVersion:    mod.Version,
						LatestVersion:     ver.VersionNumber,
						LatestJar:         primaryFile.Filename,
						LatestDownloadURL: primaryFile.URL,
						LatestReleaseType: ver.VersionType,
						LatestReleaseDate: ver.DatePublished,
						Changelog:         ver.Changelog,
					}
					if updates[mod.Name].Title == "" {
						u := updates[mod.Name]
						u.Title = mod.Name
						updates[mod.Name] = u
					}
					if updates[mod.Name].LatestVersion == "" {
						u := updates[mod.Name]
						u.LatestVersion = ver.Name
						updates[mod.Name] = u
					}
				}
			}
		}
	}

	// 4. Check remaining mods not updated by Modrinth batch check
	for _, mod := range installedMods {
		if _, ok := updates[mod.Name]; ok {
			continue
		}

		// Hangar provider
		if mod.Provider == "hangar" && (mod.ProjectSlug != "" || mod.ProjectID != "") {
			slug := mod.ProjectSlug
			if slug == "" {
				slug = mod.ProjectID
			}
			hangarURL := fmt.Sprintf("https://hangar.papermc.io/api/v1/projects/%s/versions?limit=5", url.PathEscape(slug))
			resp, err := px.Do(ctx, http.MethodGet, hangarURL, nil, nil, force)
			if err == nil && resp.StatusCode == http.StatusOK {
				var vers struct {
					Result []hangarVersion `json:"result"`
				}
				if json.Unmarshal(resp.Body, &vers) == nil && len(vers.Result) > 0 {
					latest := vers.Result[0]
					var dl hangarVersionDownload
					if pDl, ok := latest.Downloads["PAPER"]; ok && pDl.FileInfo != nil {
						dl = pDl
					} else {
						for _, anyDl := range latest.Downloads {
							if anyDl.FileInfo != nil && anyDl.FileInfo.Name != "" {
								dl = anyDl
								break
							}
						}
					}
					if dl.FileInfo != nil && dl.FileInfo.Name != "" {
						isSame := strings.EqualFold(dl.FileInfo.Name, mod.File) ||
							strings.EqualFold(dl.FileInfo.Name+".disabled", mod.File) ||
							(mod.Version != "" && cleanVersionString(mod.Version) == cleanVersionString(latest.Name))
						if !isSame {
							title := mod.Title
							if title == "" {
								title = mod.Name
							}
							updates[mod.Name] = ModUpdateInfo{
								ModName:           mod.Name,
								ModFile:           mod.File,
								Provider:          "hangar",
								ProjectSlug:       slug,
								ProjectID:         mod.ProjectID,
								Title:             title,
								CurrentVersion:    mod.Version,
								LatestVersion:     latest.Name,
								LatestJar:         dl.FileInfo.Name,
								LatestDownloadURL: dl.DownloadURL,
								LatestReleaseDate: latest.CreatedAt,
							}
						}
					}
				}
			}
			continue
		}

		// Spiget provider
		if mod.Provider == "spiget" && mod.ProjectID != "" {
			if resID, err := strconv.Atoi(mod.ProjectID); err == nil {
				spigetURL := fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions?size=5&sort=-releaseDate", resID)
				resp, err := px.Do(ctx, http.MethodGet, spigetURL, nil, nil, force)
				if err == nil && resp.StatusCode == http.StatusOK {
					var vers []spigetVersion
					if json.Unmarshal(resp.Body, &vers) == nil && len(vers) > 0 {
						latest := vers[0]
						currVer := cleanVersionString(mod.Version)
						latestVer := cleanVersionString(latest.Name)
						if currVer == "" || (latestVer != "" && !strings.EqualFold(currVer, latestVer)) {
							title := mod.Title
							if title == "" {
								title = mod.Name
							}
							var relDate string
							if latest.ReleaseDate > 0 {
								relDate = time.Unix(latest.ReleaseDate, 0).UTC().Format(time.RFC3339)
							}
							updates[mod.Name] = ModUpdateInfo{
								ModName:           mod.Name,
								ModFile:           mod.File,
								Provider:          "spiget",
								ProjectID:         strconv.Itoa(resID),
								ProjectSlug:       mod.ProjectSlug,
								Title:             title,
								CurrentVersion:    mod.Version,
								LatestVersion:     latest.Name,
								LatestJar:         fmt.Sprintf("%s.jar", mod.Name),
								LatestDownloadURL: fmt.Sprintf("https://cdn.spiget.org/file/spiget-resources/%d.jar", resID),
								LatestReleaseDate: relDate,
							}
						}
					}
				}
			}
			continue
		}

		// CurseForge provider
		if mod.Provider == "curseforge" && mod.ProjectID != "" {
			cfKey := getCurseForgeKey(ctx)
			if cfKey != "" {
				if modID, err := strconv.Atoi(mod.ProjectID); err == nil {
					cfURL := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files?pageSize=5", modID)
					if serverVersion != "" {
						cfURL += fmt.Sprintf("&gameVersion=%s", url.QueryEscape(serverVersion))
					}
					hdr := http.Header{"x-api-key": []string{cfKey}}
					resp, err := px.Do(ctx, http.MethodGet, cfURL, nil, hdr, force)
					if err == nil && resp.StatusCode == http.StatusOK {
						var cfRes curseForgeFilesResponse
						if json.Unmarshal(resp.Body, &cfRes) == nil && len(cfRes.Data) > 0 {
							latest := cfRes.Data[0]
							isSame := strings.EqualFold(latest.FileName, mod.File) ||
								strings.EqualFold(latest.FileName+".disabled", mod.File) ||
								(mod.Version != "" && cleanVersionString(mod.Version) == cleanVersionString(latest.DisplayName))
							if !isSame && latest.DownloadURL != "" {
								title := mod.Title
								if title == "" {
									title = mod.Name
								}
								latestVer := latest.DisplayName
								if latestVer == "" {
									latestVer = latest.FileName
								}
								updates[mod.Name] = ModUpdateInfo{
									ModName:           mod.Name,
									ModFile:           mod.File,
									Provider:          "curseforge",
									ProjectID:         strconv.Itoa(modID),
									ProjectSlug:       mod.ProjectSlug,
									Title:             title,
									CurrentVersion:    mod.Version,
									LatestVersion:     latestVer,
									LatestJar:         latest.FileName,
									LatestDownloadURL: latest.DownloadURL,
									LatestReleaseDate: latest.FileDate,
								}
							}
						}
					}
				}
			}
			continue
		}

		// Fallback: Modrinth project lookup by slug/id/mod_id
		slugOrID := mod.ProjectSlug
		if slugOrID == "" {
			slugOrID = mod.ProjectID
		}
		if slugOrID == "" {
			slugOrID = mod.ModID
		}
		if slugOrID != "" && (mod.Provider == "modrinth" || mod.Provider == "") {
			reqURL := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", url.PathEscape(slugOrID))
			q := url.Values{}
			if len(serverLoaders) > 0 {
				loadersJSON, _ := json.Marshal(serverLoaders)
				q.Set("loaders", string(loadersJSON))
			}
			if serverVersion != "" {
				versionsJSON, _ := json.Marshal([]string{serverVersion})
				q.Set("game_versions", string(versionsJSON))
			}
			if enc := q.Encode(); enc != "" {
				reqURL += "?" + enc
			}

			resp, err := px.Do(ctx, http.MethodGet, reqURL, nil, nil, force)
			if err == nil && resp.StatusCode == http.StatusOK {
				var vers []modrinthVersion
				if json.Unmarshal(resp.Body, &vers) == nil && len(vers) > 0 {
					latest := vers[0]
					if len(latest.Files) > 0 {
						var primaryFile *modrinthVersionFile
						for i := range latest.Files {
							if latest.Files[i].Primary {
								primaryFile = &latest.Files[i]
								break
							}
						}
						if primaryFile == nil {
							primaryFile = &latest.Files[0]
						}

						isSameFile := strings.EqualFold(primaryFile.Filename, mod.File) ||
							strings.EqualFold(primaryFile.Filename+".disabled", mod.File)
						isSameHash := mod.SHA1 != "" && primaryFile.Hashes["sha1"] != "" &&
							strings.EqualFold(primaryFile.Hashes["sha1"], mod.SHA1)

						if !isSameFile && !isSameHash {
							// Persist metadata if it was not tracked before
							if modDir, _, err := s.modsPath(serverID, srv.ServerType); err == nil && (mod.Provider == "" || mod.ProjectID == "") {
								writeModMeta(modDir, mod.File, ModDownloadMeta{
									Provider:    "modrinth",
									ProjectID:   latest.ProjectID,
									ProjectSlug: mod.ProjectSlug,
								}, mod.ModID)
							}

							title := mod.Title
							if title == "" {
								title = mod.Name
							}
							latestVer := latest.VersionNumber
							if latestVer == "" {
								latestVer = latest.Name
							}
							updates[mod.Name] = ModUpdateInfo{
								ModName:           mod.Name,
								ModFile:           mod.File,
								Provider:          "modrinth",
								ProjectID:         latest.ProjectID,
								ProjectSlug:       mod.ProjectSlug,
								Title:             title,
								CurrentVersion:    mod.Version,
								LatestVersion:     latestVer,
								LatestJar:         primaryFile.Filename,
								LatestDownloadURL: primaryFile.URL,
								LatestReleaseType: latest.VersionType,
								LatestReleaseDate: latest.DatePublished,
								Changelog:         latest.Changelog,
							}
						}
					}
				}
			}
		}
	}

	result = &ServerModUpdatesResponse{
		Updates:     updates,
		LastChecked: time.Now(),
		TotalMods:   len(installedMods),
		UpdateCount: len(updates),
	}
	s.setUpdatesCache(serverID, result)
	return result, nil
}

// GetModAvailableJars fetches available jar builds from upstream for a specific installed mod.
func (s *Store) GetModAvailableJars(ctx context.Context, serverID, modName string) ([]AvailableModJar, error) {
	srv, err := s.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}

	listRes, err := s.ListMods(ctx, serverID)
	if err != nil {
		return nil, err
	}

	var targetMod *Mod
	for i := range listRes.Items {
		m := &listRes.Items[i]
		if m.Name == modName || m.File == modName || modDisplayBase(m.File) == modName {
			targetMod = m
			break
		}
	}
	if targetMod == nil {
		return nil, fmt.Errorf("mod not found: %s", modName)
	}

	serverLoaders := serverLoadersForType(srv.ServerType)
	serverVersion := strings.TrimSpace(srv.Version)
	px := s.getProxy()

	var jars []AvailableModJar

	// 1. Modrinth resolution
	slugOrID := targetMod.ProjectSlug
	if slugOrID == "" {
		slugOrID = targetMod.ProjectID
	}
	if slugOrID == "" {
		slugOrID = targetMod.ModID
	}

	if targetMod.Provider == "modrinth" || (targetMod.Provider == "" && slugOrID != "") {
		reqURL := fmt.Sprintf("https://api.modrinth.com/v2/project/%s/version", url.PathEscape(slugOrID))
		q := url.Values{}
		if len(serverLoaders) > 0 {
			loadersJSON, _ := json.Marshal(serverLoaders)
			q.Set("loaders", string(loadersJSON))
		}
		if serverVersion != "" {
			versionsJSON, _ := json.Marshal([]string{serverVersion})
			q.Set("game_versions", string(versionsJSON))
		}
		if enc := q.Encode(); enc != "" {
			reqURL += "?" + enc
		}

		resp, err := px.Do(ctx, http.MethodGet, reqURL, nil, nil, false)
		if err == nil && resp.StatusCode == http.StatusOK {
			var vers []modrinthVersion
			if json.Unmarshal(resp.Body, &vers) == nil {
				for _, v := range vers {
					for _, f := range v.Files {
						isCur := (targetMod.SHA1 != "" && f.Hashes["sha1"] != "" && strings.EqualFold(targetMod.SHA1, f.Hashes["sha1"])) ||
							strings.EqualFold(f.Filename, targetMod.File) ||
							strings.EqualFold(f.Filename+".disabled", targetMod.File)
						jars = append(jars, AvailableModJar{
							VersionID:     v.ID,
							VersionName:   v.Name,
							VersionNumber: v.VersionNumber,
							Filename:      f.Filename,
							DownloadURL:   f.URL,
							SizeBytes:     f.Size,
							ReleaseType:   v.VersionType,
							GameVersions:  v.GameVersions,
							Loaders:       v.Loaders,
							DatePublished: v.DatePublished,
							IsCurrent:     isCur,
						})
					}
				}
				if len(jars) > 0 {
					return jars, nil
				}
			}
		}
	}

	// 2. Hangar resolution
	if targetMod.Provider == "hangar" {
		slug := targetMod.ProjectSlug
		if slug == "" {
			slug = targetMod.ProjectID
		}
		if slug != "" {
			hURL := fmt.Sprintf("https://hangar.papermc.io/api/v1/projects/%s/versions?limit=25", url.PathEscape(slug))
			resp, err := px.Do(ctx, http.MethodGet, hURL, nil, nil, false)
			if err == nil && resp.StatusCode == http.StatusOK {
				var vers struct {
					Result []hangarVersion `json:"result"`
				}
				if json.Unmarshal(resp.Body, &vers) == nil {
					for _, v := range vers.Result {
						var dl hangarVersionDownload
						if pDl, ok := v.Downloads["PAPER"]; ok && pDl.FileInfo != nil {
							dl = pDl
						} else {
							for _, anyDl := range v.Downloads {
								if anyDl.FileInfo != nil && anyDl.FileInfo.Name != "" {
									dl = anyDl
									break
								}
							}
						}
						if dl.FileInfo != nil && dl.FileInfo.Name != "" {
							isCur := strings.EqualFold(dl.FileInfo.Name, targetMod.File) ||
								strings.EqualFold(dl.FileInfo.Name+".disabled", targetMod.File)
							jars = append(jars, AvailableModJar{
								VersionID:     v.Name,
								VersionName:   v.Name,
								VersionNumber: v.Name,
								Filename:      dl.FileInfo.Name,
								DownloadURL:   dl.DownloadURL,
								SizeBytes:     dl.FileInfo.SizeBytes,
								DatePublished: v.CreatedAt,
								IsCurrent:     isCur,
							})
						}
					}
					if len(jars) > 0 {
						return jars, nil
					}
				}
			}
		}
	}

	// 3. Spiget resolution
	if targetMod.Provider == "spiget" && targetMod.ProjectID != "" {
		if resID, err := strconv.Atoi(targetMod.ProjectID); err == nil {
			sURL := fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions?size=25&sort=-releaseDate", resID)
			resp, err := px.Do(ctx, http.MethodGet, sURL, nil, nil, false)
			if err == nil && resp.StatusCode == http.StatusOK {
				var vers []spigetVersion
				if json.Unmarshal(resp.Body, &vers) == nil {
					for _, v := range vers {
						isCur := targetMod.Version != "" && strings.EqualFold(targetMod.Version, v.Name)
						var relDate string
						if v.ReleaseDate > 0 {
							relDate = time.Unix(v.ReleaseDate, 0).UTC().Format(time.RFC3339)
						}
						jars = append(jars, AvailableModJar{
							VersionID:     strconv.Itoa(v.ID),
							VersionName:   v.Name,
							VersionNumber: v.Name,
							Filename:      fmt.Sprintf("%s-%s.jar", targetMod.Name, v.Name),
							DownloadURL:   fmt.Sprintf("https://cdn.spiget.org/file/spiget-resources/%d.jar", resID),
							DatePublished: relDate,
							IsCurrent:     isCur,
						})
					}
					if len(jars) > 0 {
						return jars, nil
					}
				}
			}
		}
	}

	// 4. CurseForge resolution
	if targetMod.Provider == "curseforge" && targetMod.ProjectID != "" {
		if modID, err := strconv.Atoi(targetMod.ProjectID); err == nil {
			cfURL := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files?pageSize=25", modID)
			if serverVersion != "" {
				cfURL += fmt.Sprintf("&gameVersion=%s", url.QueryEscape(serverVersion))
			}
			cfKey := getCurseForgeKey(ctx)
			var hdr http.Header
			if cfKey != "" {
				hdr = http.Header{"x-api-key": []string{cfKey}}
			}
			resp, err := px.Do(ctx, http.MethodGet, cfURL, nil, hdr, false)
			if err == nil && resp.StatusCode == http.StatusOK {
				var cfRes curseForgeFilesResponse
				if json.Unmarshal(resp.Body, &cfRes) == nil {
					for _, f := range cfRes.Data {
						isCur := strings.EqualFold(f.FileName, targetMod.File) ||
							strings.EqualFold(f.FileName+".disabled", targetMod.File)
						relType := "release"
						if f.ReleaseType == 2 {
							relType = "beta"
						} else if f.ReleaseType == 3 {
							relType = "alpha"
						}
						jars = append(jars, AvailableModJar{
							VersionID:     strconv.Itoa(f.ID),
							VersionName:   f.DisplayName,
							VersionNumber: f.DisplayName,
							Filename:      f.FileName,
							DownloadURL:   f.DownloadURL,
							SizeBytes:     f.FileLength,
							ReleaseType:   relType,
							GameVersions:  f.GameVersions,
							DatePublished: f.FileDate,
							IsCurrent:     isCur,
						})
					}
					if len(jars) > 0 {
						return jars, nil
					}
				}
			}
		}
	}

	return jars, nil
}
