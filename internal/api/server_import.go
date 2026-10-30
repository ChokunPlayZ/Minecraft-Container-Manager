package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/storage"
)

func (s *Server) handleImportServer(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, storage.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "Server ZIP files must be 2 GiB or smaller.")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_upload", "Choose an exported server ZIP file and try again.")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Choose an exported server ZIP file.")
		return
	}
	defer file.Close()
	if header.Size > storage.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "upload_too_large", "Server ZIP files must be 2 GiB or smaller.")
		return
	}
	var in servers.ImportInput
	if settings := r.FormValue("settings"); settings != "" {
		if len(settings) > 16<<10 || json.Unmarshal([]byte(settings), &in) != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "The server import settings are invalid.")
			return
		}
	}
	srv, err := s.servers.Import(r.Context(), file, header.Size, in)
	if err != nil {
		var validation *servers.ImportValidationError
		switch {
		case errors.As(err, &validation):
			writeError(w, http.StatusBadRequest, "invalid_import", validation.Message)
		case errors.Is(err, servers.ErrInvalidArchive), errors.Is(err, storage.ErrUnsafe):
			writeError(w, http.StatusBadRequest, "invalid_archive", "The ZIP is damaged or contains unsafe paths, links or duplicate files. Export the server again and retry.")
		default:
			s.writeServerErr(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, srv)
}
