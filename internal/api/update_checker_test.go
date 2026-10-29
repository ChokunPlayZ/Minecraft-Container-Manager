package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestUpdateCheckerAPIOptInAndAuthentication(t *testing.T) {
	h, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	users := auth.NewUsers(h.DB)
	user, err := users.Create(context.Background(), "updates@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(h.DB)
	token, err := sessions.Create(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: h, Servers: servers.NewStore(h, nil, nil, 25565, 25575, t.TempDir(), ""), Users: users, Sessions: sessions})
	request := func(method, path, body string, authenticated bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			addAuthAndCSRF(t, req, token)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	for _, item := range []struct{ method, path string }{{"GET", "/api/updates"}, {"PUT", "/api/updates/settings"}, {"POST", "/api/updates/check"}} {
		if rr := request(item.method, item.path, `{"enabled":true}`, false); rr.Code < 400 {
			t.Fatalf("unauthenticated %s accepted", item.path)
		}
	}
	if rr := request("GET", "/api/updates", "", true); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"enabled":false`) {
		t.Fatalf("default: %d %s", rr.Code, rr.Body.String())
	}
	if rr := request("POST", "/api/updates/check", `{}`, true); rr.Code != 400 {
		t.Fatalf("opt-out accepted a scan: %d", rr.Code)
	}
	for _, body := range []string{`{}`, `{"enabled":"true"}`, `{"enabled":null}`} {
		if rr := request("PUT", "/api/updates/settings", body, true); rr.Code != 400 {
			t.Fatalf("invalid setting: %d", rr.Code)
		}
	}
	if rr := request("PUT", "/api/updates/settings", `{"enabled":true}`, true); rr.Code != 200 || !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Fatalf("enable: %d %s", rr.Code, rr.Body.String())
	}
	if rr := request("PUT", "/api/settings", `{"settings":{"update_checker_enabled":"true"}}`, true); rr.Code != http.StatusBadRequest {
		t.Fatal("generic settings bypassed opt-in endpoint")
	}
	if rr := request("PUT", "/api/updates/settings", `{"enabled":false}`, true); rr.Code != 200 {
		t.Fatalf("disable: %d", rr.Code)
	}
}

func TestBuildQueriesImmediatelyMarkMatchingServersOutdated(t *testing.T) {
	h, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	requests := 0
	resolver := jars.NewResolverWithBases(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		requests++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"id":226},{"id":227}]`))}
	})}, "https://catalog.test", "", "")
	store := servers.NewStore(h, nil, resolver, 25565, 25575, t.TempDir(), "")
	if _, err := h.DB.Exec(`INSERT INTO servers (id,name,server_type,version,build,ram_mb,host_port,state,created_at,updated_at)
		VALUES ('older','older','paper','1.21.4','226',1024,25565,'stopped','',''),
		('current','current','paper','1.21.4','227',1024,25566,'stopped','',''),
		('other','other','folia','1.21.4','226',1024,25567,'stopped','','')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{jars: resolver, servers: store}
	status := func() servers.UpdateCheckerStatus {
		t.Helper()
		rr := httptest.NewRecorder()
		s.handleGetUpdateChecker(rr, httptest.NewRequest(http.MethodGet, "/api/updates", nil))
		var result servers.UpdateCheckerStatus
		if rr.Code != 200 {
			t.Fatalf("update status: %d %s", rr.Code, rr.Body.String())
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if before := status(); before.Enabled || len(before.Servers) != 0 || requests != 0 {
		t.Fatalf("status started a scan: %+v, %d requests", before, requests)
	}
	if code, body := getJar(t, s, false, map[string]string{"kind": "paper", "v": "1.21.4"}); code != http.StatusOK {
		t.Fatalf("build query: %d %s", code, body)
	}
	after := status()
	if requests != 1 || after.Enabled || after.LastChecked != "" || after.Servers["older"].Software == nil || !after.Servers["older"].Software.Available || after.Servers["older"].Software.LatestBuild != "227" {
		t.Fatalf("build query did not update badges: %+v, %d requests", after, requests)
	}
	for _, id := range []string{"current", "other"} {
		if result := after.Servers[id].Software; result != nil && result.Available {
			t.Errorf("%s marked outdated: %+v", id, result)
		}
	}
}
