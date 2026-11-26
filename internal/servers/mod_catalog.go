package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy"
)

const (
	releaseChannel      = "release"
	unclassifiedChannel = "unclassified"
)

func modReleaseType(channel string) string {
	switch channel = strings.ToLower(strings.TrimSpace(channel)); channel {
	case "stable", "release":
		return releaseChannel
	case "":
		return unclassifiedChannel
	default:
		return channel
	}
}

func catalogReleaseType(channel string) string {
	if channel == "" {
		return releaseChannel
	}
	return modReleaseType(channel)
}

func curseForgeReleaseType(value int) string {
	switch value {
	case 0, 1:
		return releaseChannel
	case 2:
		return "beta"
	case 3:
		return "alpha"
	default:
		return unclassifiedChannel
	}
}

var curseForgeLoaderType = map[string]int{"forge": 1, "fabric": 4, "quilt": 5, "neoforge": 6}

var spigetVersionDownloadPath = regexp.MustCompile(`^/v2/resources/[0-9]+/versions/[0-9]+/download$`)

var namedModChannelPattern = regexp.MustCompile(`(?i)(^|[^a-z])(alpha|beta|release|stable)($|[^a-z])`)

func namedModChannel(name string) string {
	if parts := namedModChannelPattern.FindStringSubmatch(name); parts != nil {
		return modReleaseType(parts[2])
	}
	return unclassifiedChannel
}

func canSwitchModChannel(current, next string) bool {
	rank := map[string]int{"alpha": 1, "beta": 2, releaseChannel: 3}
	return rank[current] > 0 && rank[next] > rank[current]
}

type modCatalogBuild struct {
	AvailableModJar
	ProjectID string
	Changelog string
}

type modCatalog struct {
	Provider    string
	ProjectID   string
	ProjectSlug string
	Builds      []modCatalogBuild
}

func modrinthBuild(version modrinthVersion, file modrinthVersionFile, mod Mod) modCatalogBuild {
	number := version.VersionNumber
	if number == "" {
		number = version.Name
	}
	return modCatalogBuild{
		ProjectID: version.ProjectID,
		Changelog: version.Changelog,
		AvailableModJar: AvailableModJar{
			VersionID: version.ID, VersionName: version.Name, VersionNumber: number,
			Filename: file.Filename, DownloadURL: file.URL, Hashes: file.Hashes, SizeBytes: file.Size,
			ReleaseType: catalogReleaseType(version.VersionType), GameVersions: version.GameVersions,
			Loaders: version.Loaders, DatePublished: version.DatePublished,
			IsCurrent: matchesModFile(mod, file.Filename, file.Hashes),
		},
	}
}

func matchesModFile(mod Mod, filename string, hashes map[string]string) bool {
	if mod.SHA1 != "" && hashes["sha1"] != "" {
		return strings.EqualFold(mod.SHA1, hashes["sha1"])
	}
	return strings.EqualFold(filename, strings.TrimSuffix(mod.File, ".disabled"))
}

func installedModChannel(mod Mod, builds []modCatalogBuild) string {
	if mod.ReleaseType != "" {
		return mod.ReleaseType
	}
	for _, build := range builds {
		if build.IsCurrent {
			return build.ReleaseType
		}
	}
	if inferred := namedModChannel(mod.Version); inferred != unclassifiedChannel || mod.Provider == "spiget" {
		return inferred
	}
	return releaseChannel
}

func selectModBuild(builds []modCatalogBuild, channel string) (modCatalogBuild, bool) {
	// A multi-file release can list a different jar first. Keep the installed
	// file if it is already part of the newest matching version.
	for i, b := range builds {
		if b.ReleaseType != channel || b.DownloadURL == "" || b.Filename == "" {
			continue
		}
		for _, other := range builds[i:] {
			if other.VersionID != b.VersionID {
				break
			}
			if other.IsCurrent && other.ReleaseType == channel {
				return other, true
			}
		}
		return b, true
	}
	return modCatalogBuild{}, false
}

func modrinthProject(mod Mod) string {
	for _, id := range []string{mod.ProjectID, mod.ProjectSlug, mod.ModID} {
		if id != "" {
			return id
		}
	}
	return ""
}

func modrinthInstalledVersions(ctx context.Context, px *proxy.Service, mods []Mod, force bool) map[string]modrinthVersion {
	var hashes []string
	for _, mod := range mods {
		if mod.SHA1 != "" && (mod.Provider == "" || mod.Provider == "modrinth") &&
			(mod.ReleaseType == "" || modrinthProject(mod) == "") {
			hashes = append(hashes, mod.SHA1)
		}
	}
	if len(hashes) == 0 {
		return nil
	}
	slices.Sort(hashes)
	body, _ := json.Marshal(map[string]any{"hashes": slices.Compact(hashes), "algorithm": "sha1"})
	resp, err := px.Do(ctx, http.MethodPost, "https://api.modrinth.com/v2/version_files", body, nil, force)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	var versions map[string]modrinthVersion
	if json.Unmarshal(resp.Body, &versions) != nil {
		return nil
	}
	return versions
}

func withInstalledModrinthVersion(mod Mod, version modrinthVersion) Mod {
	for _, file := range version.Files {
		if !strings.EqualFold(file.Hashes["sha1"], mod.SHA1) {
			continue
		}
		if mod.ReleaseType == "" {
			mod.ReleaseType = catalogReleaseType(version.VersionType)
		}
		if mod.ProjectID == "" {
			mod.ProjectID = version.ProjectID
		}
		mod.Provider = "modrinth"
		break
	}
	return mod
}

func modrinthUpdatesForChannels(ctx context.Context, px *proxy.Service, mods []Mod, loaders []string, gameVersion string, force bool) (map[string]modCatalogBuild, []string) {
	groups := make(map[string][]string)
	for _, mod := range mods {
		if mod.SHA1 != "" && (mod.Provider == "modrinth" || mod.Provider == "") {
			channel := installedModChannel(mod, nil)
			if channel == releaseChannel || channel == "beta" || channel == "alpha" {
				groups[channel] = append(groups[channel], mod.SHA1)
			}
		}
	}
	results := make(map[string]modCatalogBuild)
	var problems []string
	for _, channel := range []string{releaseChannel, "beta", "alpha"} {
		hashes := groups[channel]
		if len(hashes) == 0 {
			continue
		}
		payload := map[string]any{"hashes": hashes, "algorithm": "sha1", "version_types": []string{channel}}
		if len(loaders) > 0 {
			payload["loaders"] = loaders
		}
		if gameVersion != "" {
			payload["game_versions"] = []string{gameVersion}
		}
		versions, err := fetchModrinthHashUpdates(ctx, px, payload, force)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for _, mod := range mods {
			if installedModChannel(mod, nil) != channel || (mod.Provider != "" && mod.Provider != "modrinth") {
				continue
			}
			v := versions[mod.SHA1]
			if len(v.Files) == 0 || catalogReleaseType(v.VersionType) != channel {
				continue
			}
			file := v.Files[0]
			for _, candidate := range v.Files {
				if matchesModFile(mod, candidate.Filename, candidate.Hashes) {
					file = candidate
					break
				}
				if candidate.Primary {
					file = candidate
				}
			}
			results[mod.File] = modrinthBuild(v, file, mod)
		}
	}
	return results, problems
}

func fetchModCatalog(ctx context.Context, px *proxy.Service, mod Mod, platform, gameVersion, cfKey string, force, listAll bool) (modCatalog, error) {
	catalog := modCatalog{Provider: mod.Provider, ProjectID: mod.ProjectID, ProjectSlug: mod.ProjectSlug}
	get := func(target string, headers http.Header, out any) error {
		resp, err := px.Do(ctx, http.MethodGet, target, nil, headers, force)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK || resp.CacheStatus == "STALE" {
			return fmt.Errorf("%s returned an unavailable response (%d)", target, resp.StatusCode)
		}
		return json.Unmarshal(resp.Body, out)
	}

	hasSelection := func() bool {
		if listAll {
			return false
		}
		if mod.ReleaseType == "" && !slices.ContainsFunc(catalog.Builds, func(build modCatalogBuild) bool { return build.IsCurrent }) {
			return false
		}
		_, ok := selectModBuild(catalog.Builds, installedModChannel(mod, catalog.Builds))
		return ok
	}

	switch mod.Provider {
	case "", "modrinth":
		slug := modrinthProject(mod)
		if slug == "" {
			break
		}
		catalog.Provider = "modrinth"
		query := url.Values{}
		if loaders := serverLoadersForType(platform); len(loaders) > 0 {
			encoded, _ := json.Marshal(loaders)
			query.Set("loaders", string(encoded))
		}
		if gameVersion != "" {
			encoded, _ := json.Marshal([]string{gameVersion})
			query.Set("game_versions", string(encoded))
		}
		target := "https://api.modrinth.com/v2/project/" + url.PathEscape(slug) + "/version"
		if q := query.Encode(); q != "" {
			target += "?" + q
		}
		var versions []modrinthVersion
		if err := get(target, nil, &versions); err != nil {
			return catalog, err
		}
		for _, v := range versions {
			for _, f := range v.Files {
				if f.Primary {
					catalog.Builds = append(catalog.Builds, modrinthBuild(v, f, mod))
				}
			}
			for _, f := range v.Files {
				if !f.Primary {
					catalog.Builds = append(catalog.Builds, modrinthBuild(v, f, mod))
				}
			}
		}
	case "hangar":
		if catalog.ProjectSlug == "" {
			catalog.ProjectSlug = mod.ProjectID
		}
		if catalog.ProjectSlug == "" {
			break
		}
		hangarPlatform := "PAPER"
		if platform == "waterfall" || platform == "bungeecord" {
			hangarPlatform = "WATERFALL"
		}
		for offset := 0; ; {
			target := fmt.Sprintf("https://hangar.papermc.io/api/v1/projects/%s/versions?limit=25", url.PathEscape(catalog.ProjectSlug))
			if offset > 0 {
				target += fmt.Sprintf("&offset=%d", offset)
			}
			var page struct {
				Result     []hangarVersion
				Pagination struct{ Count int }
			}
			if err := get(target, nil, &page); err != nil {
				return catalog, err
			}
			for _, v := range page.Result {
				dl := v.Downloads[hangarPlatform]
				supported := v.PlatformDependencies[hangarPlatform]
				if dl.FileInfo == nil || (gameVersion != "" && len(supported) > 0 && !slices.Contains(supported, gameVersion)) {
					continue
				}
				catalog.Builds = append(catalog.Builds, modCatalogBuild{AvailableModJar: AvailableModJar{
					VersionID: v.Name, VersionName: v.Name, VersionNumber: v.Name,
					Filename: dl.FileInfo.Name, DownloadURL: dl.DownloadURL, SizeBytes: dl.FileInfo.SizeBytes,
					Hashes: map[string]string{"sha256": dl.FileInfo.SHA256}, ReleaseType: catalogReleaseType(v.Channel.Name),
					GameVersions: supported, DatePublished: v.CreatedAt, IsCurrent: matchesModFile(mod, dl.FileInfo.Name, nil),
				}})
			}
			offset += len(page.Result)
			if len(page.Result) == 0 || offset >= page.Pagination.Count || hasSelection() {
				break
			}
		}
	case "curseforge":
		id, err := strconv.Atoi(mod.ProjectID)
		if err != nil {
			break
		}
		if cfKey == "" {
			return catalog, fmt.Errorf("CurseForge checks require a shared API key in App Settings or CURSEFORGE_API_KEY on the panel")
		}
		for offset := 0; ; {
			target := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files?pageSize=50", id)
			if gameVersion != "" {
				target += "&gameVersion=" + url.QueryEscape(gameVersion)
			}
			if loader, ok := curseForgeLoaderType[platform]; ok {
				target += fmt.Sprintf("&modLoaderType=%d", loader)
			}
			if offset > 0 {
				target += fmt.Sprintf("&index=%d", offset)
			}
			var page curseForgeFilesResponse
			if err := get(target, http.Header{"X-Api-Key": {cfKey}}, &page); err != nil {
				return catalog, err
			}
			for _, f := range page.Data {
				name := f.DisplayName
				if name == "" {
					name = f.FileName
				}
				download := f.DownloadURL
				if download == "" && f.ID > 0 && f.FileName != "" {
					download = fmt.Sprintf("https://edge.forgecdn.net/files/%d/%d/%s", f.ID/1000, f.ID%1000, url.PathEscape(f.FileName))
				}
				hashes := curseForgeHashes(f.Hashes)
				catalog.Builds = append(catalog.Builds, modCatalogBuild{AvailableModJar: AvailableModJar{
					VersionID: strconv.Itoa(f.ID), VersionName: name, VersionNumber: name, Filename: f.FileName,
					DownloadURL: download, Hashes: hashes, SizeBytes: f.FileLength, ReleaseType: curseForgeReleaseType(f.ReleaseType),
					GameVersions: f.GameVersions, DatePublished: f.FileDate, IsCurrent: matchesModFile(mod, f.FileName, hashes),
				}})
			}
			offset += len(page.Data)
			if len(page.Data) == 0 || offset >= page.Pagination.TotalCount || hasSelection() {
				break
			}
		}
	case "spiget":
		id, err := strconv.Atoi(mod.ProjectID)
		if err != nil {
			break
		}
		for page := 1; ; page++ {
			target := fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions?size=100&sort=-releaseDate", id)
			if page > 1 {
				target += fmt.Sprintf("&page=%d", page)
			}
			var versions []spigetVersion
			if err := get(target, nil, &versions); err != nil {
				return catalog, err
			}
			for _, v := range versions {
				var date string
				if v.ReleaseDate > 0 {
					date = time.Unix(v.ReleaseDate, 0).UTC().Format(time.RFC3339)
				}
				isCurrent := mod.CatalogVersionID != "" && mod.CatalogVersionID == strconv.Itoa(v.ID)
				if mod.CatalogVersionID == "" && mod.Version != "" {
					isCurrent = cleanVersionString(mod.Version) == cleanVersionString(v.Name)
				}
				catalog.Builds = append(catalog.Builds, modCatalogBuild{AvailableModJar: AvailableModJar{
					VersionID: strconv.Itoa(v.ID), VersionName: v.Name, VersionNumber: v.Name,
					Filename:    strings.TrimSuffix(mod.File, ".disabled"),
					DownloadURL: fmt.Sprintf("https://api.spiget.org/v2/resources/%d/versions/%d/download", id, v.ID),
					ReleaseType: namedModChannel(v.Name), DatePublished: date, IsCurrent: isCurrent,
				}})
			}
			if len(versions) < 100 || hasSelection() {
				break
			}
		}
	}
	return catalog, nil
}
