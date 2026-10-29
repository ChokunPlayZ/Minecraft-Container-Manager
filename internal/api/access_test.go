package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

type accessFixture struct {
	t        *testing.T
	store    *db.Store
	handler  http.Handler
	users    *auth.Users
	admin    auth.User
	member   auth.User
	assigned servers.Server
	other    servers.Server
}

// newAccessFixture seeds an admin, a member assigned to one of two servers,
// and the API handler serving them.
func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	users := auth.NewUsers(store.DB)
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	member, err := users.CreateWithRole(ctx, "member@example.test", "hash", auth.RoleMember)
	if err != nil {
		t.Fatalf("create member: %v", err)
	}

	serverStore := servers.NewStore(store, nil, jars.NewResolver(), 25565, 25575, dir, "")
	create := func(name string) servers.Server {
		srv, err := serverStore.Create(ctx, servers.CreateInput{Name: name, ServerType: jars.TypeCustom, Version: "1.0", RAMMB: 1024})
		if err != nil {
			t.Fatalf("create server %s: %v", name, err)
		}
		return srv
	}
	assigned, other := create("Assigned"), create("Other")
	if err := users.SetServerAccess(ctx, member.ID, []string{assigned.ID}); err != nil {
		t.Fatalf("assign server: %v", err)
	}

	handler := New(Options{
		DB:       store,
		Users:    users,
		Sessions: auth.NewManager(store.DB),
		Passkeys: auth.NewPasskeys(store.DB),
		Servers:  serverStore,
		Backups:  backups.New(store.DB, backups.S3Config{}, dir, filepath.Join(dir, "backups")),
	})
	return &accessFixture{t: t, store: store, handler: handler, users: users, admin: admin, member: member, assigned: assigned, other: other}
}

func (f *accessFixture) do(user auth.User, method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(f.t, f.store, user.ID)})
	if method != http.MethodGet {
		csrf := mustCSRF(f.t)
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
		req.Header.Set(csrfHeaderName, csrf)
	}
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)
	return rr
}

func (f *accessFixture) expect(rr *httptest.ResponseRecorder, want int) {
	f.t.Helper()
	if rr.Code != want {
		f.t.Fatalf("status = %d, want %d; body=%s", rr.Code, want, rr.Body.String())
	}
}

func TestMemberSeesOnlyAssignedServers(t *testing.T) {
	f := newAccessFixture(t)

	rr := f.do(f.member, http.MethodGet, "/api/servers", "")
	f.expect(rr, http.StatusOK)
	var list struct {
		Servers []servers.Server `json:"servers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list.Servers) != 1 || list.Servers[0].ID != f.assigned.ID {
		t.Fatalf("member servers = %+v, want only %s", list.Servers, f.assigned.ID)
	}

	f.expect(f.do(f.member, http.MethodGet, "/api/servers/"+f.assigned.ID, ""), http.StatusOK)
	// Unassigned servers look nonexistent to members.
	f.expect(f.do(f.member, http.MethodGet, "/api/servers/"+f.other.ID, ""), http.StatusNotFound)
	f.expect(f.do(f.member, http.MethodGet, "/api/servers/"+f.other.ID+"/files", ""), http.StatusNotFound)
	f.expect(f.do(f.member, http.MethodPost, "/api/servers/"+f.other.ID+"/start", ""), http.StatusNotFound)

	// Admins still see every server.
	f.expect(f.do(f.admin, http.MethodGet, "/api/servers/"+f.other.ID, ""), http.StatusOK)
}

func TestMemberCannotUseAdminRoutes(t *testing.T) {
	f := newAccessFixture(t)
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/servers", `{"name":"x","server_type":"custom","version":"1.0","ram_mb":1024}`},
		{http.MethodDelete, "/api/servers/" + f.assigned.ID, ""},
		{http.MethodPost, "/api/servers/" + f.assigned.ID + "/copy", `{"name":"copy"}`},
		{http.MethodPost, "/api/servers/" + f.assigned.ID + "/dns", `{}`},
		{http.MethodGet, "/api/users", ""},
		{http.MethodPatch, "/api/users/" + f.member.ID, `{"role":"admin"}`},
		{http.MethodDelete, "/api/users/" + f.admin.ID, ""},
		{http.MethodGet, "/api/settings", ""},
		{http.MethodGet, "/api/ports/available", ""},
		{http.MethodGet, "/api/system/capacity", ""},
	} {
		rr := f.do(f.member, tc.method, tc.path, tc.body)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want 403; body=%s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
	if role, _ := f.users.Role(context.Background(), f.member.ID); role != auth.RoleMember {
		t.Fatalf("member role = %q after denied self-promotion", role)
	}
	// Members keep access to their own account.
	f.expect(f.do(f.member, http.MethodGet, "/api/auth/me", ""), http.StatusOK)
	f.expect(f.do(f.member, http.MethodPatch, "/api/users/"+f.member.ID, `{"email":"me@example.test"}`), http.StatusOK)
	f.expect(f.do(f.member, http.MethodPatch, "/api/users/"+f.admin.ID, `{"email":"taken@example.test"}`), http.StatusForbidden)
	f.expect(f.do(f.member, http.MethodPatch, "/api/users/"+f.member.ID, `{"server_ids":[]}`), http.StatusForbidden)
}

func TestMemberCannotChangeServerResources(t *testing.T) {
	f := newAccessFixture(t)
	path := "/api/servers/" + f.assigned.ID

	// The settings form resends unchanged resources alongside other edits.
	f.expect(f.do(f.member, http.MethodPatch, path, `{"name":"Renamed","ram_mb":1024,"extra_ports":[]}`), http.StatusOK)

	for _, body := range []string{`{"ram_mb":4096}`, `{"cpu_limit":2}`, `{"host_port":25570}`,
		`{"extra_ports":[{"id":"a","host_port":25571,"container_port":8100,"protocol":"tcp"}]}`} {
		rr := f.do(f.member, http.MethodPatch, path, body)
		if rr.Code != http.StatusForbidden {
			t.Errorf("PATCH %s status = %d, want 403", body, rr.Code)
		}
	}
	f.expect(f.do(f.admin, http.MethodPatch, path, `{"ram_mb":4096}`), http.StatusOK)
}

func TestBackupRoutesCheckOwningServer(t *testing.T) {
	f := newAccessFixture(t)
	if _, err := f.store.DB.Exec(`INSERT INTO backups (id, server_id, name, status, created_at) VALUES ('b-other', ?, 'other', 'completed', '2026-01-01T00:00:00Z')`, f.other.ID); err != nil {
		t.Fatalf("seed backup: %v", err)
	}

	f.expect(f.do(f.member, http.MethodDelete, "/api/backups/b-other", ""), http.StatusNotFound)
	f.expect(f.do(f.member, http.MethodGet, "/api/backups/b-other/download", ""), http.StatusNotFound)
	// A backup can only be restored through the server that owns it.
	f.expect(f.do(f.member, http.MethodPost, "/api/servers/"+f.assigned.ID+"/restore/b-other", ""), http.StatusBadRequest)
	f.expect(f.do(f.admin, http.MethodPost, "/api/servers/"+f.assigned.ID+"/restore/b-other", ""), http.StatusBadRequest)

	var n int
	if err := f.store.DB.QueryRow(`SELECT COUNT(1) FROM backups WHERE id = 'b-other'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("backup should remain, count=%d err=%v", n, err)
	}
}

func TestAdminAssignsServersAndRoles(t *testing.T) {
	f := newAccessFixture(t)
	ctx := context.Background()

	rr := f.do(f.admin, http.MethodPost, "/api/users",
		`{"email":"new@example.test","password":"password123","role":"member","server_ids":["`+f.other.ID+`"]}`)
	f.expect(rr, http.StatusCreated)
	var created auth.User
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Role != auth.RoleMember || len(created.ServerIDs) != 1 || created.ServerIDs[0] != f.other.ID {
		t.Fatalf("created = %+v", created)
	}

	// Unknown servers are rejected without changing existing access.
	f.expect(f.do(f.admin, http.MethodPatch, "/api/users/"+f.member.ID, `{"server_ids":["missing"]}`), http.StatusBadRequest)
	if ids, _ := f.users.ServerIDs(ctx, f.member.ID); len(ids) != 1 || ids[0] != f.assigned.ID {
		t.Fatalf("access after rejected update = %v", ids)
	}

	f.expect(f.do(f.admin, http.MethodPatch, "/api/users/"+f.member.ID,
		`{"server_ids":["`+f.assigned.ID+`","`+f.other.ID+`"]}`), http.StatusOK)
	f.expect(f.do(f.member, http.MethodGet, "/api/servers/"+f.other.ID, ""), http.StatusOK)

	// Promotion grants everything and drops the now-redundant assignments.
	f.expect(f.do(f.admin, http.MethodPatch, "/api/users/"+f.member.ID, `{"role":"admin"}`), http.StatusOK)
	if ids, _ := f.users.ServerIDs(ctx, f.member.ID); len(ids) != 0 {
		t.Fatalf("admin kept assignments %v", ids)
	}
	f.expect(f.do(f.member, http.MethodGet, "/api/users", ""), http.StatusOK)

	f.expect(f.do(f.admin, http.MethodPatch, "/api/users/"+f.admin.ID, `{"role":"member"}`), http.StatusBadRequest)
	f.expect(f.do(f.admin, http.MethodPatch, "/api/users/"+f.member.ID, `{"role":"owner"}`), http.StatusBadRequest)
}

func TestServerDeletionRemovesAccess(t *testing.T) {
	f := newAccessFixture(t)
	f.expect(f.do(f.admin, http.MethodDelete, "/api/servers/"+f.assigned.ID, ""), http.StatusOK)
	if ids, _ := f.users.ServerIDs(context.Background(), f.member.ID); len(ids) != 0 {
		t.Fatalf("access to deleted server remains: %v", ids)
	}
}
