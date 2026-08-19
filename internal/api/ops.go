package api

import (
	"net/http"
)

// Ops lists the operators configured for a server.
func (s *Server) handleListOps(w http.ResponseWriter, r *http.Request) {
	ops, err := s.servers.ListOps(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ops": ops})
}

// AddOP promotes a player to operator.
func (s *Server) handleAddOP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Level int    `json:"level"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	ops, err := s.servers.AddOP(r.Context(), r.PathValue("id"), in.Name, in.Level)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ops": ops})
}

// RemoveOP revokes operator status for a player.
func (s *Server) handleRemoveOP(w http.ResponseWriter, r *http.Request) {
	err := s.servers.RemoveOP(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
