package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/mcm-panel/mcm/internal/jars"
)

type versionItem struct {
	Name   string `json:"name"`
	Latest string `json:"latest,omitempty"`
}

type buildItem struct {
	Version string `json:"version"`
	Build   string `json:"build"`
	Display string `json:"display"`
}

func (s *Server) handleSystemJavaVersions(w http.ResponseWriter, r *http.Request) {
	releases, err := s.jars.AvailableJavaVersions(r.Context())
	if err != nil {
		s.logUpstream(err, r)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't fetch Java versions")
		return
	}
	writeJSON(w, http.StatusOK, releases)
}

func (s *Server) handleJarVersions(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	var versions []string
	var err error
	switch kind {
	case "paper":
		versions, err = s.jars.PaperVersions(r.Context())
	case "fabric":
		versions, err = s.jars.FabricGameVersions(r.Context())
	case "vanilla":
		var m jars.VersionManifest
		m, err = s.jars.MojangVersions(r.Context())
		if err == nil {
			versions = make([]string, 0, len(m.Versions))
			for _, v := range m.Versions {
				t := strings.ToLower(v.Type)
				if t == "release" || t == "snapshot" {
					versions = append(versions, v.ID)
				}
			}
		}
	case "forge":
		versions, err = s.jars.ForgeGameVersions(r.Context())
	case "neoforge":
		versions, err = s.jars.NeoForgeGameVersions(r.Context())
	case "spigot":
		versions, err = s.jars.SpigotGameVersions(r.Context())
	case "purpur":
		versions, err = s.jars.PurpurVersions(r.Context())
	case "folia":
		versions, err = s.jars.FoliaVersions(r.Context())
	case "waterfall":
		versions, err = s.jars.WaterfallVersions(r.Context())
	case "velocity":
		versions, err = s.jars.VelocityVersions(r.Context())
	case "quilt":
		versions, err = s.jars.QuiltGameVersions(r.Context())
	case "mohist":
		versions, err = s.jars.MohistVersions(r.Context())
	case "sponge":
		versions, err = s.jars.SpongeVersions(r.Context())
	case "geysermc":
		versions, err = s.jars.GeyserVersions(r.Context())
	case "bungeecord":
		versions = []string{"latest"}
	case "pufferfish":
		versions = []string{"1.20.4", "1.20.2", "1.20.1", "1.19.4"}
	case "leaf":
		versions = []string{"1.21.1", "1.21", "1.20.4"}
	case "ketting":
		versions = []string{"1.20.4", "1.20.2", "1.20.1"}
	case "limbo", "nanolimbo":
		versions = []string{"latest", "1.21", "1.20"}
	case "crucible":
		versions = []string{"1.7.10"}
	case "custom":
		versions = []string{"custom"}
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "That server type isn't supported")
		return
	}
	if err != nil {
		s.logUpstream(err, r)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't fetch versions from the upstream provider")
		return
	}
	out := make([]versionItem, 0, len(versions))
	for _, v := range versions {
		out = append(out, versionItem{Name: v})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleJarBuilds(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	version := r.PathValue("v")
	var (
		builds []string
		err    error
	)
	switch kind {
	case "paper":
		var nums []jars.Build
		nums, err = s.jars.PaperBuilds(r.Context(), version)
		for _, b := range nums {
			builds = append(builds, strconv.Itoa(b.Number))
		}
	case "fabric":
		builds, err = s.jars.FabricLoaders(r.Context(), version)
	case "vanilla":
		builds = []string{"latest"}
	case "forge":
		builds, err = s.jars.ForgeBuilds(r.Context(), version)
	case "neoforge":
		builds, err = s.jars.NeoForgeBuilds(r.Context(), version)
	case "spigot":
		builds, err = s.jars.SpigotBuilds(r.Context(), version)
	case "purpur":
		builds, err = s.jars.PurpurBuilds(r.Context(), version)
	case "folia":
		builds, err = s.jars.FoliaBuilds(r.Context(), version)
	case "waterfall":
		builds, err = s.jars.WaterfallBuilds(r.Context(), version)
	case "velocity":
		builds, err = s.jars.VelocityBuilds(r.Context(), version)
	case "quilt":
		builds, err = s.jars.QuiltLoaders(r.Context(), version)
	case "mohist":
		builds, err = s.jars.MohistBuilds(r.Context(), version)
	case "sponge":
		builds = []string{"latest"}
	case "geysermc":
		builds, err = s.jars.GeyserBuilds(r.Context(), version)
	case "bungeecord":
		builds, err = s.jars.BungeeCordBuilds(r.Context())
	case "pufferfish", "leaf", "ketting", "limbo", "nanolimbo", "crucible":
		builds = []string{"latest"}
	case "custom":
		builds = []string{"custom"}
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "That server type isn't supported")
		return
	}
	if err != nil {
		s.logUpstream(err, r)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't fetch builds from the upstream provider")
		return
	}
	sort.SliceStable(builds, func(i, j int) bool {
		return jars.CompareVersionTokens(builds[i], builds[j]) > 0
	})
	out := make([]buildItem, 0, len(builds))
	for _, b := range builds {
		out = append(out, buildItem{Version: version, Build: b, Display: b})
	}
	writeJSON(w, http.StatusOK, out)
}
