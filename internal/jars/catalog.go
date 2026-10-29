package jars

import (
	"context"
	"errors"
	"strconv"

	"github.com/mcm-panel/mcm/internal/checkcache"
)

var ErrUpdateCheckUnsupported = errors.New("automatic software checks are unavailable for this server type")

func (r *Resolver) observeVersions(ctx context.Context, kind JarType, versions []string) []string {
	if r.ObserveCatalog != nil {
		r.ObserveCatalog(ctx, string(kind), "", versions)
	}
	return versions
}

func (r *Resolver) observeBuilds(ctx context.Context, kind JarType, version string, builds []string) []string {
	if r.ObserveCatalog != nil {
		r.ObserveCatalog(ctx, string(kind), version, builds)
	}
	return builds
}

func (r *Resolver) ForUpdateCheck() *Resolver {
	copy := *r
	client := *r.Client
	client.Transport = checkcache.Transport{Base: client.Transport}
	copy.Client = &client
	return &copy
}

// CatalogVersions uses live catalogs only; curated installation fallbacks cannot
// establish whether an update actually exists.
func (r *Resolver) CatalogVersions(ctx context.Context, kind string) ([]string, error) {
	switch JarType(kind) {
	case TypePaper:
		return r.PaperVersions(ctx)
	case TypeFabric:
		return r.FabricGameVersions(ctx)
	case TypeVanilla:
		manifest, err := r.MojangVersions(ctx)
		var versions []string
		for _, v := range manifest.Versions {
			if v.Type == "release" {
				versions = append(versions, v.ID)
			}
		}
		return versions, err
	case TypeForge:
		return r.ForgeGameVersions(ctx)
	case TypeNeoForge:
		return r.NeoForgeGameVersions(ctx)
	case TypeSpigot:
		return r.SpigotGameVersions(ctx)
	case TypePurpur:
		return r.PurpurVersions(ctx)
	case TypeFolia:
		return r.FoliaVersions(ctx)
	case TypeWaterfall:
		return r.WaterfallVersions(ctx)
	case TypeVelocity:
		return r.VelocityVersions(ctx)
	case TypeQuilt:
		return r.QuiltGameVersions(ctx)
	case TypeMohist:
		return r.MohistVersions(ctx)
	case TypeYouer:
		return r.YouerVersions(ctx)
	case TypeSponge:
		return r.SpongeVersions(ctx)
	case TypeGeyser:
		return r.GeyserVersions(ctx)
	case TypeBungeeCord:
		return []string{"latest"}, nil
	default:
		return nil, ErrUpdateCheckUnsupported
	}
}

func (r *Resolver) CatalogBuilds(ctx context.Context, kind, version string) ([]string, error) {
	switch JarType(kind) {
	case TypePaper:
		builds, err := r.PaperBuilds(ctx, version)
		out := make([]string, 0, len(builds))
		for _, b := range builds {
			out = append(out, strconv.Itoa(b.Number))
		}
		return out, err
	case TypeFabric:
		return r.FabricLoaders(ctx, version)
	case TypeForge:
		return r.ForgeBuilds(ctx, version)
	case TypeNeoForge:
		return r.NeoForgeBuilds(ctx, version)
	case TypeSpigot:
		return r.SpigotBuilds(ctx, version)
	case TypePurpur:
		return r.PurpurBuilds(ctx, version)
	case TypeFolia:
		return r.FoliaBuilds(ctx, version)
	case TypeWaterfall:
		return r.WaterfallBuilds(ctx, version)
	case TypeVelocity:
		return r.VelocityBuilds(ctx, version)
	case TypeQuilt:
		return r.QuiltLoaders(ctx, version)
	case TypeMohist:
		return r.MohistBuilds(ctx, version)
	case TypeYouer:
		return r.YouerBuilds(ctx, version)
	case TypeGeyser:
		return r.GeyserBuilds(ctx, version)
	case TypeBungeeCord:
		return r.BungeeCordBuilds(ctx)
	default:
		return nil, nil
	}
}
