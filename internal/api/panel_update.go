package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mcm-panel/mcm/internal/buildinfo"
	"github.com/mcm-panel/mcm/internal/panelupdate"
)

func (s *Server) handlePanelUpdateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.panelUpdates == nil {
		writeJSON(w, http.StatusOK, panelupdate.Status{Build: buildinfo.Current(), Reason: "Web updates require an installation managed by mcmctl."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := s.panelUpdates.Status(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update_status_failed", "Could not read panel update status: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePanelUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.panelUpdates == nil {
		writeError(w, http.StatusConflict, "unsupported", "Web updates require an installation managed by mcmctl.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	status, err := s.panelUpdates.Check(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "update_check_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handlePanelUpdateStart(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Commit string `json:"commit"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.Commit == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "The checked commit is required.")
		return
	}
	if s.panelUpdates == nil {
		writeError(w, http.StatusConflict, "unsupported", "Web updates require an installation managed by mcmctl.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	status, err := s.panelUpdates.Start(ctx, input.Commit)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, panelupdate.ErrConflict) {
			code = http.StatusConflict
		}
		writeError(w, code, "update_start_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, status)
}
