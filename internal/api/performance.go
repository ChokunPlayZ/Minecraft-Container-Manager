package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) handlePerformanceHistory(w http.ResponseWriter, r *http.Request) {
	history, err := s.servers.PerformanceHistory(r.Context(), r.PathValue("id"), r.URL.Query().Get("range"))
	if err != nil {
		if errors.Is(err, servers.ErrPerformanceWindow) {
			writeError(w, http.StatusBadRequest, "invalid_range", err.Error())
		} else {
			s.writeServerErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handlePerformanceCollection(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CollectionMode *string `json:"collection_mode"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.CollectionMode == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "collection_mode is required")
		return
	}
	settings, err := s.servers.SetPerformanceCollection(r.Context(), r.PathValue("id"), *input.CollectionMode)
	if err != nil {
		if errors.Is(err, servers.ErrPerformanceCollectionMode) {
			writeError(w, http.StatusBadRequest, "invalid_collection_mode", err.Error())
		} else {
			s.writeServerErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleCaptureSparkReport(w http.ResponseWriter, r *http.Request) {
	report, err := s.servers.CaptureSparkReport(r.Context(), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, servers.ErrServerNotRunning), errors.Is(err, servers.ErrSparkBusy):
			writeError(w, http.StatusConflict, "capture_unavailable", err.Error())
		case errors.Is(err, servers.ErrSparkUnavailable):
			writeError(w, http.StatusUnprocessableEntity, "spark_unavailable", "Install and enable spark, then restart the server to capture a report.")
		default:
			s.writeServerErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, report)
}
