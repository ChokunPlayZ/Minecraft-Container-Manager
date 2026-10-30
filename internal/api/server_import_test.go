package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestHandleImportServer(t *testing.T) {
	dir := t.TempDir()
	dbh, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbh.Close() })
	ctx := context.Background()
	users := auth.NewUsers(dbh.DB)
	user, err := users.Create(ctx, "admin@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(dbh.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: dbh, Users: users, Sessions: sessions, Servers: servers.NewStore(dbh, nil, nil, 25565, 25575, dir, "")})
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, err := zw.Create("server.jar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("original jar")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, settings string
		data           []byte
		status         int
	}{
		{"success", `{"name":"Imported","server_type":"paper","version":"1.21.1","ram_mb":4096}`, archive.Bytes(), 201},
		{"missing settings", `{}`, archive.Bytes(), 400},
		{"invalid settings", `{`, archive.Bytes(), 400},
		{"missing file", `{}`, nil, 400},
		{"invalid archive", `{"name":"Bad","server_type":"paper","version":"1.21.1"}`, []byte("bad zip"), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			if tc.data != nil {
				part, err := form.CreateFormFile("file", "server-export.zip")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := part.Write(tc.data); err != nil {
					t.Fatal(err)
				}
			}
			if err := form.WriteField("settings", tc.settings); err != nil {
				t.Fatal(err)
			}
			if err := form.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/servers/import", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			csrf := mustCSRF(t)
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
			req.Header.Set(csrfHeaderName, csrf)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.status {
				t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
			}
			if tc.status == 201 {
				var srv servers.Server
				if err := json.Unmarshal(rr.Body.Bytes(), &srv); err != nil {
					t.Fatal(err)
				}
				if srv.Name != "Imported" || srv.RAMMB != 4096 || srv.State != servers.StateStopped || srv.HostPort != 25565 {
					t.Fatalf("unexpected import: %+v", srv)
				}
			}
		})
	}
	for _, signedIn := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodPost, "/api/servers/import", bytes.NewReader(nil))
		want := http.StatusUnauthorized
		if signedIn {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			want = http.StatusForbidden // authenticated uploads still require CSRF.
		} else {
			csrf := mustCSRF(t)
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
			req.Header.Set(csrfHeaderName, csrf)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != want {
			t.Fatalf("authentication/CSRF: got %d, want %d: %s", rr.Code, want, rr.Body.String())
		}
	}
}
