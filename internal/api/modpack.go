package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

// handleGetInstalledModpack returns the currently installed modpack on the server.
func (s *Server) handleGetInstalledModpack(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	modpack, err := s.servers.GetInstalledModpack(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, servers.ErrModpackNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{
				"installed": false,
				"modpack":   nil,
			})
			return
		}
		s.writeServerErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"installed": true,
		"modpack":   modpack,
	})
}

// handleInspectModpack inspects an uploaded archive or remote URL.
func (s *Server) handleInspectModpack(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")

	// 1. Multipart form file upload
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(512 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse upload")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "missing file in form")
			return
		}
		defer file.Close()

		tmpFile, err := os.CreateTemp("", "mcm_inspect_*.zip")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create temp file")
			return
		}
		defer func() {
			tmpFile.Close()
			_ = os.Remove(tmpFile.Name())
		}()

		if _, err := io.Copy(tmpFile, file); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save temp file")
			return
		}

		manifest, err := s.servers.InspectModpackFile(tmpFile.Name())
		if err != nil {
			if errors.Is(err, servers.ErrInvalidModpack) || errors.Is(err, servers.ErrInvalidArchive) {
				writeError(w, http.StatusBadRequest, "invalid_modpack", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if manifest.Name == "Modpack" && header.Filename != "" {
			manifest.Name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		}

		writeJSON(w, http.StatusOK, manifest)
		return
	}

	// 2. JSON request with remote URL
	var in struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if in.URL == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "url is required")
		return
	}

	tmpFile, err := downloadURLToTemp(r.Context(), in.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "download_failed", fmt.Sprintf("failed to fetch modpack: %v", err))
		return
	}
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	manifest, err := s.servers.InspectModpackFile(tmpFile)
	if err != nil {
		if errors.Is(err, servers.ErrInvalidModpack) || errors.Is(err, servers.ErrInvalidArchive) {
			writeError(w, http.StatusBadRequest, "invalid_modpack", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

// handleInstallModpack installs a modpack from multipart file or remote URL.
func (s *Server) handleInstallModpack(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	contentType := r.Header.Get("Content-Type")

	cfKey := r.Header.Get("x-curseforge-api-key")

	// 1. Multipart Form Upload
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(512 << 20); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not parse modpack upload")
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "missing file parameter")
			return
		}
		defer file.Close()

		tmpFile, err := os.CreateTemp("", "mcm_modpack_upload_*.zip")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to create temp file")
			return
		}
		tmpPath := tmpFile.Name()
		defer func() {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
		}()

		if _, err := io.Copy(tmpFile, file); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to write modpack archive")
			return
		}
		_ = tmpFile.Close()

		autoConfig := true
		if acVal := r.FormValue("auto_configure_server"); acVal != "" {
			if parsed, err := strconv.ParseBool(acVal); err == nil {
				autoConfig = parsed
			}
		}

		createdWithModpack := false
		if cwmVal := r.FormValue("created_with_modpack"); cwmVal != "" {
			if parsed, err := strconv.ParseBool(cwmVal); err == nil {
				createdWithModpack = parsed
			}
		}

		opts := servers.InstallModpackOpts{
			Source:              "upload",
			AutoConfigureServer: autoConfig,
			CurseForgeAPIKey:    cfKey,
			CreatedWithModpack:  createdWithModpack,
		}

		installed, err := s.servers.InstallModpack(r.Context(), serverID, tmpPath, opts)
		if err != nil {
			if errors.Is(err, servers.ErrModpackLocked) {
				writeError(w, http.StatusForbidden, "modpack_locked", err.Error())
				return
			}
			if errors.Is(err, servers.ErrInvalidModpack) || errors.Is(err, servers.ErrInvalidArchive) {
				writeError(w, http.StatusBadRequest, "invalid_modpack", err.Error())
				return
			}
			s.writeServerErr(w, err)
			return
		}

		writeJSON(w, http.StatusOK, installed)
		return
	}

	// 2. JSON body (URL or Catalog source)
	var in struct {
		URL                 string `json:"url"`
		Source              string `json:"source"`
		ProjectID           string `json:"project_id"`
		ProjectSlug         string `json:"project_slug"`
		VersionID           string `json:"version_id"`
		AutoConfigureServer bool   `json:"auto_configure_server"`
		CurseForgeAPIKey    string `json:"curseforge_api_key"`
		CreatedWithModpack  bool   `json:"created_with_modpack"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if in.URL == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "url is required")
		return
	}

	if in.CurseForgeAPIKey == "" {
		in.CurseForgeAPIKey = cfKey
	}

	tmpPath, err := downloadURLToTemp(r.Context(), in.URL)
	if err != nil {
		writeError(w, http.StatusBadGateway, "download_failed", fmt.Sprintf("failed to download modpack archive: %v", err))
		return
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	source := in.Source
	if source == "" {
		source = "url"
	}

	opts := servers.InstallModpackOpts{
		Source:              source,
		URL:                 in.URL,
		ProjectID:           in.ProjectID,
		ProjectSlug:         in.ProjectSlug,
		VersionID:           in.VersionID,
		AutoConfigureServer: in.AutoConfigureServer,
		CurseForgeAPIKey:    in.CurseForgeAPIKey,
		CreatedWithModpack:  in.CreatedWithModpack,
	}

	installed, err := s.servers.InstallModpack(r.Context(), serverID, tmpPath, opts)
	if err != nil {
		if errors.Is(err, servers.ErrModpackLocked) {
			writeError(w, http.StatusForbidden, "modpack_locked", err.Error())
			return
		}
		if errors.Is(err, servers.ErrInvalidModpack) || errors.Is(err, servers.ErrInvalidArchive) {
			writeError(w, http.StatusBadRequest, "invalid_modpack", err.Error())
			return
		}
		s.writeServerErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, installed)
}

// handleUninstallModpack uninstalls the current modpack and removes its tracked files.
func (s *Server) handleUninstallModpack(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if err := s.servers.UninstallModpack(r.Context(), serverID); err != nil {
		if errors.Is(err, servers.ErrModpackLocked) {
			writeError(w, http.StatusForbidden, "modpack_locked", "servers created with a modpack cannot have their modpack removed")
			return
		}
		if errors.Is(err, servers.ErrModpackNotFound) {
			writeError(w, http.StatusNotFound, "modpack_not_found", "no modpack installed")
			return
		}
		s.writeServerErr(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func downloadURLToTemp(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("invalid URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mcm-panel/1.0 (https://github.com/mcm-panel/mcm)")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "mcm_modpack_dl_*.zip")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}
