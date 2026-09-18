package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) handleListServers(w http.ResponseWriter, r *http.Request) {
	list, err := s.servers.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list servers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": list})
}

func (s *Server) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	var in servers.CreateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.Name == "" || in.Version == "" || in.RAMMB <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "name, version, and ram_mb are required")
		return
	}
	// Apply configured defaults for resource limits when not explicitly set.
	if s.cfg != nil {
		if in.CPULimit == 0 {
			in.CPULimit = s.cfg.DefaultCPULimit
		}
		if in.MemoryLimitMB == 0 {
			in.MemoryLimitMB = s.cfg.DefaultMemoryMB
		}
	}
	srv, err := s.servers.Create(r.Context(), in)
	if err != nil {
		if errors.Is(err, ports.ErrPortPoolFull) {
			writeError(w, http.StatusConflict, "port_pool_full", "port pool full")
			return
		}
		if errors.Is(err, servers.ErrPortInUse) {
			writeError(w, http.StatusConflict, "port_in_use", "The selected port is already in use by another server.")
			return
		}
		if errors.Is(err, jars.ErrUpstream) || errors.Is(err, servers.ErrUpstream) {
			s.logUpstream(err, r)
		}
		status, code, message := friendlyCreateErr(err)
		writeError(w, status, code, message)
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

func (s *Server) handleGetServer(w http.ResponseWriter, r *http.Request) {
	srv, err := s.servers.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	var in servers.UpdateInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	srv, err := s.servers.Update(r.Context(), r.PathValue("id"), in)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	if err := s.servers.Delete(r.Context(), r.PathValue("id")); err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleServerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			srv servers.Server
			err error
		)
		switch action {
		case "start":
			srv, err = s.servers.Start(r.Context(), r.PathValue("id"))
		case "stop":
			srv, err = s.servers.Stop(r.Context(), r.PathValue("id"))
		case "kill":
			srv, err = s.servers.Kill(r.Context(), r.PathValue("id"))
		case "restart":
			srv, err = s.servers.Restart(r.Context(), r.PathValue("id"))
		}
		if err != nil {
			s.writeServerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, srv)
	}
}

func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	srv, err := s.servers.Status(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) handleServerStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.servers.Stats(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleRecreateServer(w http.ResponseWriter, r *http.Request) {
	srv, err := s.servers.Recreate(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

func (s *Server) handleAvailablePorts(w http.ResponseWriter, r *http.Request) {
	free, err := s.servers.Pool().Available(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not query available ports")
		return
	}
	configured, err := s.servers.Pool().ConfiguredPorts(r.Context())
	if err != nil {
		configured = free
	}
	used, _ := s.servers.Pool().UsedDetails(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"available": free, "pool": configured, "used": used})
}

func (s *Server) handleCheckPort(w http.ResponseWriter, r *http.Request) {
	portStr := r.URL.Query().Get("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeError(w, http.StatusBadRequest, "invalid_port", "Invalid port number")
		return
	}
	serverID := r.URL.Query().Get("server_id")
	if checkErr := s.servers.CheckPortFree(r.Context(), serverID, port); checkErr != nil {
		msg := checkErr.Error()
		if idx := strings.Index(msg, "port already in use: "); idx >= 0 {
			msg = msg[idx+len("port already in use: "):]
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"port":    port,
			"in_use":  true,
			"message": msg,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"port":    port,
		"in_use":  false,
		"message": "Port is available",
	})
}

func (s *Server) handleExportServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	srv, err := s.servers.Get(r.Context(), id)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	cleanName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, srv.Name)
	if cleanName == "" {
		cleanName = "server"
	}
	filename := fmt.Sprintf("%s-export.zip", cleanName)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if err := s.servers.Export(r.Context(), id, w); err != nil {
		if s.logger != nil {
			s.logger.Printf("server export failed id=%s err=%v", id, err)
		}
	}
}

func (s *Server) handleCopyServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in servers.CopyInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	srv, err := s.servers.Copy(r.Context(), id, in)
	if err != nil {
		if errors.Is(err, servers.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "server not found")
			return
		}
		if errors.Is(err, ports.ErrPortPoolFull) {
			writeError(w, http.StatusConflict, "port_pool_full", "port pool full")
			return
		}
		if errors.Is(err, servers.ErrPortInUse) {
			writeError(w, http.StatusConflict, "port_in_use", "The selected port is already in use by another server.")
			return
		}
		if s.logger != nil {
			s.logger.Printf("copy server failed source_id=%s err=%v", id, err)
		}
		writeError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("Failed to copy server: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}

func (s *Server) handleServerConsole(w http.ResponseWriter, r *http.Request) {
	rc, err := s.servers.Console(r.Context(), r.PathValue("id"), true)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	defer rc.Close()
	lastID := 0
	if lastIDStr := r.Header.Get("Last-Event-ID"); lastIDStr != "" {
		if n, err := strconv.Atoi(lastIDStr); err == nil && n > 0 {
			lastID = n
		}
	}
	s.streamConsole(r.Context(), w, rc, lastID)
}

// handleServerConsoleCommand sends a single command to a running server's
// console via the container stdin. It does not require RCON to be enabled.
func (s *Server) handleServerConsoleCommand(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Command string `json:"command"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := s.servers.SendConsoleCommand(r.Context(), r.PathValue("id"), in.Command); err != nil {
		switch {
		case errors.Is(err, servers.ErrServerNotRunning):
			writeError(w, http.StatusConflict, "server_not_running",
				"Server is not running - start it before sending console commands")
		case errors.Is(err, docker.ErrConsolePipeDisabled):
			writeError(w, http.StatusConflict, "console_pipe_disabled",
				"Console input isn't enabled on this server's container. Recreate the server to enable it.")
		default:
			s.logConsoleCmdError(err, r)
			writeError(w, http.StatusInternalServerError, "console_error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) logConsoleCmdError(err error, r *http.Request) {
	if s.logger == nil {
		return
	}
	s.logger.Printf("console command failed server=%s method=%s path=%s err=%v",
		r.PathValue("id"), r.Method, r.URL.Path, err)
}

func (s *Server) handleInstall(provision bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input servers.InstallInput
		if provision && r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&input)
		}
		res, err := s.servers.Install(r.Context(), r.PathValue("id"), provision, input)
		if err != nil {
			s.writeServerErr(w, err)
			return
		}
		status := http.StatusOK
		if provision {
			status = http.StatusCreated
		}
		writeJSON(w, status, res)
	}
}

func (s *Server) writeServerErr(w http.ResponseWriter, err error) {
	if errors.Is(err, servers.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "server not found")
		return
	}
	if errors.Is(err, servers.ErrPortInUse) {
		msg := err.Error()
		if idx := strings.Index(msg, "port already in use: "); idx >= 0 {
			msg = msg[idx+len("port already in use: "):]
		}
		writeError(w, http.StatusConflict, "port_in_use", msg)
		return
	}
	if errors.Is(err, ports.ErrPortPoolFull) {
		writeError(w, http.StatusConflict, "port_pool_full", "port pool full")
		return
	}
	if errors.Is(err, jars.ErrUpstream) || errors.Is(err, servers.ErrUpstream) {
		s.logUpstream(err, nil)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't reach the upstream provider right now.")
		return
	}
	// The error is intentionally hidden from the client, but it must reach the
	// logs so operators can diagnose why a server action failed.
	s.logger.Printf("server action failed err=%v", err)
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong while handling your request.")
}

// friendlyCreateErr maps a server-creation error to a user-facing status, code,
// and message. Upstream fetch failures surface as 502; jar validation problems
// as 400; port conflicts as 409; everything else as 500.
func friendlyCreateErr(err error) (int, string, string) {
	if errors.Is(err, servers.ErrPortInUse) {
		msg := err.Error()
		if idx := strings.Index(msg, "port already in use: "); idx >= 0 {
			msg = msg[idx+len("port already in use: "):]
		}
		return http.StatusConflict, "port_in_use", msg
	}
	if errors.Is(err, jars.ErrUpstream) || errors.Is(err, servers.ErrUpstream) {
		return http.StatusBadGateway, "upstream_error", "Couldn't reach the upstream provider right now."
	}
	if errors.Is(err, servers.ErrInvalidJar) {
		return http.StatusBadRequest, "invalid_request", "That server type or version isn't supported."
	}
	return http.StatusInternalServerError, "internal", "Something went wrong while handling your request."
}
