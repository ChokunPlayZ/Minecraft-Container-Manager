package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestSavedLogEndpoints(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "logs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()
	users := auth.NewUsers(database.DB)
	user, err := users.Create(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(database.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	store := servers.NewStore(database, nil, jars.NewResolver(), 25565, 25575, dir, "")
	const serverID = "saved-log-test"
	_, err = database.DB.ExecContext(ctx, `INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, created_at, updated_at) VALUES (?, 'Log test', 'paper', '1.21.4', 2048, 25565, '2026-10-26T12:00:00Z', '2026-10-26T12:00:00Z')`, serverID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: database, Users: users, Sessions: sessions, Servers: store})
	logsDir := filepath.Join(dir, "servers", serverID, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var gzipData bytes.Buffer
	gz := gzip.NewWriter(&gzipData)
	if _, err := gz.Write([]byte("Saved Minecraft log\n")); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	if err := tw.WriteHeader(&tar.Header{Name: "saved.log", Mode: 0o644, Size: 7}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("Tar log")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"old.log.gz": gzipData.Bytes(), "history.tar": tarData.Bytes(), "bad.log.gz": []byte("damaged")} {
		if err := os.WriteFile(filepath.Join(logsDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := "/api/servers/" + serverID + "/logs"
	cases := []struct {
		name, target string
		auth         bool
		status       int
	}{
		{"list needs auth", base, false, http.StatusUnauthorized},
		{"content needs auth", base + "/content?path=logs/old.log.gz", false, http.StatusUnauthorized},
		{"list", base, true, http.StatusOK},
		{"archive members", base + "?path=logs/history.tar", true, http.StatusOK},
		{"gzip", base + "/content?path=logs/old.log.gz", true, http.StatusOK},
		{"tar member", base + "/content?path=logs/history.tar&entry=saved.log", true, http.StatusOK},
		{"bad gzip", base + "/content?path=logs/bad.log.gz", true, http.StatusBadRequest},
		{"path traversal", base + "/content?path=" + url.QueryEscape("../outside.log"), true, http.StatusBadRequest},
		{"missing file", base + "/content?path=logs/missing.log", true, http.StatusNotFound},
		{"missing server", "/api/servers/missing/logs", true, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.auth {
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d, want=%d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.name == "gzip" || tc.name == "tar member" {
				var result servers.LogContent
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				want := "Saved Minecraft log\n"
				if tc.name == "tar member" {
					want = "Tar log"
				}
				if result.Content != want || result.Truncated || result.LimitBytes != 2<<20 {
					t.Fatalf("unexpected preview: %+v", result)
				}
			}
		})
	}
}
