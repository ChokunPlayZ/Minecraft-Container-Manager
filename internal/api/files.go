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

// handleReadFileContent returns a file's text content for the web editor.
func (s *Server) handleReadFileContent(w http.ResponseWriter, r *http.Request) {
	rel := normalizeRel(r.URL.Query().Get("path"))
	content, err := s.servers.ReadFileContent(r.PathValue("id"), rel)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": rel, "content": content})
}

// handleWriteFileContent saves text content to a file in the server data dir,
// creating the file and any parent directories as needed.
func (s *Server) handleWriteFileContent(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	entry, err := s.servers.WriteFileContent(r.PathValue("id"), normalizeRel(in.Path), in.Content)
	if err != nil {
		s.writeFileErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleUploadFile saves one or more uploaded files into the server's data dir.
// The multipart request may carry multiple "file" fields, which are all saved
// into the same destination directory.
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not parse upload")
		return
	}
	dir := normalizeRel(r.URL.Query().Get("dir"))
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing file field")
		return
	}
	entries := make([]servers.FileEntry, 0, len(files))
	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read uploaded file")
			return
		}
		// The reader must be closed after each upload to avoid leaking the temp
		// file backing a large multipart part.
		entry, uerr := s.servers.UploadFile(r.PathValue("id"), dir, fh.Filename, file)
		_ = file.Close()
		if uerr != nil {
			s.writeFileErr(w, uerr)
			return
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusCreated, entries)
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
	case errors.Is(err, servers.ErrReadTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large",
			"That file is too large to edit in the browser. Download it and edit locally instead.")
	case errors.Is(err, io.ErrShortBuffer), errors.Is(err, io.ErrUnexpectedEOF):
		writeError(w, http.StatusBadRequest, "invalid_archive", "That zip archive couldn't be read.")
	default:
		writeError(w, http.StatusInternalServerError, "internal",
			"Something went wrong while handling your request.")
	}
}
