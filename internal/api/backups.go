package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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
		Name    string `json:"name"`
		Storage string `json:"storage"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	b, err := s.backups.Backup(context.WithoutCancel(r.Context()), id, body.Name, body.Storage)
	if err != nil {
		s.writeBackupErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) handleBackupProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}
	prog := s.backups.GetProgress(id)
	if prog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":   true,
		"progress": prog,
	})
}

func (s *Server) handleBackupEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cancel := s.backups.SubscribeProgress(id)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case prog, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(prog)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleUploadBackup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), id); err != nil {
		s.writeServerErr(w, err)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing file parameter")
		return
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" && header != nil {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		name = strings.TrimSuffix(name, ".tar")
	}
	storage := r.FormValue("storage")

	b, err := s.backups.UploadBackup(context.WithoutCancel(r.Context()), id, name, file, header.Size, storage)
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
	if err := s.backups.Restore(context.WithoutCancel(r.Context()), r.PathValue("backupId")); err != nil {
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

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	backupID := r.PathValue("backupId")
	rc, size, name, err := s.backups.OpenBackup(r.Context(), backupID)
	if err != nil {
		s.writeBackupErr(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if _, err := io.Copy(w, rc); err != nil && s.logger != nil {
		s.logger.Printf("download backup failed id=%s err=%v", backupID, err)
	}
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

