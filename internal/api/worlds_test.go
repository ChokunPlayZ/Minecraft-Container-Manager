package api

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestWorldEndpoints(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	users := auth.NewUsers(d.DB)
	user, err := users.Create(context.Background(), "worlds@test.example", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := mustSession(t, d, user.ID)
	_, err = d.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('one','Test','paper','1.21.4',2048,25565,'stopped','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: d, Users: users, Sessions: auth.NewManager(d.DB), Servers: servers.NewStore(d, nil, nil, 25565, 25575, t.TempDir(), "")})
	for _, test := range []struct {
		method, path, body string
		auth, csrf         bool
		want               int
	}{
		{"GET", "/api/servers/one/worlds", "", false, false, 401},
		{"POST", "/api/servers/one/worlds", `{"name":"fresh"}`, true, false, 403},
		{"GET", "/api/servers/missing/worlds", "", true, false, 404},
		{"GET", "/api/servers/one/worlds", "", true, false, 200},
		{"POST", "/api/servers/one/worlds", `{"name":"fresh","seed":"9223372036854775807","level_type":"minecraft:normal","generate_structures":true}`, true, true, 201},
		{"POST", "/api/servers/one/worlds", `{"name":"fresh"}`, true, true, 409},
		{"POST", "/api/servers/one/worlds", `{"name":"../escape"}`, true, true, 400},
		{"POST", "/api/servers/one/worlds/fresh/activate", "", true, true, 200},
		{"DELETE", "/api/servers/one/worlds/fresh", "", true, true, 400},
		{"GET", "/api/servers/one/worlds/fresh/export", "", true, false, 400},
		{"GET", "/api/servers/one/worlds/fresh/regions?dimension=fresh", "", true, false, 200},
		{"GET", "/api/servers/one/worlds/fresh/chunks?dimension=fresh&x=oops&z=0", "", true, false, 400},
		{"GET", "/api/servers/one/worlds/fresh/regions?dimension=mods", "", true, false, 400},
		{"POST", "/api/servers/one/worlds/fresh/chunks/reset", `{"dimension":"fresh","chunks":[]}`, true, true, 400},
	} {
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		if test.auth {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if test.csrf {
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf"})
			req.Header.Set(csrfHeaderName, "test-csrf")
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != test.want {
			t.Fatalf("%s %s = %d want %d: %s", test.method, test.path, rec.Code, test.want, rec.Body)
		}
	}
	// Exercise multipart upload and ZIP response through auth and CSRF middleware.
	var nbt bytes.Buffer
	nbt.Write([]byte{10, 0, 0, 10, 0, 4, 'D', 'a', 't', 'a', 4, 0, 10, 'R', 'a', 'n', 'd', 'o', 'm', 'S', 'e', 'e', 'd'})
	_ = binary.Write(&nbt, binary.BigEndian, int64(9223372036854775807))
	nbt.Write([]byte{0, 0})
	var level bytes.Buffer
	gz := gzip.NewWriter(&level)
	_, _ = gz.Write(nbt.Bytes())
	_ = gz.Close()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	entry, _ := zw.Create("old/level.dat")
	_, _ = entry.Write(level.Bytes())
	entry, _ = zw.Create("old/playerdata/player.dat")
	_, _ = entry.Write([]byte("inventory"))
	_ = zw.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("name", "uploaded")
	part, _ := form.CreateFormFile("file", "save.zip")
	_, _ = part.Write(archive.Bytes())
	_ = form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/servers/one/worlds/import", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf"})
	req.Header.Set(csrfHeaderName, "test-csrf")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"seed":"9223372036854775807"`) {
		t.Fatalf("import: %d %s", rec.Code, rec.Body)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/servers/one/worlds/uploaded/export", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "uploaded.zip") {
		t.Fatalf("export: %d %s", rec.Code, rec.Body)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range zr.File {
		if f.Name == "uploaded/playerdata/player.dat" {
			in, openErr := f.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			data, readErr := io.ReadAll(in)
			in.Close()
			if readErr != nil || string(data) != "inventory" {
				t.Fatalf("exported data: %q %v", data, readErr)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("export omitted player inventory")
	}

}
