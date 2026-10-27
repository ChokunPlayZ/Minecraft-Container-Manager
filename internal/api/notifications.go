package api

import (
	"net/http"

	"github.com/mcm-panel/mcm/internal/notifications"
)

func (s *Server) handleGetDiscord(w http.ResponseWriter, r *http.Request) {
	settings, err := s.notifications.Settings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read notification settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutDiscord(w http.ResponseWriter, r *http.Request) {
	var input notifications.Update
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	settings, err := s.notifications.Save(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleTestDiscord(w http.ResponseWriter, r *http.Request) {
	var input struct {
		WebhookURL *string `json:"webhook_url"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := s.notifications.Test(r.Context(), input.WebhookURL); err != nil {
		writeError(w, http.StatusBadGateway, "discord_delivery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Test message sent to Discord."})
}
