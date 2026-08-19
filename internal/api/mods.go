package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/servers"
)

// handleListMods lists the mods or plugins installed for a server.
func (s *Server) handleListMods(w http.ResponseWriter, r *http.Request) {
	res, err := s.servers.ListMods(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleUploadMod accepts a multipart upload of a mod/plugin jar.
func (s *Server) handleUploadMod(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not parse upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing file field")
		return
	}
	defer file.Close()
	mod, err := s.servers.UploadMod(r.Context(), r.PathValue("id"), header.Filename, file)
	if err != nil {
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mod)
}

// handleSetModEnabled toggles a mod/plugin between enabled and disabled.
func (s *Server) handleSetModEnabled(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	mod, err := s.servers.SetModEnabled(r.Context(), r.PathValue("id"), r.PathValue("name"), in.Enabled)
	if err != nil {
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mod)
}

// handleDeleteMod removes an installed mod/plugin.
func (s *Server) handleDeleteMod(w http.ResponseWriter, r *http.Request) {
	err := s.servers.DeleteMod(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) writeModErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servers.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "server not found")
	case errors.Is(err, servers.ErrInvalidModName):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, servers.ErrUnsupportedMods):
		writeError(w, http.StatusBadRequest, "unsupported", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}
