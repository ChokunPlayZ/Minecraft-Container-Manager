package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mcm-panel/mcm/internal/servers"
)

// Normalize a client-supplied relative path by stripping a leading slash so it
// is always treated as relative to the server's data directory.
func normalizeRel(rel string) string {
	return strings.TrimPrefix(rel, "/")
}

// handleListFiles lists a directory inside the server's data dir.
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	rel := normalizeRel(r.URL.Query().Get("path"))
	res, err := s.servers.ListFiles(r.PathValue("id"), rel)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDownloadFile streams a file from the server's data dir.
func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	rel := normalizeRel(r.URL.Query().Get("path"))
	handle, name, err := s.servers.OpenFileForDownload(r.PathValue("id"), rel)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	defer handle.Close()

	info, err := handle.Stat()
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(name)+"\"")
	http.ServeContent(w, r, name, info.ModTime(), handle)
}

// handleUploadFile saves an uploaded file into the server's data dir.
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not parse upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing file field")
		return
	}
	// The file reader must be closed after the handler finishes to avoid
	// leaking the temp file backing a large multipart upload.
	defer file.Close()

	dir := normalizeRel(r.URL.Query().Get("dir"))
	entry, err := s.servers.UploadFile(r.PathValue("id"), dir, header.Filename, file)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleArchiveFile zips a file or directory.
func (s *Server) handleArchiveFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Source string `json:"source"`
		Name   string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entry, err := s.servers.Archive(r.PathValue("id"), in.Source, in.Name)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleUnzipFile extracts a zip archive into a destination directory.
func (s *Server) handleUnzipFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Archive string `json:"archive"`
		Dest    string `json:"dest"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	count, err := s.servers.Unzip(r.PathValue("id"), in.Archive, in.Dest)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
}

// handleDownloadFromURL downloads a file from a URL into the server's data dir.
func (s *Server) handleDownloadFromURL(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL  string `json:"url"`
		Dir  string `json:"dir"`
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entry, err := s.servers.DownloadFromURL(r.PathValue("id"), in.URL, in.Dir, in.Name)
	if err != nil {
		if errors.Is(err, servers.ErrDownloadFailed) {
			writeError(w, http.StatusBadGateway, "download_failed",
				"Couldn't download from that URL. Check the link and try again.")
			return
		}
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleDeleteFile deletes a file or directory within the server's data dir.
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	rel := normalizeRel(r.URL.Query().Get("path"))
	if err := s.servers.DeleteFile(r.PathValue("id"), rel); err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleMkdir creates a directory within the server's data dir.
func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entry, err := s.servers.Mkdir(r.PathValue("id"), in.Path)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleRenameFile renames or moves a path within the server's data dir.
func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entry, err := s.servers.Rename(r.PathValue("id"), in.Path, in.Name)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// writeFileErr maps file-operation errors to friendly HTTP responses.
func (s *Server) writeFileErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, servers.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "server not found")
	case errors.Is(err, servers.ErrInvalidPath):
		writeError(w, http.StatusBadRequest, "invalid_path", "That path isn't valid.")
	case errors.Is(err, servers.ErrPathNotFound):
		writeError(w, http.StatusNotFound, "not_found", "That file or folder doesn't exist.")
	case errors.Is(err, servers.ErrIsDirectory):
		writeError(w, http.StatusBadRequest, "is_directory", "That entry is a folder, not a file.")
	case errors.Is(err, servers.ErrInvalidArchive):
		writeError(w, http.StatusBadRequest, "invalid_archive", "That file isn't a valid zip archive.")
	case errors.Is(err, io.ErrShortBuffer), errors.Is(err, io.ErrUnexpectedEOF):
		writeError(w, http.StatusBadRequest, "invalid_archive", "That zip archive couldn't be read.")
	default:
		writeError(w, http.StatusInternalServerError, "internal",
			"Something went wrong while handling your request.")
	}
}
