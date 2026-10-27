package api

import (
	"context"
	"net/http"
	"time"
)

// handleDockerStatus reports the reachability of the container runtime and the
// presence of the runtime image used to launch server containers. This lets
// the UI surface why a server might fail to start before the user tries to
// create or launch one.
func (s *Server) handleDockerStatus(w http.ResponseWriter, r *http.Request) {
	if s.servers == nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", "container runtime is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status := s.servers.DockerStatus(ctx)
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleHostCapacity(w http.ResponseWriter, r *http.Request) {
	if s.servers == nil {
		writeError(w, http.StatusServiceUnavailable, "capacity_unavailable", "Host resources are unavailable. Refresh to try again.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.servers.HostCapacity(ctx))
}
