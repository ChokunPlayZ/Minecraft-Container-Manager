package jars

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ForgeBuild is a single Forge build for a Minecraft version.
type ForgeBuild struct {
	Version string `json:"version"`
	Build   int    `json:"build"`
}

// ForgeMetadata is the JSON shape returned by GET {base}/maven-metadata.json,
// keyed by Minecraft version.
type ForgeMetadata map[string][]ForgeBuild

// NeoForgeReleases is the JSON shape returned by GET {base}/releases.
type NeoForgeReleases struct {
	Promos   map[string]string `json:"promos"`
	Versions []string          `json:"versions"`
}

// SpigotVersions is the JSON shape returned by GET {base}/, keyed by version.
// Values are opaque per-version metadata we do not need to decode.
type SpigotVersions map[string]json.RawMessage

func (r *Resolver) forgeBase() string {
	if r.ForgeBase != "" {
		return r.ForgeBase
	}
	return defaultForgeBase
}

func (r *Resolver) neoForgeBase() string {
	if r.NeoForgeBase != "" {
		return r.NeoForgeBase
	}
	return defaultNeoForgeBase
}

func (r *Resolver) spigotBase() string {
	if r.SpigotBase != "" {
		return r.SpigotBase
	}
	return defaultSpigotBase
}

// ForgeGameVersions returns the Minecraft versions that have Forge builds.
func (r *Resolver) ForgeGameVersions(ctx context.Context) ([]string, error) {
	var meta ForgeMetadata
	if err := r.getJSON(ctx, r.forgeBase()+"/maven-metadata.json", &meta); err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(meta))
	for v := range meta {
		versions = append(versions, v)
	}
	return sortedStrings(versions), nil
}

// ForgeBuilds returns the full Forge versions (e.g. "1.21.1-52.0.14") for a
// Minecraft version.
func (r *Resolver) ForgeBuilds(ctx context.Context, version string) ([]string, error) {
	var meta ForgeMetadata
	if err := r.getJSON(ctx, r.forgeBase()+"/maven-metadata.json", &meta); err != nil {
		return nil, err
	}
	builds, ok := meta[version]
	if !ok {
		return nil, fmt.Errorf("no forge builds for version %q", version)
	}
	out := make([]string, 0, len(builds))
	for _, b := range builds {
		out = append(out, b.Version)
	}
	return out, nil
}

// NeoForgeGameVersions returns the Minecraft versions that have NeoForge
// builds, derived from NeoForge "mc-forgever" version strings.
func (r *Resolver) NeoForgeGameVersions(ctx context.Context) ([]string, error) {
	var rel NeoForgeReleases
	if err := r.getJSON(ctx, r.neoForgeBase()+"/releases", &rel); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, v := range rel.Versions {
		i := strings.Index(v, "-")
		if i <= 0 {
			continue
		}
		mc := v[:i]
		if !seen[mc] {
			seen[mc] = true
			out = append(out, mc)
		}
	}
	return sortedStrings(out), nil
}

// NeoForgeBuilds returns the full NeoForge versions (e.g. "1.20-47.1.106") for
// a Minecraft version.
func (r *Resolver) NeoForgeBuilds(ctx context.Context, version string) ([]string, error) {
	var rel NeoForgeReleases
	if err := r.getJSON(ctx, r.neoForgeBase()+"/releases", &rel); err != nil {
		return nil, err
	}
	prefix := version + "-"
	var out []string
	for _, v := range rel.Versions {
		if strings.HasPrefix(v, prefix) {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no neoforge builds for version %q", version)
	}
	return out, nil
}

// SpigotGameVersions returns the Minecraft versions tracked by Spigot's hub.
func (r *Resolver) SpigotGameVersions(ctx context.Context) ([]string, error) {
	var versions SpigotVersions
	if err := r.getJSON(ctx, r.spigotBase()+"/", &versions); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(versions))
	for v := range versions {
		out = append(out, v)
	}
	return sortedStrings(out), nil
}

// SpigotBuilds returns the available builds for a Spigot version. GetBukkit
// only publishes the latest jar per version, so this returns a single
// "latest" entry after confirming the version exists.
func (r *Resolver) SpigotBuilds(ctx context.Context, version string) ([]string, error) {
	versions, err := r.SpigotGameVersions(ctx)
	if err != nil {
		return nil, err
	}
	if !containsString(versions, version) {
		return nil, fmt.Errorf("spigot version %q not found", version)
	}
	return []string{"latest"}, nil
}
