package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) writeWorldErr(w http.ResponseWriter, err error) {
	var validation *servers.WorldValidationError
	switch {
	case errors.Is(err, servers.ErrNotFound), errors.Is(err, servers.ErrPathNotFound), os.IsNotExist(err):
		writeError(w, 404, "not_found", "The server, world or region was not found. Refresh and try again.")
	case errors.Is(err, servers.ErrWorldBusy):
		writeError(w, 409, "world_busy", err.Error())
	case errors.Is(err, servers.ErrWorldConflict):
		writeError(w, 409, "world_conflict", err.Error())
	case errors.Is(err, servers.ErrWorldUnsupported):
		writeError(w, 400, "world_unsupported", err.Error())
	case errors.As(err, &validation):
		writeError(w, 400, "invalid_world", validation.Message)
	case errors.Is(err, servers.ErrInvalidPath), errors.Is(err, servers.ErrInvalidArchive):
		writeError(w, 400, "invalid_world", "The world contains an unsafe path, symbolic link or invalid ZIP entry.")
	default:
		s.logger.Printf("world operation: %v", err)
		writeError(w, 500, "internal", "The world operation failed. Check available storage and file permissions, then try again.")
	}
}
func (s *Server) handleListWorlds(w http.ResponseWriter, r *http.Request) {
	result, err := s.servers.ListWorlds(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleSaveWorld(create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in servers.WorldInput
		if err := decodeJSON(w, r, &in); err != nil {
			writeError(w, 400, "invalid_request", "Invalid world settings.")
			return
		}
		if !create {
			in.Name = r.PathValue("world")
		}
		result, err := s.servers.SaveWorld(r.Context(), r.PathValue("id"), in, create)
		if err != nil {
			s.writeWorldErr(w, err)
			return
		}
		status := 200
		if create {
			status = 201
		}
		writeJSON(w, status, result)
	}
}
func (s *Server) handleActivateWorld(w http.ResponseWriter, r *http.Request) {
	result, err := s.servers.ActivateWorld(r.Context(), r.PathValue("id"), r.PathValue("world"))
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleDeleteWorld(w http.ResponseWriter, r *http.Request) {
	recovery, err := s.servers.DeleteWorld(r.Context(), r.PathValue("id"), r.PathValue("world"))
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "recovery_path": recovery})
}
func (s *Server) handleImportWorld(w http.ResponseWriter, r *http.Request) {
	// Reject unknown servers before receiving a potentially large upload.
	if _, err := s.servers.Get(r.Context(), r.PathValue("id")); err != nil {
		s.writeWorldErr(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, servers.MaxWorldUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		writeError(w, 400, "invalid_upload", "Upload a ZIP of at most 2 GiB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	upload, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "invalid_upload", "Choose a world ZIP file.")
		return
	}
	defer upload.Close()
	if header.Size > servers.MaxWorldUploadBytes {
		writeError(w, 413, "upload_too_large", "World ZIP files must be 2 GiB or smaller.")
		return
	}
	temp, err := os.CreateTemp("", "mcm-world-upload-*.zip")
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	defer os.Remove(temp.Name())
	_, err = io.Copy(temp, upload)
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	result, err := s.servers.ImportWorld(r.Context(), r.PathValue("id"), r.FormValue("name"), temp.Name())
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 201, result)
}
func (s *Server) handleExportWorld(w http.ResponseWriter, r *http.Request) {
	filename, err := s.servers.ExportWorld(r.Context(), r.PathValue("id"), r.PathValue("world"))
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	defer os.Remove(filename)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": r.PathValue("world") + ".zip"}))
	http.ServeFile(w, r, filename)
}
func (s *Server) handleWorldRegions(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if value := r.URL.Query().Get("offset"); value != "" {
		var err error
		offset, err = strconv.Atoi(value)
		if err != nil {
			writeError(w, 400, "invalid_request", "Invalid region page.")
			return
		}
	}
	result, err := s.servers.ListWorldRegions(r.Context(), r.PathValue("id"), r.PathValue("world"), r.URL.Query().Get("dimension"), offset)
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) handleWorldChunks(w http.ResponseWriter, r *http.Request) {
	x, errX := strconv.Atoi(r.URL.Query().Get("x"))
	z, errZ := strconv.Atoi(r.URL.Query().Get("z"))
	if errX != nil || errZ != nil {
		writeError(w, 400, "invalid_request", "Region coordinates must be integers.")
		return
	}
	result, err := s.servers.ListWorldChunks(r.Context(), r.PathValue("id"), r.PathValue("world"), r.URL.Query().Get("dimension"), x, z)
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"chunks": result})
}
func (s *Server) handleResetWorldChunks(w http.ResponseWriter, r *http.Request) {
	var in servers.WorldChunkSelection
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, 400, "invalid_request", "Invalid chunk selection.")
		return
	}
	result, err := s.servers.ResetWorldChunks(r.Context(), r.PathValue("id"), r.PathValue("world"), in)
	if err != nil {
		s.writeWorldErr(w, err)
		return
	}
	writeJSON(w, 200, result)
}
