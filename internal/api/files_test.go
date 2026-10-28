package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestFileContentRevisionAPI(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "files.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	store := servers.NewStore(database, nil, nil, 25565, 25575, dir, "")
	if _, err := database.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('id','test','custom','custom',1024,0,'stopped','now','now')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{servers: store}
	if _, err := store.WriteFileContent("id", "config.txt", "opened"); err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRequest(http.MethodGet, "/?path=config.txt", nil)
	read.SetPathValue("id", "id")
	rr := httptest.NewRecorder()
	s.handleReadFileContent(rr, read)
	var opened servers.FileContent
	if err := json.Unmarshal(rr.Body.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusOK || opened.Revision == "" || !opened.Exists || rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read snapshot: %d %s", rr.Code, rr.Body.String())
	}
	save := func(content, revision string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(map[string]string{"path": "config.txt", "content": content, "expected_revision": revision})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
		req.SetPathValue("id", "id")
		res := httptest.NewRecorder()
		s.handleWriteFileContent(res, req)
		return res
	}
	if _, err := store.WriteFileContent("id", "config.txt", "external change"); err != nil {
		t.Fatal(err)
	}
	conflict := save("my edits", opened.Revision)
	var body struct {
		Error   struct{ Code string } `json:"error"`
		Current servers.FileContent   `json:"current"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if conflict.Code != http.StatusConflict || body.Error.Code != "file_changed" || body.Current.Content != "external change" || body.Current.Revision == opened.Revision {
		t.Fatalf("conflict: %d %s", conflict.Code, conflict.Body.String())
	}
	accepted := save("my edits", body.Current.Revision)
	var entry servers.FileEntry
	if err := json.Unmarshal(accepted.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if accepted.Code != http.StatusCreated || entry.Revision == "" || entry.Revision == body.Current.Revision {
		t.Fatalf("accepted save: %d %s", accepted.Code, accepted.Body.String())
	}
}

func TestWriteFileErrReportsFilesystemFailures(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{"permission denied", syscall.EACCES, http.StatusForbidden, "permission_denied", "ownership and permissions"},
		{"operation not permitted", syscall.EPERM, http.StatusForbidden, "permission_denied", "ownership and permissions"},
		{"read only", syscall.EROFS, http.StatusInternalServerError, "read_only_filesystem", "read-only"},
		{"disk full", syscall.ENOSPC, http.StatusInsufficientStorage, "insufficient_storage", "Free up space"},
		{"quota exceeded", syscall.EDQUOT, http.StatusInsufficientStorage, "insufficient_storage", "disk quota"},
		{"directory", syscall.EISDIR, http.StatusBadRequest, "is_directory", "folder, not a file"},
		{"parent is a file", syscall.ENOTDIR, http.StatusBadRequest, "invalid_path", "path is a file"},
		{"missing", syscall.ENOENT, http.StatusNotFound, "not_found", "doesn't exist"},
		{"unknown", errors.New("device failure"), http.StatusInternalServerError, "internal", "Something went wrong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			s := &Server{logger: log.New(&logs, "", 0)}
			req := httptest.NewRequest(http.MethodPut, "/api/servers/test-server/files/content", nil)
			req.SetPathValue("id", "test-server")
			rr := httptest.NewRecorder()
			const file = "/data/servers/test-server/config/mod.toml"
			err := fmt.Errorf("save config: %w", &os.PathError{Op: "open", Path: file, Err: tc.err})
			s.writeFileErr(rr, req, err)

			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.status, rr.Body.String())
			}
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != tc.code || !strings.Contains(body.Error.Message, tc.message) {
				t.Fatalf("unexpected error response: %s", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), file) || strings.Contains(rr.Body.String(), "save config:") {
				t.Fatalf("response leaked filesystem details: %s", rr.Body.String())
			}
			for _, want := range []string{"file operation failed", `server="test-server"`, "method=PUT", req.URL.Path, file, tc.err.Error()} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("log missing %q: %s", want, logs.String())
				}
			}
		})
	}
}

func TestWriteFileErrWithoutLogger(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/api/servers/id/files/content", nil)
	rr := httptest.NewRecorder()
	s.writeFileErr(rr, req, &os.PathError{Op: "open", Path: "/data/config.toml", Err: syscall.EACCES})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}
