package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
)

// handleListPlayers lists the players currently connected to a running server.
func (s *Server) handleListPlayers(w http.ResponseWriter, r *http.Request) {
	res, err := s.servers.PlayerList(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRunPlayerCommand executes a server command (kick/ban/op/give, etc.)
// against a running server over RCON on behalf of an online player.
func (s *Server) handleRunPlayerCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name := r.PathValue("name")

	var in struct {
		Action string                    `json:"action"`
		Args   servers.PlayerCommandArgs `json:"args"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	command, err := servers.BuildPlayerCommand(name, in.Action, in.Args)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	response, err := s.servers.RunPlayerCommand(r.Context(), id, name, command)
	if err != nil {
		switch {
		case errors.Is(err, servers.ErrRCONDisabled):
			writeError(w, http.StatusBadRequest, "rcon_disabled",
				"RCON is not enabled - set enable-rcon=true in server.properties and restart the server")
		case errors.Is(err, servers.ErrServerNotRunning):
			writeError(w, http.StatusConflict, "server_not_running",
				"Server is not running - start it before sending player commands")
		case errors.Is(err, servers.ErrNotFound):
			s.writeServerErr(w, err)
		default:
			writeError(w, http.StatusInternalServerError, "rcon_error",
				"Couldn't reach the server's RCON endpoint right now.")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "response": response})
}
