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
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

var mcVersionRegex = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// RecommendJavaVersion returns the recommended Java major version for a given Minecraft version.
func RecommendJavaVersion(mcVersion string) int {
	clean := strings.TrimSpace(strings.TrimPrefix(mcVersion, "v"))
	if strings.HasPrefix(clean, "25w") || strings.HasPrefix(clean, "26w") {
		return 25
	}
	m := mcVersionRegex.FindStringSubmatch(clean)
	if len(m) >= 3 {
		major, _ := strconv.Atoi(m[1])
		minor, _ := strconv.Atoi(m[2])
		patch := 0
		if len(m) >= 4 && m[3] != "" {
			patch, _ = strconv.Atoi(m[3])
		}
		if major >= 25 {
			return 25
		}
		if major == 1 {
			if minor >= 22 {
				return 25
			}
			if minor >= 21 || (minor == 20 && patch >= 5) {
				return 21
			}
			if minor >= 17 {
				return 17
			}
			if minor > 0 && minor <= 16 {
				return 8
			}
		}
	}
	return 21
}

// RecommendJavaVersionForType returns the recommended Java version considering both server type and version.
func RecommendJavaVersionForType(serverType, version string) int {
	switch strings.ToLower(serverType) {
	case "geysermc", "velocity", "waterfall", "bungeecord", "limbo", "nanolimbo":
		return 21
	default:
		return RecommendJavaVersion(version)
	}
}


const (
	defaultPaperBase      = "https://fill.papermc.io/v3"
	defaultFabricBase     = "https://meta.fabricmc.net/v2"
	defaultMojangManf     = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	defaultForgeBase      = "https://files.minecraftforge.net/net/minecraftforge/forge"
	defaultNeoForgeBase   = "https://maven.neoforged.net"
	defaultSpigotBase     = "https://hub.spigotmc.org/versions"
	defaultBuildToolsURL  = "https://hub.spigotmc.org/jenkins/job/BuildTools/lastSuccessfulBuild/artifact/target/BuildTools.jar"
	requestTimeout        = 20 * time.Second
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
	BuildToolsURL  string
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
		BuildToolsURL:  defaultBuildToolsURL,
	}
}

func (r *Resolver) buildToolsURL() string {
	if r.BuildToolsURL != "" {
		return r.BuildToolsURL
	}
	return defaultBuildToolsURL
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
		targetVer := version
		if targetVer == "" || targetVer == "latest" {
			vers, err := r.PurpurVersions(ctx)
			if err == nil && len(vers) > 0 {
				targetVer = vers[len(vers)-1]
			} else {
				targetVer = "1.21.4"
			}
		}
		builds, err := r.PurpurBuilds(ctx, targetVer)
		if err != nil && strings.HasSuffix(targetVer, ".0") {
			trimmed := strings.TrimSuffix(targetVer, ".0")
			if b2, err2 := r.PurpurBuilds(ctx, trimmed); err2 == nil {
				targetVer = trimmed
				builds = b2
				err = nil
			}
		}
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypePurpur, Version: targetVer, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[len(builds)-1]
		}
		return Resolved{Type: TypePurpur, Version: targetVer, Build: b}, nil
	case TypeFolia:
		builds, err := r.FoliaBuilds(ctx, version)
		if err != nil || len(builds) == 0 {
			return Resolved{Type: TypeFolia, Version: version, Build: "latest"}, nil
		}
		b, err := selectString(builds, build)
		if err != nil {
			b = builds[0]
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
			if err != nil {
				return Resolved{}, fmt.Errorf("no mohist builds found for version %q: %w", version, err)
			}
			return Resolved{}, fmt.Errorf("no mohist builds found for version %q", version)
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
	req.Header.Set("User-Agent", "MCM-Panel/1.0 (Minecraft Container Manager; +https://github.com/mcm-panel/mcm)")

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

// CompareVersionTokens compares two version or build strings.
// Returns 1 if a > b, -1 if a < b, 0 if a == b.
func CompareVersionTokens(a, b string) int {
	if a == b {
		return 0
	}
	if a == "latest" {
		return 1
	}
	if b == "latest" {
		return -1
	}
	numA, errA := strconv.ParseInt(a, 10, 64)
	numB, errB := strconv.ParseInt(b, 10, 64)
	if errA == nil && errB == nil {
		if numA > numB {
			return 1
		}
		if numA < numB {
			return -1
		}
		return 0
	}

	splitA := strings.FieldsFunc(a, func(r rune) bool { return r == '.' || r == '-' })
	splitB := strings.FieldsFunc(b, func(r rune) bool { return r == '.' || r == '-' })

	maxLen := len(splitA)
	if len(splitB) > maxLen {
		maxLen = len(splitB)
	}

	for i := 0; i < maxLen; i++ {
		if i >= len(splitA) {
			return -1
		}
		if i >= len(splitB) {
			return 1
		}
		segA := splitA[i]
		segB := splitB[i]
		if segA == segB {
			continue
		}
		nA, eA := strconv.ParseInt(segA, 10, 64)
		nB, eB := strconv.ParseInt(segB, 10, 64)
		if eA == nil && eB == nil {
			if nA > nB {
				return 1
			}
			if nA < nB {
				return -1
			}
			continue
		}
		if segA > segB {
			return 1
		}
		return -1
	}
	return 0
}

func (r *Resolver) paperDownloadURL(ctx context.Context, project, version, build string) (string, error) {
	bEndpoint := build
	if bEndpoint == "" || bEndpoint == "latest" {
		bEndpoint = "latest"
	}
	metaURL := fmt.Sprintf("%s/projects/%s/versions/%s/builds/%s", r.PaperBase, project, version, bEndpoint)
	var meta struct {
		ID        int `json:"id"`
		Downloads map[string]struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"downloads"`
	}
	if err := r.getJSON(ctx, metaURL, &meta); err == nil && len(meta.Downloads) > 0 {
		if d, ok := meta.Downloads["server:default"]; ok && d.URL != "" {
			return d.URL, nil
		}
		if d, ok := meta.Downloads["application"]; ok && d.URL != "" {
			return d.URL, nil
		}
		for _, d := range meta.Downloads {
			if d.URL != "" {
				return d.URL, nil
			}
		}
	}

	// Fallback to legacy/direct path if getJSON fails or downloads map is empty
	b := build
	if b == "" || b == "latest" {
		b = "latest"
	}
	return fmt.Sprintf("%s/projects/%s/versions/%s/builds/%s/downloads/%s-%s-%s.jar", r.PaperBase, project, version, b, project, version, b), nil
}

func (r *Resolver) resolveGitHubReleaseAsset(ctx context.Context, repo, tag, prefix string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if tag != "" && tag != "latest" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	}
	var rel struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := r.getJSON(ctx, url, &rel); err == nil && len(rel.Assets) > 0 {
		for _, a := range rel.Assets {
			name := strings.ToLower(a.Name)
			if strings.HasSuffix(name, ".jar") && !strings.Contains(name, "sources") && !strings.Contains(name, "javadoc") {
				if prefix == "" || strings.HasPrefix(strings.ToLower(a.Name), strings.ToLower(prefix)) {
					return a.BrowserDownloadURL, nil
				}
			}
		}
		for _, a := range rel.Assets {
			name := strings.ToLower(a.Name)
			if strings.HasSuffix(name, ".jar") && !strings.Contains(name, "sources") && !strings.Contains(name, "javadoc") {
				return a.BrowserDownloadURL, nil
			}
		}
	}
	// Fallback to latest release if specific tag was not found
	if tag != "" && tag != "latest" {
		return r.resolveGitHubReleaseAsset(ctx, repo, "latest", prefix)
	}
	return "", fmt.Errorf("no jar asset found for %s", repo)
}

func (r *Resolver) resolveLimboJar(ctx context.Context, version string) (string, error) {
	// 1. Try Modrinth official LOOHP limbo-server distribution
	mrURL := "https://api.modrinth.com/v2/project/limbo-server/version"
	if version != "" && version != "latest" {
		mrURL = fmt.Sprintf("https://api.modrinth.com/v2/project/limbo-server/version?game_versions=%%5B%%22%s%%22%%5D", url.QueryEscape(version))
	}
	var mrData []struct {
		Files []struct {
			URL     string `json:"url"`
			Primary bool   `json:"primary"`
		} `json:"files"`
	}
	if err := r.getJSON(ctx, mrURL, &mrData); err == nil && len(mrData) > 0 {
		for _, v := range mrData {
			for _, f := range v.Files {
				if f.Primary && f.URL != "" {
					return f.URL, nil
				}
			}
			if len(v.Files) > 0 && v.Files[0].URL != "" {
				return v.Files[0].URL, nil
			}
		}
	}

	// 2. If filtered version query on Modrinth yielded no results, try Modrinth latest
	if version != "" && version != "latest" {
		if err := r.getJSON(ctx, "https://api.modrinth.com/v2/project/limbo-server/version", &mrData); err == nil && len(mrData) > 0 {
			if len(mrData[0].Files) > 0 && mrData[0].Files[0].URL != "" {
				return mrData[0].Files[0].URL, nil
			}
		}
	}

	// 3. Try Jenkins build artifacts
	jenkinsURL := "https://ci.loohpjames.com/job/Limbo/lastSuccessfulBuild/api/json"
	var jkData struct {
		Artifacts []struct {
			RelativePath string `json:"relativePath"`
		} `json:"artifacts"`
	}
	if err := r.getJSON(ctx, jenkinsURL, &jkData); err == nil && len(jkData.Artifacts) > 0 {
		for _, a := range jkData.Artifacts {
			if strings.HasSuffix(a.RelativePath, ".jar") {
				return "https://ci.loohpjames.com/job/Limbo/lastSuccessfulBuild/artifact/" + a.RelativePath, nil
			}
		}
	}

	// 4. Reliable pinned CDN fallback (LOOHP Limbo on Modrinth CDN)
	return "https://cdn.modrinth.com/data/gIDqs3gn/versions/t4FmKS67/Limbo-2026.0.3-ALPHA-26.3.jar", nil
}

func (r *Resolver) resolveSpongeTag(ctx context.Context, version string) string {
	url := fmt.Sprintf("https://dl-api.spongepowered.org/v2/groups/org.spongepowered/artifacts/spongevanilla/versions?tags=minecraft:%s&limit=1", version)
	var res struct {
		Artifacts map[string]any `json:"artifacts"`
	}
	if err := r.getJSON(ctx, url, &res); err == nil {
		for tag := range res.Artifacts {
			return tag
		}
	}
	return version
}

func (r *Resolver) resolveQuiltInstallerURL(ctx context.Context) string {
	defaultURL := "https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/0.15.1/quilt-installer-0.15.1.jar"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/maven-metadata.xml", nil)
	if err != nil {
		return defaultURL
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return defaultURL
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return defaultURL
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return defaultURL
	}
	re := regexp.MustCompile(`<release>([^<]+)</release>`)
	m := re.FindSubmatch(body)
	if len(m) >= 2 {
		rel := string(m[1])
		return fmt.Sprintf("https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/%s/quilt-installer-%s.jar", rel, rel)
	}
	return defaultURL
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
		var err error
		dlURL, err = r.paperDownloadURL(ctx, "paper", version, build)
		if err != nil {
			return fmt.Errorf("resolve paper download url: %w", err)
		}

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
		targetVer := version
		if targetVer == "" || targetVer == "latest" {
			vers, err := r.PurpurVersions(ctx)
			if err == nil && len(vers) > 0 {
				targetVer = vers[len(vers)-1]
			} else {
				targetVer = "1.21.4"
			}
		}
		builds, err := r.PurpurBuilds(ctx, targetVer)
		if err != nil && strings.HasSuffix(targetVer, ".0") {
			trimmed := strings.TrimSuffix(targetVer, ".0")
			if b2, err2 := r.PurpurBuilds(ctx, trimmed); err2 == nil {
				targetVer = trimmed
				builds = b2
				err = nil
			}
		}
		if err != nil {
			if vers, errVers := r.PurpurVersions(ctx); errVers == nil {
				for _, v := range vers {
					if strings.HasPrefix(v, targetVer) || strings.HasPrefix(targetVer, v) {
						targetVer = v
						builds, _ = r.PurpurBuilds(ctx, targetVer)
						break
					}
				}
			}
		}
		if build == "" || build == "latest" {
			if len(builds) > 0 {
				build = builds[len(builds)-1]
			} else {
				build = "latest"
			}
		}
		dlURL = fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/%s/download", targetVer, build)

	case TypeFolia:
		var err error
		dlURL, err = r.paperDownloadURL(ctx, "folia", version, build)
		if err != nil {
			return fmt.Errorf("resolve folia download url: %w", err)
		}

	case TypeWaterfall:
		var err error
		dlURL, err = r.paperDownloadURL(ctx, "waterfall", version, build)
		if err != nil {
			return fmt.Errorf("resolve waterfall download url: %w", err)
		}

	case TypeVelocity:
		var err error
		dlURL, err = r.paperDownloadURL(ctx, "velocity", version, build)
		if err != nil {
			return fmt.Errorf("resolve velocity download url: %w", err)
		}

	case TypeSpigot:
		if _, err := os.Stat(targetJar); err == nil {
			return nil
		}
		installerPath := filepath.Join(destDir, "installer.jar")
		if err := r.DownloadFile(ctx, r.buildToolsURL(), installerPath); err != nil {
			return fmt.Errorf("download spigot buildtools: %w", err)
		}
		return nil

	case TypeQuilt:
		installerURL := r.resolveQuiltInstallerURL(ctx)
		installerPath := filepath.Join(destDir, "installer.jar")
		if err := r.DownloadFile(ctx, installerURL, installerPath); err != nil {
			return fmt.Errorf("download quilt installer: %w", err)
		}
		return nil

	case TypeMohist:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if build == "" || build == "latest" {
			builds, err := r.MohistBuilds(ctx, version)
			if err != nil || len(builds) == 0 {
				if err != nil {
					return fmt.Errorf("no mohist builds available for version %q: %w", version, err)
				}
				return fmt.Errorf("no mohist builds available for version %q", version)
			}
			build = builds[0]
		}
		dlURL = fmt.Sprintf("https://api.mohistmc.com/project/mohist/%s/builds/%s/download", version, build)
		dlErr := r.DownloadFile(ctx, dlURL, targetJar)
		if dlErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("download mohist %s build %s: %w", version, build, ctx.Err())
		}
		dlURL = fmt.Sprintf("https://mohistmc.com/builds-raw/Mohist-%s/Mohist-%s-%s.jar", version, version, build)
		if fbErr := r.DownloadFile(ctx, dlURL, targetJar); fbErr != nil {
			return fmt.Errorf("download mohist %s build %s: %w (fallback failed: %v)", version, build, dlErr, fbErr)
		}
		return nil

	case TypeKetting:
		var err error
		dlURL, err = r.resolveGitHubReleaseAsset(ctx, "kettingpowered/kettinglauncher", "latest", "kettinglauncher-")
		if err != nil {
			dlURL = "https://github.com/kettingpowered/kettinglauncher/releases/latest/download/kettinglauncher.jar"
		}

	case TypeLimbo:
		var err error
		dlURL, err = r.resolveLimboJar(ctx, version)
		if err != nil {
			dlURL = "https://cdn.modrinth.com/data/gIDqs3gn/versions/t4FmKS67/Limbo-2026.0.3-ALPHA-26.3.jar"
		}

	case TypeNanoLimbo:
		var err error
		dlURL, err = r.resolveGitHubReleaseAsset(ctx, "Nan1t/NanoLimbo", "latest", "NanoLimbo")
		if err != nil {
			dlURL, err = r.resolveGitHubReleaseAsset(ctx, "BoomEaro/NanoLimbo", "latest", "nanolimbo")
		}
		if err != nil {
			dlURL = "https://github.com/Nan1t/NanoLimbo/releases/latest/download/NanoLimbo.jar"
		}

	case TypeCrucible:
		var err error
		dlURL, err = r.resolveGitHubReleaseAsset(ctx, "CrucibleMC/Crucible", "latest", "Crucible-")
		if err != nil {
			dlURL = "https://github.com/CrucibleMC/Crucible/releases/latest/download/crucible.jar"
		}

	case TypePufferfish:
		dlURL = "https://ci.pufferfish.host/job/Pufferfish-1.20/lastSuccessfulBuild/artifact/build/libs/pufferfish-paperclip-1.20.4-R0.1-SNAPSHOT-reobf.jar"

	case TypeLeaf:
		tag := ""
		if version != "" {
			tag = "ver-" + version
		}
		var err error
		dlURL, err = r.resolveGitHubReleaseAsset(ctx, "Winds-Studio/Leaf", tag, "leaf-")
		if err != nil {
			dlURL = fmt.Sprintf("https://github.com/Winds-Studio/Leaf/releases/latest/download/leaf-%s.jar", version)
		}

	case TypeSponge:
		tag := build
		if tag == "" || tag == "latest" {
			tag = r.resolveSpongeTag(ctx, version)
		}
		if tag == "" {
			tag = version
		}
		dlURL = fmt.Sprintf("https://repo.spongepowered.org/repository/maven-public/org/spongepowered/spongevanilla/%s/spongevanilla-%s-universal.jar", tag, tag)
		err := r.DownloadFile(ctx, dlURL, targetJar)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("download sponge: %w", ctx.Err())
		}
		dlURL = fmt.Sprintf("https://repo.spongepowered.org/repository/maven-public/org/spongepowered/spongevanilla/%s/spongevanilla-%s.jar", tag, tag)
		if fbErr := r.DownloadFile(ctx, dlURL, targetJar); fbErr != nil {
			return fmt.Errorf("download sponge: %w (fallback failed: %v)", err, fbErr)
		}
		return nil

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

