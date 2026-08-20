package api

import (
	"net/http"
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
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "That server type isn't supported")
		return
	}
	if err != nil {
		s.logUpstream(err, r)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't fetch builds from the upstream provider")
		return
	}
	out := make([]buildItem, 0, len(builds))
	for _, b := range builds {
		out = append(out, buildItem{Version: version, Build: b, Display: b})
	}
	writeJSON(w, http.StatusOK, out)
}
