package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/docker"
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
// against a running server on behalf of an online player. It works whether the
// server is reached over RCON or the console stdin pipe.
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
		case errors.Is(err, servers.ErrServerNotRunning):
			writeError(w, http.StatusConflict, "server_not_running",
				"Server is not running - start it before sending player commands")
		case errors.Is(err, docker.ErrConsolePipeDisabled):
			writeError(w, http.StatusConflict, "console_pipe_disabled",
				"Console input isn't enabled on this server's container. Recreate the server to enable it.")
		case errors.Is(err, servers.ErrNotFound):
			s.writeServerErr(w, err)
		default:
			writeError(w, http.StatusInternalServerError, "rcon_error",
				"Couldn't send the command to the server right now.")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "response": response})
}
