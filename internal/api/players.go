package api

import "net/http"

// handleListPlayers lists the players currently connected to a running server.
func (s *Server) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	res, err := s.servers.PlayerList(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
