package api

import (
	"net/http"
)

// handleListWhitelist returns the whitelisted players configured for a server.
func (s *Server) handleListWhitelist(w http.ResponseWriter, r *http.Request) {
	entries, err := s.servers.ListWhitelist(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"whitelist": entries})
}

// handleAddWhitelist whitelists a player.
func (s *Server) handleAddWhitelist(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	entries, err := s.servers.AddWhitelist(r.Context(), r.PathValue("id"), in.Name)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"whitelist": entries})
}

// handleRemoveWhitelist removes a player from the whitelist.
func (s *Server) handleRemoveWhitelist(w http.ResponseWriter, r *http.Request) {
	err := s.servers.RemoveWhitelist(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
