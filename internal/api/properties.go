package api

import (
	"net/http"
	"strings"
)

// handleGetProperties returns the raw server.properties content for a server.
func (s *Server) handleGetProperties(w http.ResponseWriter, r *http.Request) {
	if _, err := s.servers.Get(r.Context(), r.PathValue("id")); err != nil {
		s.writeServerErr(w, err)
		return
	}
	props, err := s.servers.GetProperties(r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, props)
}

// handleSaveProperties writes the full server.properties content for a server.
func (s *Server) handleSaveProperties(w http.ResponseWriter, r *http.Request) {
	if _, err := s.servers.Get(r.Context(), r.PathValue("id")); err != nil {
		s.writeServerErr(w, err)
		return
	}
	var in struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if strings.TrimSpace(in.Content) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "server.properties cannot be empty")
		return
	}
	props, err := s.servers.SaveProperties(r.PathValue("id"), in.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Could not save server.properties.")
		return
	}
	writeJSON(w, http.StatusOK, props)
}
