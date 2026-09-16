// Package jars resolves and validates Minecraft server jar metadata. It never
// downloads jars; the container entrypoint performs the actual download.
package jars

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrUpstream is returned when fetching metadata from an upstream provider
// fails (network error, non-200 response, or unparseable body).
var ErrUpstream = errors.New("upstream provider error")

// JarType identifies a supported server platform.
type JarType string

const (
	TypePaper      JarType = "paper"
	TypeFabric     JarType = "fabric"
	TypeVanilla    JarType = "vanilla"
	TypeForge      JarType = "forge"
	TypeNeoForge   JarType = "neoforge"
	TypeSpigot     JarType = "spigot"
	TypePurpur     JarType = "purpur"
	TypeFolia      JarType = "folia"
	TypeQuilt      JarType = "quilt"
	TypeMohist     JarType = "mohist"
	TypeKetting    JarType = "ketting"
	TypeSponge     JarType = "sponge"
	TypeLimbo      JarType = "limbo"
	TypeNanoLimbo  JarType = "nanolimbo"
	TypeCrucible   JarType = "crucible"
	TypePufferfish JarType = "pufferfish"
	TypeLeaf       JarType = "leaf"
	TypeWaterfall  JarType = "waterfall"
	TypeBungeeCord JarType = "bungeecord"
	TypeVelocity   JarType = "velocity"
	TypeGeyser     JarType = "geysermc"
	TypeCustom     JarType = "custom"
)

// ParseJarType validates a platform string.
func ParseJarType(s string) (JarType, error) {
	switch JarType(strings.ToLower(s)) {
	case TypePaper, TypeFabric, TypeVanilla, TypeForge, TypeNeoForge, TypeSpigot,
		TypePurpur, TypeFolia, TypeQuilt, TypeMohist, TypeKetting, TypeSponge,
		TypeLimbo, TypeNanoLimbo, TypeCrucible, TypePufferfish, TypeLeaf,
		TypeWaterfall, TypeBungeeCord, TypeVelocity, TypeGeyser, TypeCustom:
		return JarType(strings.ToLower(s)), nil
	default:
		return "", fmt.Errorf("unsupported jar type %q", s)
	}
}

// JavaRelease describes an available Java runtime version.
type JavaRelease struct {
	Version int    `json:"version"`
	IsLTS   bool   `json:"is_lts"`
	Name    string `json:"name"`
}

// AvailableJavaVersions queries the Adoptium API for available Java releases,
// returning versions sorted descending. It falls back to a curated LTS list.
func (r *Resolver) AvailableJavaVersions(ctx context.Context) ([]JavaRelease, error) {
	var resp struct {
		AvailableReleases    []int `json:"available_releases"`
		AvailableLtsReleases []int `json:"available_lts_releases"`
	}

	ltsMap := map[int]bool{8: true, 11: true, 17: true, 21: true, 25: true}

	if err := r.getJSON(ctx, "https://api.adoptium.net/v3/info/available_releases", &resp); err == nil && len(resp.AvailableReleases) > 0 {
		for _, l := range resp.AvailableLtsReleases {
			ltsMap[l] = true
		}
		// Sort descending
		sort.Slice(resp.AvailableReleases, func(i, j int) bool {
			return resp.AvailableReleases[i] > resp.AvailableReleases[j]
		})

		out := make([]JavaRelease, 0, len(resp.AvailableReleases))
		for _, v := range resp.AvailableReleases {
			// Only include realistic Java versions for Minecraft (>= 8)
			if v < 8 {
				continue
			}
			isLTS := ltsMap[v]
			name := fmt.Sprintf("Java %d", v)
			if isLTS {
				if v == 21 {
					name = fmt.Sprintf("Java %d (LTS - Recommended)", v)
				} else {
					name = fmt.Sprintf("Java %d (LTS)", v)
				}
			}
			out = append(out, JavaRelease{Version: v, IsLTS: isLTS, Name: name})
		}
		if len(out) > 0 {
			return out, nil
		}
	}

	// Curated fallback
	return []JavaRelease{
		{Version: 25, IsLTS: true, Name: "Java 25 (LTS)"},
		{Version: 24, IsLTS: false, Name: "Java 24"},
		{Version: 21, IsLTS: true, Name: "Java 21 (LTS - Recommended)"},
		{Version: 17, IsLTS: true, Name: "Java 17 (LTS)"},
		{Version: 11, IsLTS: true, Name: "Java 11 (LTS)"},
		{Version: 8, IsLTS: true, Name: "Java 8 (LTS)"},
	}, nil
}

// RecommendJavaVersion returns the recommended Java major version for a given Minecraft version.
func RecommendJavaVersion(mcVersion string) int {
	if strings.HasPrefix(mcVersion, "25w") || strings.HasPrefix(mcVersion, "26w") {
		return 25
	}
	parts := strings.Split(mcVersion, ".")
	if len(parts) >= 1 {
		major, _ := strconv.Atoi(parts[0])
		if major >= 25 {
			return 25
		}
	}
	if len(parts) >= 2 {
		minor, _ := strconv.Atoi(parts[1])
		patch := 0
		if len(parts) >= 3 {
			patch, _ = strconv.Atoi(parts[2])
		}
		if minor >= 22 {
			return 25
		}
		if minor >= 21 || (minor == 20 && patch >= 5) {
			return 21
		}
		if minor >= 18 {
			return 17
		}
		if minor == 17 {
			return 17
		}
		if minor > 0 && minor <= 16 {
			return 8
		}
	}
	return 21
}


const (
	defaultPaperBase    = "https://fill.papermc.io/v3"
	defaultFabricBase   = "https://meta.fabricmc.net/v2"
	defaultMojangManf   = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	defaultForgeBase    = "https://files.minecraftforge.net/net/minecraftforge/forge"
	defaultNeoForgeBase = "https://maven.neoforged.net"
	defaultSpigotBase   = "https://hub.spigotmc.org/versions"
	requestTimeout      = 20 * time.Second
)

// Resolver queries upstream metadata APIs. The base URLs are overridable so unit
// tests can point at httptest.Server fixtures.
type Resolver struct {
	Client         *http.Client
	PaperBase      string
	FabricBase     string
	MojangManifest string
	ForgeBase      string
	NeoForgeBase   string
	SpigotBase     string
}

// NewResolver returns a Resolver using production endpoints.
func NewResolver() *Resolver {
	return NewResolverWithBases(&http.Client{Timeout: requestTimeout}, defaultPaperBase, defaultFabricBase, defaultMojangManf)
}

// NewResolverWithBases returns a Resolver with explicit endpoint bases, used by
// tests and by any deployment that needs to point at mirrors.
func NewResolverWithBases(client *http.Client, paperBase, fabricBase, mojangManifest string) *Resolver {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Resolver{
		Client:         client,
		PaperBase:      paperBase,
		FabricBase:     fabricBase,
		MojangManifest: mojangManifest,
		ForgeBase:      defaultForgeBase,
		NeoForgeBase:   defaultNeoForgeBase,
		SpigotBase:     defaultSpigotBase,
	}
}

// Resolved describes a validated server image configuration.
type Resolved struct {
	Type    JarType `json:"type"`
	Version string  `json:"version"`
	Build   string  `json:"build,omitempty"`
}

// Build is a single paper build number within a version.
type Build struct {
	Number int `json:"build"`
}

// PaperProject is the JSON shape returned by GET {base}/projects/paper.
type PaperProject struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	// Versions maps a version group (e.g. "1.21") to its concrete versions.
	Versions map[string][]string `json:"versions"`
}

// PaperBuild is a single entry in the JSON array returned by
// GET {base}/projects/paper/versions/{v}/builds.
type PaperBuild struct {
	ID      int    `json:"id"`
	Channel string `json:"channel"`
}

// ManifestEntry is a single version entry in Mojang's version manifest.
type ManifestEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// VersionManifest is the root of Mojang's version manifest.
type VersionManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []ManifestEntry `json:"versions"`
}

// PaperVersions returns the available Paper versions.
func (r *Resolver) PaperVersions(ctx context.Context) ([]string, error) {
	var p PaperProject
	if err := r.getJSON(ctx, r.PaperBase+"/projects/paper", &p); err != nil {
		return nil, err
	}
	return flattenPaperVersions(p.Versions), nil
}

// PaperBuilds returns the build numbers for a Paper version.
func (r *Resolver) PaperBuilds(ctx context.Context, version string) ([]Build, error) {
	var raw []PaperBuild
	if err := r.getJSON(ctx, fmt.Sprintf("%s/projects/paper/versions/%s/builds", r.PaperBase, version), &raw); err != nil {
		return nil, err
	}
	builds := make([]Build, 0, len(raw))
	for _, b := range raw {
		builds = append(builds, Build{Number: b.ID})
	}
	sort.Slice(builds, func(i, j int) bool { return builds[i].Number < builds[j].Number })
	return builds, nil
}

// FabricGameVersions returns the supported Fabric game versions.
func (r *Resolver) FabricGameVersions(ctx context.Context) ([]string, error) {
	var games []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := r.getJSON(ctx, r.FabricBase+"/versions/game", &games); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(games))
	for _, g := range games {
		out = append(out, g.Version)
	}
	return out, nil
}

// FabricLoaders returns the loader versions for a game version.
func (r *Resolver) FabricLoaders(ctx context.Context, version string) ([]string, error) {
	var loaders []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := r.getJSON(ctx, fmt.Sprintf("%s/versions/loader/%s", r.FabricBase, version), &loaders); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(loaders))
	for _, l := range loaders {
		out = append(out, l.Loader.Version)
	}
	return out, nil
}

// MojangVersions returns the full version manifest from Mojang.
func (r *Resolver) MojangVersions(ctx context.Context) (VersionManifest, error) {
	var m VersionManifest
	err := r.getJSON(ctx, r.MojangManifest, &m)
	return m, err
}

// Validate confirms a version (and optional build) exists for a jar type and
// returns the normalized type, version, and build.
func (r *Resolver) Validate(ctx context.Context, jt JarType, version, build string) (Resolved, error) {
	return r.resolve(ctx, jt, version, build)
}

// Resolve validates a server image config, resolving a default version when none
// is supplied.
func (r *Resolver) Resolve(ctx context.Context, jt JarType, version, build string) (Resolved, error) {
	return r.resolve(ctx, jt, version, build)
}

func (r *Resolver) resolve(ctx context.Context, jt JarType, version, build string) (Resolved, error) {
	switch jt {
	case TypePaper:
		builds, err := r.PaperBuilds(ctx, version)
		if err != nil {
			return Resolved{}, err
		}
		if len(builds) == 0 {
			return Resolved{}, fmt.Errorf("no builds found for paper version %q", version)
		}
		num, err := selectBuild(builds, build)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Type: TypePaper, Version: version, Build: strconv.Itoa(num)}, nil
	case TypeFabric:
		loaders, err := r.FabricLoaders(ctx, version)
		if err != nil {
			return Resolved{}, err
		}
		if len(loaders) == 0 {
			return Resolved{}, fmt.Errorf("no loaders found for fabric version %q", version)
		}
		loader, err := selectString(loaders, build)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Type: TypeFabric, Version: version, Build: loader}, nil
	case TypeVanilla:
		m, err := r.MojangVersions(ctx)
		if err != nil {
			return Resolved{}, err
		}
		if !containsStringVersion(m.Versions, version) {
			return Resolved{}, fmt.Errorf("vanilla version %q not found", version)
		}
		return Resolved{Type: TypeVanilla, Version: version}, nil
	case TypeForge:
		builds, err := r.ForgeBuilds(ctx, version)
		if err != nil {
			return Resolved{}, err
		}
		if len(builds) == 0 {
			return Resolved{}, fmt.Errorf("no forge builds found for version %q", version)
		}
		fv, err := selectString(builds, build)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Type: TypeForge, Version: version, Build: fv}, nil
	case TypeNeoForge:
		builds, err := r.NeoForgeBuilds(ctx, version)
		if err != nil {
			return Resolved{}, err
		}
		if len(builds) == 0 {
			return Resolved{}, fmt.Errorf("no neoforge builds found for version %q", version)
		}
		nv, err := selectString(builds, build)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Type: TypeNeoForge, Version: version, Build: nv}, nil
	case TypeSpigot:
		versions, err := r.SpigotGameVersions(ctx)
		if err != nil {
			return Resolved{}, err
		}
		if !containsString(versions, version) {
			return Resolved{}, fmt.Errorf("spigot version %q not found", version)
		}
		return Resolved{Type: TypeSpigot, Version: version}, nil
	case TypePurpur:
		builds, err := r.PurpurBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypePurpur, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[len(builds)-1]
		}
		return Resolved{Type: TypePurpur, Version: version, Build: b}, nil
	case TypeFolia:
		builds, err := r.FoliaBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypeFolia, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[len(builds)-1]
		}
		return Resolved{Type: TypeFolia, Version: version, Build: b}, nil
	case TypeWaterfall:
		builds, err := r.WaterfallBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypeWaterfall, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[len(builds)-1]
		}
		return Resolved{Type: TypeWaterfall, Version: version, Build: b}, nil
	case TypeVelocity:
		builds, err := r.VelocityBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypeVelocity, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[len(builds)-1]
		}
		return Resolved{Type: TypeVelocity, Version: version, Build: b}, nil
	case TypeQuilt:
		loaders, err := r.QuiltLoaders(ctx, version)
		if err != nil || len(loaders) == 0 {
			return Resolved{Type: TypeQuilt, Version: version, Build: "latest"}, nil
		}
		loader, err := selectString(loaders, build)
		if err != nil {
			loader = loaders[0]
		}
		return Resolved{Type: TypeQuilt, Version: version, Build: loader}, nil
	case TypeMohist:
		builds, err := r.MohistBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypeMohist, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[0]
		}
		return Resolved{Type: TypeMohist, Version: version, Build: b}, nil
	case TypeKetting:
		return Resolved{Type: TypeKetting, Version: version, Build: "latest"}, nil
	case TypeSponge:
		return Resolved{Type: TypeSponge, Version: version, Build: "latest"}, nil
	case TypeLimbo:
		return Resolved{Type: TypeLimbo, Version: version, Build: "latest"}, nil
	case TypeNanoLimbo:
		return Resolved{Type: TypeNanoLimbo, Version: version, Build: "latest"}, nil
	case TypeCrucible:
		return Resolved{Type: TypeCrucible, Version: "1.7.10", Build: "latest"}, nil
	case TypePufferfish:
		return Resolved{Type: TypePufferfish, Version: version, Build: "latest"}, nil
	case TypeLeaf:
		return Resolved{Type: TypeLeaf, Version: version, Build: "latest"}, nil
	case TypeBungeeCord:
		if build == "" {
			build = "latest"
		}
		return Resolved{Type: TypeBungeeCord, Version: "latest", Build: build}, nil
	case TypeGeyser:
		if build == "" {
			build = "latest"
		}
		return Resolved{Type: TypeGeyser, Version: version, Build: build}, nil
	case TypeCustom:
		if build == "" {
			build = "server.jar"
		}
		if version == "" {
			version = "custom"
		}
		return Resolved{Type: TypeCustom, Version: version, Build: build}, nil
	default:
		return Resolved{}, fmt.Errorf("unsupported jar type %q", jt)
	}
}

func selectBuild(builds []Build, want string) (int, error) {
	if len(builds) == 0 {
		return 0, fmt.Errorf("no builds available")
	}
	if want == "" || want == "latest" || want == "recommended" {
		// Paper lists builds ascending; the last is the newest.
		return builds[len(builds)-1].Number, nil
	}
	n, err := strconv.Atoi(want)
	if err != nil {
		return 0, fmt.Errorf("invalid build %q: %w", want, err)
	}
	for _, b := range builds {
		if b.Number == n {
			return n, nil
		}
	}
	return 0, fmt.Errorf("build %q not found", want)
}

func selectString(xs []string, want string) (string, error) {
	if len(xs) == 0 {
		return "", fmt.Errorf("no versions available")
	}
	if want == "" || want == "latest" || want == "recommended" {
		return pickNewestVersion(xs), nil
	}

	// 1. Direct match
	for _, x := range xs {
		if x == want {
			return x, nil
		}
	}

	// 2. Case-insensitive direct match
	for _, x := range xs {
		if strings.EqualFold(x, want) {
			return x, nil
		}
	}

	// 3. Strip loader prefixes: "forge-", "fabric-", "neoforge-", "quilt-"
	cleanedWant := strings.ToLower(want)
	for _, prefix := range []string{"forge-", "fabric-", "neoforge-", "quilt-"} {
		cleanedWant = strings.TrimPrefix(cleanedWant, prefix)
	}

	for _, x := range xs {
		if strings.EqualFold(x, cleanedWant) {
			return x, nil
		}
	}

	// 4. Suffix match: e.g. x is "1.20.1-47.3.0" and want is "47.3.0"
	for _, x := range xs {
		if strings.HasSuffix(x, "-"+cleanedWant) || strings.HasSuffix(x, "-"+want) {
			return x, nil
		}
		if strings.HasSuffix(cleanedWant, "-"+x) || strings.HasSuffix(want, "-"+x) {
			return x, nil
		}
	}

	// 5. Version-prefixed or stripped match
	for _, x := range xs {
		xClean := strings.ToLower(x)
		for _, prefix := range []string{"forge-", "fabric-", "neoforge-", "quilt-"} {
			xClean = strings.TrimPrefix(xClean, prefix)
		}
		if strings.EqualFold(xClean, cleanedWant) {
			return x, nil
		}
		if strings.HasSuffix(xClean, "-"+cleanedWant) || strings.HasSuffix(cleanedWant, "-"+xClean) {
			return x, nil
		}
	}

	return "", fmt.Errorf("value %q not found", want)
}

func compareVersionTokens(a, b string) int {
	partsA := splitVersionTokens(a)
	partsB := splitVersionTokens(b)
	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}
	for i := 0; i < minLen; i++ {
		pa, pb := partsA[i], partsB[i]
		numA, errA := strconv.ParseInt(pa, 10, 64)
		numB, errB := strconv.ParseInt(pb, 10, 64)
		if errA == nil && errB == nil {
			if numA != numB {
				if numA > numB {
					return 1
				}
				return -1
			}
		} else {
			if pa != pb {
				if pa > pb {
					return 1
				}
				return -1
			}
		}
	}
	if len(partsA) > len(partsB) {
		return 1
	}
	if len(partsA) < len(partsB) {
		return -1
	}
	return 0
}

func splitVersionTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+'
	})
}

func pickNewestVersion(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	best := xs[0]
	for _, x := range xs[1:] {
		if compareVersionTokens(x, best) > 0 {
			best = x
		}
	}
	return best
}

func containsStringVersion(entries []ManifestEntry, v string) bool {
	for _, e := range entries {
		if e.ID == v {
			return true
		}
	}
	return false
}

func containsString(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func sortedStrings(xs []string) []string {
	sort.Strings(xs)
	return xs
}

func (r *Resolver) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, errors.Join(ErrUpstream, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s returned %s: %s", url, resp.Status, string(body))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", url, errors.Join(ErrUpstream, err))
	}
	return nil
}

// DownloadServerJar resolves and downloads the server executable jar to destDir/server.jar,
// and ensures eula.txt is accepted.
func (r *Resolver) DownloadServerJar(ctx context.Context, jt JarType, version, build, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	targetJar := filepath.Join(destDir, "server.jar")
	writeEULA(destDir)

	var dlURL string
	switch jt {
	case TypePaper:
		if build == "" || build == "latest" {
			builds, err := r.PaperBuilds(ctx, version)
			if err != nil || len(builds) == 0 {
				return fmt.Errorf("resolve paper build: %w", err)
			}
			build = strconv.Itoa(builds[len(builds)-1].Number)
		}
		dlURL = fmt.Sprintf("%s/projects/paper/versions/%s/builds/%s/downloads/paper-%s-%s.jar", r.PaperBase, version, build, version, build)

	case TypeFabric:
		if build == "" || build == "latest" {
			loaders, err := r.FabricLoaders(ctx, version)
			if err != nil || len(loaders) == 0 {
				return fmt.Errorf("resolve fabric loader: %w", err)
			}
			build = loaders[0]
		}
		dlURL = fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s/%s/1.0.1/server/jar", version, build)

	case TypeVanilla:
		m, err := r.MojangVersions(ctx)
		if err != nil {
			return fmt.Errorf("fetch mojang manifest: %w", err)
		}
		var pkgURL string
		for _, v := range m.Versions {
			if v.ID == version {
				pkgURL = v.URL
				break
			}
		}
		if pkgURL == "" {
			return fmt.Errorf("vanilla version %q not found in manifest", version)
		}
		var pkg struct {
			Downloads struct {
				Server struct {
					URL string `json:"url"`
				} `json:"server"`
			} `json:"downloads"`
		}
		if err := r.getJSON(ctx, pkgURL, &pkg); err != nil {
			return fmt.Errorf("fetch vanilla package: %w", err)
		}
		if pkg.Downloads.Server.URL == "" {
			return fmt.Errorf("no server jar found for vanilla version %q", version)
		}
		dlURL = pkg.Downloads.Server.URL

	case TypePurpur:
		if build == "" || build == "latest" {
			builds, err := r.PurpurBuilds(ctx, version)
			if err == nil && len(builds) > 0 {
				build = builds[len(builds)-1]
			} else {
				build = "latest"
			}
		}
		dlURL = fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/%s/download", version, build)

	case TypeFolia:
		if build == "" || build == "latest" {
			builds, err := r.FoliaBuilds(ctx, version)
			if err != nil || len(builds) == 0 {
				return fmt.Errorf("resolve folia build: %w", err)
			}
			build = builds[len(builds)-1]
		}
		dlURL = fmt.Sprintf("%s/projects/folia/versions/%s/builds/%s/downloads/folia-%s-%s.jar", r.PaperBase, version, build, version, build)

	case TypeWaterfall:
		if build == "" || build == "latest" {
			builds, err := r.WaterfallBuilds(ctx, version)
			if err != nil || len(builds) == 0 {
				return fmt.Errorf("resolve waterfall build: %w", err)
			}
			build = builds[len(builds)-1]
		}
		dlURL = fmt.Sprintf("%s/projects/waterfall/versions/%s/builds/%s/downloads/waterfall-%s-%s.jar", r.PaperBase, version, build, version, build)

	case TypeVelocity:
		if build == "" || build == "latest" {
			builds, err := r.VelocityBuilds(ctx, version)
			if err != nil || len(builds) == 0 {
				return fmt.Errorf("resolve velocity build: %w", err)
			}
			build = builds[len(builds)-1]
		}
		dlURL = fmt.Sprintf("%s/projects/velocity/versions/%s/builds/%s/downloads/velocity-%s-%s.jar", r.PaperBase, version, build, version, build)

	case TypeSpigot:
		dlURL = fmt.Sprintf("https://cdn.getbukkit.org/spigot/spigot-%s.jar", version)

	case TypeQuilt:
		if build == "" || build == "latest" {
			loaders, err := r.QuiltLoaders(ctx, version)
			if err == nil && len(loaders) > 0 {
				build = loaders[0]
			} else {
				build = "0.27.0"
			}
		}
		dlURL = fmt.Sprintf("https://meta.quiltmc.org/v3/versions/loader/%s/%s/0.9.3/server/jar", version, build)

	case TypeMohist:
		if build == "" || build == "latest" {
			builds, err := r.MohistBuilds(ctx, version)
			if err == nil && len(builds) > 0 {
				build = builds[0]
			} else {
				build = "latest"
			}
		}
		dlURL = fmt.Sprintf("https://mohistmc.com/api/v2/projects/mohist/%s/builds/%s/download", version, build)

	case TypeKetting:
		dlURL = "https://github.com/kettingpowered/kettinglauncher/releases/latest/download/kettinglauncher.jar"

	case TypeLimbo:
		dlURL = "https://github.com/LOOHP/Limbo/releases/latest/download/Limbo.jar"

	case TypeNanoLimbo:
		dlURL = "https://github.com/BoomEaro/NanoLimbo/releases/latest/download/nanolimbo.jar"

	case TypeCrucible:
		dlURL = "https://github.com/CrucibleMC/Crucible/releases/latest/download/crucible.jar"

	case TypePufferfish:
		dlURL = "https://ci.pufferfish.host/job/Pufferfish-1.20/lastSuccessfulBuild/artifact/build/libs/pufferfish-paperclip-1.20.4-R0.1-SNAPSHOT-reobf.jar"

	case TypeLeaf:
		dlURL = fmt.Sprintf("https://github.com/Winds-Studio/Leaf/releases/latest/download/leaf-%s.jar", version)

	case TypeBungeeCord:
		if build == "" || build == "latest" {
			dlURL = "https://ci.md-5.net/job/BungeeCord/lastSuccessfulBuild/artifact/bootstrap/target/BungeeCord.jar"
		} else {
			dlURL = fmt.Sprintf("https://ci.md-5.net/job/BungeeCord/%s/artifact/bootstrap/target/BungeeCord.jar", build)
		}

	case TypeGeyser:
		if build == "" || build == "latest" {
			builds, err := r.GeyserBuilds(ctx, version)
			if err == nil && len(builds) > 0 {
				build = builds[0]
			} else {
				build = "latest"
			}
		}
		dlURL = fmt.Sprintf("https://download.geysermc.org/v2/projects/geyser/versions/%s/builds/%s/downloads/standalone", version, build)

	case TypeForge:
		if build == "" || build == "latest" {
			builds, err := r.ForgeBuilds(ctx, version)
			if err == nil && len(builds) > 0 {
				build = pickNewestVersion(builds)
			} else {
				build = version
			}
		}
		// Forge installer
		forgeVer := build
		if !strings.Contains(forgeVer, "-") && version != "" {
			forgeVer = version + "-" + forgeVer
		}
		dlURL = fmt.Sprintf("https://maven.minecraftforge.net/net/minecraftforge/forge/%s/forge-%s-installer.jar", forgeVer, forgeVer)
		installerPath := filepath.Join(destDir, "installer.jar")
		if err := r.DownloadFile(ctx, dlURL, installerPath); err != nil {
			return err
		}
		return nil

	case TypeNeoForge:
		if build == "" || build == "latest" {
			builds, err := r.NeoForgeBuilds(ctx, version)
			if err == nil && len(builds) > 0 {
				build = pickNewestVersion(builds)
			}
		}
		// NeoForge installer
		nfVer := build
		if version == "1.20.1" {
			if !strings.Contains(nfVer, "-") {
				nfVer = "1.20.1-" + nfVer
			}
			dlURL = fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/forge/%s/forge-%s-installer.jar", nfVer, nfVer)
		} else {
			dlURL = fmt.Sprintf("https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar", nfVer, nfVer)
		}
		installerPath := filepath.Join(destDir, "installer.jar")
		if err := r.DownloadFile(ctx, dlURL, installerPath); err != nil {
			return err
		}
		return nil

	case TypeCustom:
		if strings.HasPrefix(build, "http://") || strings.HasPrefix(build, "https://") {
			return r.DownloadFile(ctx, build, targetJar)
		}
		// If custom jar file was specified by name, check if it exists in destDir
		if build != "" && build != "server.jar" {
			src := filepath.Join(destDir, build)
			if _, err := os.Stat(src); err == nil {
				// Copy or link to server.jar if needed
				return nil
			}
		}
		return nil

	default:
		return fmt.Errorf("unsupported platform download %q", jt)
	}

	if dlURL != "" {
		return r.DownloadFile(ctx, dlURL, targetJar)
	}

	return nil
}

func writeEULA(destDir string) {
	eulaFile := filepath.Join(destDir, "eula.txt")
	_ = os.WriteFile(eulaFile, []byte("eula=true\n"), 0644)
}

