package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/schedules"
)

func (s *Server) scheduleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, schedules.ErrNotFound), errors.Is(err, schedules.ErrServerNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, schedules.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_schedule", err.Error())
	default:
		if s.logger != nil {
			s.logger.Printf("schedules: %v", err)
		}
		writeError(w, http.StatusInternalServerError, "internal", "Could not save or load schedules. Try again.")
	}
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.schedules.List(r.Context(), r.PathValue("id"))
	if err != nil {
		s.scheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": tasks, "default_timezone": schedules.DefaultTimezone()})
}

func (s *Server) handleCreateSchedule(w http.ResponseWriter, r *http.Request) {
	// Omitted enabled defaults to true; explicit false creates a paused task.
	input := schedules.Input{Enabled: true}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	task, err := s.schedules.Create(r.Context(), r.PathValue("id"), input)
	if err != nil {
		s.scheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) handleUpdateSchedule(w http.ResponseWriter, r *http.Request) {
	var input schedules.Input
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	task, err := s.schedules.Update(r.Context(), r.PathValue("id"), r.PathValue("scheduleId"), input)
	if err != nil {
		s.scheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleToggleSchedule(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "enabled must be true or false")
		return
	}
	task, err := s.schedules.SetEnabled(r.Context(), r.PathValue("id"), r.PathValue("scheduleId"), *input.Enabled)
	if err != nil {
		s.scheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if err := s.schedules.Delete(r.Context(), r.PathValue("id"), r.PathValue("scheduleId")); err != nil {
		s.scheduleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
