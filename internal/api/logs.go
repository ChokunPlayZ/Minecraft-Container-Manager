package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	if _, err := s.servers.Get(r.Context(), r.PathValue("id")); err != nil {
		s.writeFileErr(w, r, err)
		return
	}
	var files []servers.LogFile
	var err error
	if rel := r.URL.Query().Get("path"); rel != "" {
		files, err = s.servers.ListLogArchive(r.Context(), r.PathValue("id"), rel)
	} else {
		files, err = s.servers.ListServerLogs(r.Context(), r.PathValue("id"))
	}
	if err != nil {
		s.writeLogErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handleReadLog(w http.ResponseWriter, r *http.Request) {
	if _, err := s.servers.Get(r.Context(), r.PathValue("id")); err != nil {
		s.writeFileErr(w, r, err)
		return
	}
	res, err := s.servers.ReadServerLog(r.Context(), r.PathValue("id"), r.URL.Query().Get("path"), r.URL.Query().Get("entry"))
	if err != nil {
		s.writeLogErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) writeLogErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, servers.ErrInvalidLogArchive):
		writeError(w, http.StatusBadRequest, "invalid_log_archive", "This compressed log or tar archive could not be read. It may be damaged or incomplete.")
	case errors.Is(err, servers.ErrLogScanLimit):
		writeError(w, http.StatusRequestEntityTooLarge, "log_scan_limit", "There are too many log entries or the archive is too large to browse. Use Files to download it.")
	default:
		s.writeFileErr(w, r, err)
	}
}
