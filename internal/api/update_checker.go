package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) handleGetUpdateChecker(w http.ResponseWriter, r *http.Request) {
	status, err := s.servers.UpdateCheckerStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read update checks")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePutUpdateChecker(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "enabled must be a boolean")
		return
	}
	if err := s.servers.SetUpdateChecksEnabled(r.Context(), *input.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not save update checks")
		return
	}
	s.handleGetUpdateChecker(w, r)
}

func (s *Server) handleCheckUpdates(w http.ResponseWriter, r *http.Request) {
	if err := s.servers.StartUpdateCheck(s.logger); err != nil {
		if errors.Is(err, servers.ErrUpdateCheckerDisabled) {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not start update checks")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}
