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
	"strings"
	"syscall"
	"testing"
)

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
