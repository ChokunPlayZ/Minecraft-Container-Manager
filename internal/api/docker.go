package api

import (
	"context"
	"net/http"
	"time"
)

// handleDockerStatus reports the reachability of the Docker daemon and the
// presence of the mcm-server runtime image. This lets the UI surface why a
// server might fail to start before the user tries to create or launch one.
func (s *Server) handleDockerStatus(w http.ResponseWriter, r *http.Request) {
	if s.servers == nil {
		writeError(w, http.StatusServiceUnavailable, "docker_unavailable", "docker integration is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	status := s.servers.DockerStatus(ctx)
	writeJSON(w, http.StatusOK, status)
}
