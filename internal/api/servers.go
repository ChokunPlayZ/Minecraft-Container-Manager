package api

import (
	"errors"
	"net/http"

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

func (s *Server) handleAvailablePorts(w http.ResponseWriter, r *http.Request) {
	free, err := s.servers.Pool().Available(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not query available ports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": free})
}

func (s *Server) handleServerConsole(w http.ResponseWriter, r *http.Request) {
	rc, err := s.servers.Console(r.Context(), r.PathValue("id"), true)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	defer rc.Close()
	rc = s.configureConsoleJoinWatcher(r.Context(), r.PathValue("id"), rc)
	s.streamConsole(r.Context(), w, rc)
}

func (s *Server) handleInstall(provision bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := s.servers.Install(r.Context(), r.PathValue("id"), provision)
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
	if errors.Is(err, ports.ErrPortPoolFull) {
		writeError(w, http.StatusConflict, "port_pool_full", "port pool full")
		return
	}
	if errors.Is(err, jars.ErrUpstream) || errors.Is(err, servers.ErrUpstream) {
		s.logUpstream(err, nil)
		writeError(w, http.StatusBadGateway, "upstream_error", "Couldn't reach the upstream provider right now.")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong while handling your request.")
}

// friendlyCreateErr maps a server-creation error to a user-facing status, code,
// and message. Upstream fetch failures surface as 502; jar validation problems
// as 400; everything else as 500.
func friendlyCreateErr(err error) (int, string, string) {
	if errors.Is(err, jars.ErrUpstream) || errors.Is(err, servers.ErrUpstream) {
		return http.StatusBadGateway, "upstream_error", "Couldn't reach the upstream provider right now."
	}
	if errors.Is(err, servers.ErrInvalidJar) {
		return http.StatusBadRequest, "invalid_request", "That server type or version isn't supported."
	}
	return http.StatusInternalServerError, "internal", "Something went wrong while handling your request."
}
