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
	"sort"
	"strconv"
	"time"
)

// ErrUpstream is returned when fetching metadata from an upstream provider
// fails (network error, non-200 response, or unparseable body).
var ErrUpstream = errors.New("upstream provider error")

// JarType identifies a supported server platform.
type JarType string

const (
	TypePaper    JarType = "paper"
	TypeFabric   JarType = "fabric"
	TypeVanilla  JarType = "vanilla"
	TypeForge    JarType = "forge"
	TypeNeoForge JarType = "neoforge"
	TypeSpigot   JarType = "spigot"
)

// ParseJarType validates a platform string.
func ParseJarType(s string) (JarType, error) {
	switch JarType(s) {
	case TypePaper, TypeFabric, TypeVanilla, TypeForge, TypeNeoForge, TypeSpigot:
		return JarType(s), nil
	default:
		return "", fmt.Errorf("unsupported jar type %q", s)
	}
}

const (
	defaultPaperBase    = "https://api.papermc.io/v2"
	defaultFabricBase   = "https://meta.fabricmc.net/v2"
	defaultMojangManf   = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	defaultForgeBase    = "https://files.minecraftforge.net/net/minecraftforge/forge"
	defaultNeoForgeBase = "https://api.neoforged.net/neoforges"
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
	ProjectID     string   `json:"project_id"`
	ProjectName   string   `json:"project_name"`
	VersionGroups []string `json:"version_groups"`
	Versions      []string `json:"versions"`
}

// PaperBuildsPage is the JSON shape returned by GET {base}/projects/paper/versions/{v}/builds.
type PaperBuildsPage struct {
	ProjectID string `json:"project_id"`
	Version   string `json:"version"`
	Builds    []struct {
		Build int `json:"build"`
	} `json:"builds"`
}

// ManifestEntry is a single version entry in Mojang's version manifest.
type ManifestEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
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
	return p.Versions, nil
}

// PaperBuilds returns the build numbers for a Paper version.
func (r *Resolver) PaperBuilds(ctx context.Context, version string) ([]Build, error) {
	var page PaperBuildsPage
	if err := r.getJSON(ctx, fmt.Sprintf("%s/projects/paper/versions/%s/builds", r.PaperBase, version), &page); err != nil {
		return nil, err
	}
	builds := make([]Build, 0, len(page.Builds))
	for _, b := range page.Builds {
		builds = append(builds, Build{Number: b.Build})
	}
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
	default:
		return Resolved{}, fmt.Errorf("unsupported jar type %q", jt)
	}
}

func selectBuild(builds []Build, want string) (int, error) {
	if want == "" {
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
	if want == "" {
		return xs[len(xs)-1], nil
	}
	for _, x := range xs {
		if x == want {
			return x, nil
		}
	}
	return "", fmt.Errorf("value %q not found", want)
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
