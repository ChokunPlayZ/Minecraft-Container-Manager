package api

import (
	"context"
	"encoding/json"
	"github.com/go-webauthn/webauthn/webauthn"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/servers"
)

func permissionRequest(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrf := mustCSRF(t)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func seedPermissionServer(t *testing.T, store *db.Store, id string, port int) {
	t.Helper()
	_, err := store.DB.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,host_port,created_at,updated_at) VALUES (?,?,'paper','1.21.4',1024,?,'now','now')`, id, id, port)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPermissionIsolationAndEscalation(t *testing.T) {
	store, _ := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	sessions := auth.NewManager(store.DB)
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	seedPermissionServer(t, store, "one", 25565)
	seedPermissionServer(t, store, "two", 25566)
	viewer, err := users.CreateWithAccess(ctx, "viewer@example.test", "hash", auth.Access{Role: "viewer", ServerIDs: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Create(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: store, Users: users, Sessions: sessions, Servers: servers.NewStore(store, nil, nil, 25565, 25575, t.TempDir(), ""), Backups: backups.New(store.DB, backups.S3Config{}, t.TempDir())})
	for _, route := range []string{"GET /api/users", "GET /api/permissions", "GET /api/settings", "GET /api/notifications/discord", "GET /api/dns", "GET /api/system/capacity", "GET /api/panel/update", "GET /api/servers/two", "GET /api/servers/two/console", "GET /api/servers/one/export", "GET /api/servers/one/files", "POST /api/servers/one/start", "POST /api/servers/one/console/command", "POST /api/servers/one/players/Steve/command", "POST /api/servers/one/schedules", "POST /api/servers/one/mods", "POST /api/updates/check", "DELETE /api/servers/one", "PATCH /api/users/" + admin.ID} {
		method, path, _ := strings.Cut(route, " ")
		rr := permissionRequest(t, handler, token, method, path, `{"email":"hijack@example.test"}`)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s: got %d: %s", route, rr.Code, rr.Body.String())
		}
	}
	rr := permissionRequest(t, handler, token, "GET", "/api/servers", "")
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	var listed struct {
		Servers []servers.Server `json:"servers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Servers) != 1 || listed.Servers[0].ID != "one" {
		t.Fatalf("leaked servers: %s", rr.Body.String())
	}
	for _, body := range []string{`{"access":{"role":"administrator"}}`, `{"email":"viewer@example.test","access":{"role":"viewer"}}`} {
		rr = permissionRequest(t, handler, token, "PATCH", "/api/users/"+viewer.ID, body)
		if rr.Code != 403 {
			t.Fatalf("self privilege escalation: %d %s", rr.Code, rr.Body.String())
		}
	}
	rr = permissionRequest(t, handler, token, "PATCH", "/api/users/"+viewer.ID, `{"email":"self@example.test"}`)
	if rr.Code != 200 {
		t.Fatalf("self account update blocked: %s", rr.Body.String())
	}
	// Every server-specific endpoint must reject an inaccessible server even
	// when the caller holds every server capability.
	serverPermissions := []string{}
	for _, p := range auth.PermissionCatalog {
		if p.ServerScoped && p.ID != "servers.copy" {
			serverPermissions = append(serverPermissions, p.ID)
		}
	}
	scoped := auth.Access{Role: "custom", Permissions: serverPermissions, ServerIDs: []string{"one"}}
	if err := users.UpdateAccountAccess(ctx, viewer.ID, nil, nil, "", &scoped); err != nil {
		t.Fatal(err)
	}
	token, err = sessions.Create(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`HandleFunc\("([A-Z]+) (/api/servers/\{id\}[^"]*)"`).FindAllStringSubmatch(string(source), -1) {
		path := strings.NewReplacer("{id}", "two", "{name}", "Steve", "{world}", "world", "{scheduleId}", "schedule", "{backupId}", "backup").Replace(match[2])
		rr = permissionRequest(t, handler, token, match[1], path, `{}`)
		if rr.Code != 403 {
			t.Errorf("scope bypass %s %s: %d %s", match[1], path, rr.Code, rr.Body.String())
		}
	}
	// Operators can inspect ports while editing their assigned server, without
	// learning other servers' names or identifiers.
	rr = permissionRequest(t, handler, token, "GET", "/api/ports/available?server_id=one", "")
	if rr.Code != 200 || strings.Contains(rr.Body.String(), `"server_id":"two"`) || strings.Contains(rr.Body.String(), `"server_name":"two"`) {
		t.Fatalf("port scope: %d %s", rr.Code, rr.Body.String())
	}
	rr = permissionRequest(t, handler, token, "GET", "/api/ports/available?server_id=two", "")
	if rr.Code != 403 {
		t.Fatal("foreign port metadata allowed")
	}
	// Backups are addressed globally; authorization must still use their server.
	_, err = store.DB.Exec(`INSERT INTO backups (id,server_id,name,status,created_at) VALUES ('foreign','two','secret','completed','now')`)
	if err != nil {
		t.Fatal(err)
	}
	access := auth.Access{Role: "custom", Permissions: []string{"servers.view", "backups.download", "backups.delete", "players.manage"}, ServerIDs: []string{"one"}}
	if err := users.UpdateAccountAccess(ctx, viewer.ID, nil, nil, "", &access); err != nil {
		t.Fatal(err)
	}
	token, err = sessions.Create(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"GET /api/backups/foreign/download", "DELETE /api/backups/foreign"} {
		method, path, _ := strings.Cut(route, " ")
		rr = permissionRequest(t, handler, token, method, path, "")
		if rr.Code != 403 {
			t.Fatalf("backup scope bypass: %d %s", rr.Code, rr.Body.String())
		}
	}
	for _, action := range []string{"op", "deop", "custom"} {
		rr = permissionRequest(t, handler, token, "POST", "/api/servers/one/players/Steve/command", `{"action":"`+action+`","args":{"command":"op Steve"}}`)
		if rr.Code != 403 {
			t.Fatalf("player escalation %s: %d %s", action, rr.Code, rr.Body.String())
		}
	}
	updatesOnly := auth.Access{Role: "custom", Permissions: []string{"updates.manage"}}
	if err := users.UpdateAccountAccess(ctx, viewer.ID, nil, nil, "", &updatesOnly); err != nil {
		t.Fatal(err)
	}
	token, err = sessions.Create(ctx, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	rr = permissionRequest(t, handler, token, "GET", "/api/updates", "")
	if rr.Code != 200 {
		t.Fatalf("update manager cannot read settings: %d %s", rr.Code, rr.Body.String())
	}

}

func TestPermissionAccountDefaultsAndRevocation(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	sessions := auth.NewManager(store.DB)
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Create(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	rr := permissionRequest(t, handler, token, "POST", "/api/users", `{"email":"new@example.test","password":"password123"}`)
	if rr.Code != 201 {
		t.Fatal(rr.Body.String())
	}
	var user auth.User
	if err := json.Unmarshal(rr.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.Role != "viewer" || user.AllServers || len(user.ServerIDs) != 0 {
		t.Fatalf("unsafe defaults: %+v", user)
	}
	for _, body := range []string{`{"access":{"role":"typo"}}`, `{"access":{"role":"custom","permissions":["*"]}}`, `{"access":{"role":"viewer","server_ids":["missing"]}}`} {
		rr = permissionRequest(t, handler, token, "PATCH", "/api/users/"+user.ID, body)
		if rr.Code != 400 {
			t.Fatalf("invalid access accepted: %d %s", rr.Code, rr.Body.String())
		}
	}
	rr = permissionRequest(t, handler, token, "PATCH", "/api/users/"+admin.ID, `{"access":{"role":"viewer"}}`)
	if rr.Code != 400 {
		t.Fatalf("last administrator demoted: %d %s", rr.Code, rr.Body.String())
	}
	userToken, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	rr = permissionRequest(t, handler, token, "PATCH", "/api/users/"+user.ID, `{"access":{"role":"viewer","disabled":true}}`)
	if rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	rr = permissionRequest(t, handler, userToken, "GET", "/api/auth/me", "")
	if rr.Code != 401 {
		t.Fatalf("disabled session accepted: %d", rr.Code)
	}
	// Even an otherwise valid token created after disabling cannot bypass auth.
	userToken, err = sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	rr = permissionRequest(t, handler, userToken, "GET", "/api/auth/me", "")
	if rr.Code != 401 {
		t.Fatalf("disabled account accepted: %d", rr.Code)
	}
}

func TestEveryProtectedRouteDeclaresPermission(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	self := []string{"POST /api/auth/logout", "GET /api/auth/me", "GET /api/auth/totp", "POST /api/auth/totp/enroll", "POST /api/auth/totp/enroll/confirm", "POST /api/auth/totp/disable", "POST /api/passkey/register/begin", "POST /api/passkey/register/finish", "GET /api/passkey", "DELETE /api/passkey", "PATCH /api/users/{id}"}
	route := regexp.MustCompile(`HandleFunc\("([^"]+)", s\.(requireAuth|requirePermission)\((?:"([^"]+)", )?`)
	known := map[string]bool{}
	for _, p := range auth.PermissionCatalog {
		known[p.ID] = true
	}
	matches := route.FindAllStringSubmatch(string(source), -1)
	if len(matches) < 100 {
		t.Fatal("route audit did not inspect the API")
	}
	for _, match := range matches {
		if match[2] == "requireAuth" && !slices.Contains(self, match[1]) {
			t.Errorf("route has no capability: %s", match[1])
		}
		if match[2] == "requirePermission" && !known[match[3]] {
			t.Errorf("unknown permission on %s: %s", match[1], match[3])
		}
	}
}

func TestDisabledAccountsCannotAuthenticate(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateWithAccess(ctx, "disabled@example.test", hash, auth.Access{Role: "viewer", Disabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rr := permissionRequest(t, handler, "", "POST", "/api/auth/login", `{"email":"disabled@example.test","password":"password123"}`)
	if rr.Code != 401 {
		t.Fatalf("disabled password login accepted: %d %s", rr.Code, rr.Body.String())
	}
	passkeys := auth.NewPasskeys(store.DB)
	if err := passkeys.Add(ctx, user.ID, "disabled", &webauthn.Credential{ID: []byte("disabled-credential"), PublicKey: []byte("key")}); err != nil {
		t.Fatal(err)
	}
	server := &Server{users: users, passkeys: passkeys}
	if _, err := server.discoverableUserHandler([]byte("disabled-credential"), nil); err == nil {
		t.Fatal("disabled passkey user resolved")
	}
}

func TestPermissionStreamClosesAfterRevocation(t *testing.T) {
	store, _ := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	sessions := auth.NewManager(store.DB)
	user, err := users.CreateWithAccess(ctx, "stream@example.test", "hash", auth.Access{Role: "viewer", ServerIDs: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{users: users, sessions: sessions}
	started := make(chan struct{})
	done := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/servers/{id}/console", server.requirePermission("console.read", func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	req := httptest.NewRequest("GET", "/api/servers/one/console", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	go func() { defer close(done); mux.ServeHTTP(httptest.NewRecorder(), req) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("stream never authorized")
	}
	access := auth.Access{Role: "viewer", Disabled: true}
	if err := users.UpdateAccountAccess(ctx, user.ID, nil, nil, "", &access); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("revoked stream remained open")
	}
}
