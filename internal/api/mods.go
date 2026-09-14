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

	meta := servers.ModDownloadMeta{
		ProjectID:   r.FormValue("project_id"),
		ProjectSlug: r.FormValue("project_slug"),
		Provider:    r.FormValue("provider"),
	}

	if oldName := r.FormValue("delete_old_name"); oldName != "" {
		if meta.Provider == "" && meta.ProjectID == "" {
			if oldMod, err := s.servers.GetMod(r.Context(), r.PathValue("id"), oldName); err == nil {
				meta.Provider = oldMod.Provider
				meta.ProjectID = oldMod.ProjectID
				meta.ProjectSlug = oldMod.ProjectSlug
			}
		}
		_ = s.servers.DeleteMod(r.Context(), r.PathValue("id"), oldName)
	}

	mod, err := s.servers.UploadMod(r.Context(), r.PathValue("id"), header.Filename, file, meta)
	if err != nil {
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mod)
}

// handleDownloadMod downloads a mod/plugin artifact from a remote URL.
func (s *Server) handleDownloadMod(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL           string `json:"url"`
		Filename      string `json:"filename"`
		DeleteOldName string `json:"delete_old_name"`
		ProjectID     string `json:"project_id"`
		ProjectSlug   string `json:"project_slug"`
		Provider      string `json:"provider"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.URL == "" || in.Filename == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "url and filename are required")
		return
	}

	meta := servers.ModDownloadMeta{
		ProjectID:   in.ProjectID,
		ProjectSlug: in.ProjectSlug,
		Provider:    in.Provider,
	}

	if in.DeleteOldName != "" {
		if meta.Provider == "" && meta.ProjectID == "" {
			if oldMod, err := s.servers.GetMod(r.Context(), r.PathValue("id"), in.DeleteOldName); err == nil {
				meta.Provider = oldMod.Provider
				meta.ProjectID = oldMod.ProjectID
				meta.ProjectSlug = oldMod.ProjectSlug
			}
		}
		_ = s.servers.DeleteMod(r.Context(), r.PathValue("id"), in.DeleteOldName)
	}

	mod, err := s.servers.DownloadMod(r.Context(), r.PathValue("id"), in.Filename, in.URL, meta)
	if err != nil {
		if errors.Is(err, servers.ErrDownloadFailed) {
			writeError(w, http.StatusBadGateway, "download_failed", "Failed to download mod from remote URL.")
			return
		}
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

func contextWithCurseForgeKey(r *http.Request) *http.Request {
	cfKey := r.Header.Get("x-curseforge-api-key")
	if cfKey == "" {
		cfKey = r.Header.Get("x-api-key")
	}
	if cfKey != "" {
		ctx := servers.WithCurseForgeAPIKey(r.Context(), cfKey)
		return r.WithContext(ctx)
	}
	return r
}

// handleGetModUpdates retrieves current updates for a server's mods/plugins.
func (s *Server) handleGetModUpdates(w http.ResponseWriter, r *http.Request) {
	r = contextWithCurseForgeKey(r)
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"
	res, err := s.servers.CheckModUpdates(r.Context(), r.PathValue("id"), force)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleCheckModUpdates forces an update check across online catalogs.
func (s *Server) handleCheckModUpdates(w http.ResponseWriter, r *http.Request) {
	r = contextWithCurseForgeKey(r)
	res, err := s.servers.CheckModUpdates(r.Context(), r.PathValue("id"), true)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleGetModVersions returns available releases/jars for a specific mod.
func (s *Server) handleGetModVersions(w http.ResponseWriter, r *http.Request) {
	r = contextWithCurseForgeKey(r)
	jars, err := s.servers.GetModAvailableJars(r.Context(), r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jars)
}

// handleUpdateMod downloads and applies a new jar version, removing the old jar.
func (s *Server) handleUpdateMod(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL         string `json:"url"`
		Filename    string `json:"filename"`
		DeleteOld   bool   `json:"delete_old"`
		ProjectID   string `json:"project_id"`
		ProjectSlug string `json:"project_slug"`
		Provider    string `json:"provider"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.URL == "" || in.Filename == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "url and filename are required")
		return
	}

	meta := servers.ModDownloadMeta{
		ProjectID:   in.ProjectID,
		ProjectSlug: in.ProjectSlug,
		Provider:    in.Provider,
	}

	var oldName string
	if in.DeleteOld {
		oldName = r.PathValue("name")
		if meta.Provider == "" && meta.ProjectID == "" && oldName != "" {
			if oldMod, err := s.servers.GetMod(r.Context(), r.PathValue("id"), oldName); err == nil {
				meta.Provider = oldMod.Provider
				meta.ProjectID = oldMod.ProjectID
				meta.ProjectSlug = oldMod.ProjectSlug
			}
		}
		_ = s.servers.DeleteMod(r.Context(), r.PathValue("id"), oldName)
	}

	mod, err := s.servers.DownloadMod(r.Context(), r.PathValue("id"), in.Filename, in.URL, meta)
	if err != nil {
		if errors.Is(err, servers.ErrDownloadFailed) {
			writeError(w, http.StatusBadGateway, "download_failed", "Failed to download mod from remote URL.")
			return
		}
		s.writeModErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mod)
}

func (s *Server) writeModErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servers.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "server not found")
	case errors.Is(err, servers.ErrInvalidModName):
		writeError(w, http.StatusBadRequest, "invalid_request", "That file isn't a valid mod or plugin.")
	case errors.Is(err, servers.ErrUnsupportedMods):
		writeError(w, http.StatusBadRequest, "unsupported", "This server type doesn't support mods or plugins.")
	default:
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong while handling your request.")
	}
}
