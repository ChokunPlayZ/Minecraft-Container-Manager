package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/nbt"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestNBTEndpoints(t *testing.T) {
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	users := auth.NewUsers(d.DB)
	user, err := users.Create(context.Background(), "nbt@test.example", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := mustSession(t, d, user.ID)
	_, err = d.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('one','Test','paper','1.21.4',2048,25565,'stopped','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	s := servers.NewStore(d, nil, nil, 25565, 25575, dir, "")
	const rel = "world/playerdata/8667ba71-b85a-4004-af54-457a9734eed7.dat"
	data, err := nbt.Encode(nbt.Document{Compression: "gzip", Root: nbt.Tag{Type: 10, Children: []nbt.Tag{{Type: 4, Name: "long", Value: "9223372036854775807"}}}})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "servers/one", rel)
	if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: d, Users: users, Sessions: auth.NewManager(d.DB), Servers: s})
	call := func(method, url, body string, authenticated, csrf bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		if authenticated {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "nbt-csrf"})
			req.Header.Set(csrfHeaderName, "nbt-csrf")
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	for _, route := range []string{"/api/servers/one/files/nbt?path=" + rel, "/api/servers/one/players/data"} {
		if r := call("GET", route, "", false, false); r.Code != 401 {
			t.Fatalf("unauthenticated read: %d", r.Code)
		}
		if r := call("GET", route, "", true, false); r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("read: %d %s", r.Code, r.Body)
		}
	}
	read := call("GET", "/api/servers/one/files/nbt?path="+rel, "", true, false)
	var file servers.NBTFile
	if err = json.Unmarshal(read.Body.Bytes(), &file); err != nil {
		t.Fatal(err)
	}
	if file.Root.Children[0].Value != "9223372036854775807" {
		t.Fatal("long lost precision")
	}
	file.Root.Children[0].Value = "-9223372036854775808"
	body, _ := json.Marshal(map[string]any{"path": rel, "expected_revision": file.Revision, "root": file.Root})
	for _, test := range []struct {
		auth, csrf bool
		want       int
	}{{false, true, 401}, {true, false, 403}, {true, true, 200}, {true, true, 409}} {
		r := call("PUT", "/api/servers/one/files/nbt", string(body), test.auth, test.csrf)
		if r.Code != test.want {
			t.Fatalf("save: %d want %d: %s", r.Code, test.want, r.Body)
		}
	}
	for _, test := range []struct {
		method, url, body string
		want              int
	}{
		{"GET", "/api/servers/missing/files/nbt?path=" + rel, "", 404},
		{"GET", "/api/servers/one/files/nbt?path=../secret", "", 400},
		{"GET", "/api/servers/one/files/nbt?path=missing.dat", "", 404},
		{"PUT", "/api/servers/one/files/nbt", `{"path":"` + rel + `","root":{"type":10,"name":""}}`, 400},
		{"PUT", "/api/servers/one/files/nbt", `{} {}`, 400},
	} {
		r := call(test.method, test.url, test.body, true, true)
		if r.Code != test.want {
			t.Fatalf("%s: %d want %d: %s", test.url, r.Code, test.want, r.Body)
		}
	}
	_, err = d.DB.Exec(`UPDATE servers SET state='running' WHERE id='one'`)
	if err != nil {
		t.Fatal(err)
	}
	r := call("PUT", "/api/servers/one/files/nbt", string(body), true, true)
	if r.Code != 409 || !strings.Contains(r.Body.String(), "nbt_busy") {
		t.Fatalf("live save accepted: %d %s", r.Code, r.Body)
	}
}
