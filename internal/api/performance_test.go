package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestPerformanceEndpoints(t *testing.T) {
	handle, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	users := auth.NewUsers(handle.DB)
	user, err := users.Create(context.Background(), "performance@test.example", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := mustSession(t, handle, user.ID)
	_, err = handle.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('one','Test','paper','1.21.1',2048,25565,'stopped','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Options{DB: handle, Users: users, Sessions: auth.NewManager(handle.DB), Servers: servers.NewStore(handle, nil, nil, 25565, 25575, t.TempDir(), "")})
	for _, test := range []struct {
		method, path string
		auth, csrf   bool
		want         int
	}{
		{"GET", "/api/servers/one/performance", false, false, 401},
		{"POST", "/api/servers/one/performance/spark", false, true, 401},
		{"POST", "/api/servers/one/performance/spark", true, false, 403},
		{"POST", "/api/servers/one/performance/spark", true, true, 409},
		{"GET", "/api/servers/one/performance?range=unbounded", true, false, 400},
		{"GET", "/api/servers/missing/performance", true, false, 404},
		{"POST", "/api/servers/missing/performance/spark", true, true, 404},
		{"GET", "/api/servers/one/performance?range=7d", true, false, 200},
	} {
		req := httptest.NewRequest(test.method, test.path, nil)
		if test.auth {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if test.csrf {
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf"})
			req.Header.Set(csrfHeaderName, "test-csrf")
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		if response.Code != test.want {
			t.Fatalf("%s %s = %d: %s", test.method, test.path, response.Code, response.Body)
		}
		if test.want == 200 {
			var history servers.PerformanceHistory
			if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
				t.Fatal(err)
			}
			if history.Samples == nil || !history.Spark.Available || history.ResolutionSeconds != 1800 || history.Settings.CollectionMode != "off" {
				t.Fatalf("history=%+v", history)
			}
		}
	}
	for _, test := range []struct {
		body, id            string
		authenticated, csrf bool
		want                int
	}{
		{`{"collection_mode":"console"}`, "one", false, true, 401},
		{`{"collection_mode":"console"}`, "one", true, false, 403},
		{`{}`, "one", true, true, 400},
		{`{"collection_mode":null}`, "one", true, true, 400},
		{`{"collection_mode":true}`, "one", true, true, 400},
		{`{"collection_mode":"automatic"}`, "one", true, true, 400},
		{`{"collection_mode":""}`, "one", true, true, 400},
		{`{"collection_mode":"off"}`, "missing", true, true, 404},
		{`{"collection_mode":"console"}`, "one", true, true, 200},
		{`{"collection_mode":"rcon"}`, "one", true, true, 200},
		{`{"collection_mode":"off"}`, "one", true, true, 200},
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/servers/"+test.id+"/performance/settings", strings.NewReader(test.body))
		if test.authenticated {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if test.csrf {
			req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf"})
			req.Header.Set(csrfHeaderName, "test-csrf")
		}
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		if response.Code != test.want {
			t.Fatalf("settings %s = %d: %s", test.body, response.Code, response.Body)
		}
		if test.want == 200 {
			var settings servers.PerformanceSettings
			if err := json.Unmarshal(response.Body.Bytes(), &settings); err != nil {
				t.Fatal(err)
			}
			var mode string
			if err := handle.DB.QueryRow(`SELECT collection_mode FROM performance_settings WHERE server_id='one'`).Scan(&mode); err != nil || mode != settings.CollectionMode {
				t.Fatalf("mode=%s settings=%+v err=%v", mode, settings, err)
			}
		}
	}
}
