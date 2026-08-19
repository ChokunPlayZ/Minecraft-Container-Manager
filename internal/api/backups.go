package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) handleBackupServer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	b, err := s.backups.Backup(r.Context(), id, body.Name)
	if err != nil {
		s.writeBackupErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}
	list, err := s.backups.List(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list backups")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": list})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}
	if err := s.backups.Restore(r.Context(), r.PathValue("backupId")); err != nil {
		s.writeBackupErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.backups.Delete(r.Context(), r.PathValue("backupId")); err != nil {
		s.writeBackupErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) writeBackupErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, backups.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "backup not found")
	case errors.Is(err, backups.ErrNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "backup_not_configured", "S3 backup storage is not configured")
	case errors.Is(err, servers.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "server not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}
