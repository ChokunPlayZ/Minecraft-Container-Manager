package api

import (
	"net/http"
	"strconv"

	"github.com/mcm-panel/mcm/internal/jars"
)

type versionList struct {
	Versions []string `json:"versions"`
}

type buildList struct {
	Project string  `json:"project"`
	Version string  `json:"version"`
	Builds  []build `json:"builds"`
}

type build struct {
	Build string `json:"build"`
}

func (s *Server) handleJarVersions(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	var (
		versions []string
		err      error
	)
	switch kind {
	case "paper":
		versions, err = s.jars.PaperVersions(r.Context())
	case "fabric":
		versions, err = s.jars.FabricGameVersions(r.Context())
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported jar type")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "could not fetch versions")
		return
	}
	writeJSON(w, http.StatusOK, versionList{Versions: versions})
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
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported jar type")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_error", "could not fetch builds")
		return
	}
	out := make([]build, 0, len(builds))
	for _, b := range builds {
		out = append(out, build{Build: b})
	}
	writeJSON(w, http.StatusOK, buildList{Project: kind, Version: version, Builds: out})
}
