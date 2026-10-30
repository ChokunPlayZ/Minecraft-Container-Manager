package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestBackupRetentionAPIAndPermissions(t *testing.T) {
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
	operator, err := users.CreateWithAccess(ctx, "operator@example.test", "hash", auth.Access{Role: "operator", ServerIDs: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := users.CreateWithAccess(ctx, "manager@example.test", "hash", auth.Access{Role: "custom", Permissions: []string{"servers.view", "backups.read", "backups.delete"}, ServerIDs: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	backupStore := backups.New(store.DB, backups.S3Config{}, t.TempDir())
	handler := New(Options{DB: store, Users: users, Sessions: sessions, Servers: servers.NewStore(store, nil, nil, 25565, 25575, t.TempDir(), ""), Backups: backupStore})
	token := mustSession(t, store, admin.ID)
	body := `{"keep_count":3,"max_age_days":14,"trash_retention_days":7}`
	for _, invalid := range []string{`{}`, `{"keep_count":1}`, `{"keep_count":null,"max_age_days":0,"trash_retention_days":30}`, `{"keep_count":1.5,"max_age_days":0,"trash_retention_days":30}`, `{"keep_count":-1,"max_age_days":0,"trash_retention_days":30}`} {
		rr := permissionRequest(t, handler, token, "PUT", "/api/servers/one/backups/retention", invalid)
		if rr.Code != 400 {
			t.Fatalf("accepted %s: %d %s", invalid, rr.Code, rr.Body.String())
		}
	}
	rr := permissionRequest(t, handler, token, "PUT", "/api/servers/one/backups/retention", body)
	if rr.Code != 200 {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var preview backups.RetentionPreview
	if err := json.Unmarshal(rr.Body.Bytes(), &preview); err != nil || preview.Policy.KeepCount != 3 || preview.ToTrash == nil || preview.ToPurge == nil {
		t.Fatalf("invalid response: %s %v", rr.Body.String(), err)
	}
	for _, id := range []string{"one", "two"} {
		_, err := store.DB.Exec(`INSERT INTO backups(id,server_id,name,status,location,created_at) VALUES(?,?,?,'completed','local:test','2026-10-30T00:00:00Z')`, "backup-"+id, id, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	mutations := []string{"PUT /api/servers/one/backups/retention", "POST /api/servers/one/backups/cleanup", "DELETE /api/servers/one/backups/trash", "PATCH /api/backups/backup-one/protect", "POST /api/backups/backup-one/recover", "DELETE /api/backups/backup-one/purge"}
	for _, route := range mutations {
		method, path, _ := strings.Cut(route, " ")
		rr := permissionRequest(t, handler, mustSession(t, store, operator.ID), method, path, body)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("operator retention escalation: %s: %d %s", route, rr.Code, rr.Body.String())
		}
		foreign := strings.ReplaceAll(strings.ReplaceAll(path, "/one/", "/two/"), "backup-one", "backup-two")
		rr = permissionRequest(t, handler, mustSession(t, store, manager.ID), method, foreign, body)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("retention scope leak: %s: %d %s", foreign, rr.Code, rr.Body.String())
		}
	}
	if rr := permissionRequest(t, handler, mustSession(t, store, operator.ID), "GET", "/api/servers/one/backups/retention", ""); rr.Code != 200 {
		t.Fatal("operator cannot read retention", rr.Body.String())
	}
	managerToken := mustSession(t, store, manager.ID)
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"PATCH", "/api/backups/backup-one/protect", `{}`, 400},
		{"PATCH", "/api/backups/backup-one/protect", `{"protected":true}`, 200},
		{"DELETE", "/api/backups/backup-one", "", 409},
		{"PATCH", "/api/backups/backup-one/protect", `{"protected":false}`, 200},
		{"DELETE", "/api/backups/backup-one", "", 200},
		{"POST", "/api/backups/backup-one/recover", "", 200},
		{"PATCH", "/api/backups/backup-one/protect", `{"protected":false}`, 200},
		{"DELETE", "/api/backups/backup-one", "", 200},
	} {
		rr := permissionRequest(t, handler, managerToken, tc.method, tc.path, tc.body)
		if rr.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rr.Code, rr.Body.String())
		}
	}
	rr = permissionRequest(t, handler, managerToken, "GET", "/api/servers/one/backups", "")
	var list struct {
		Backups, Trash []backups.Backup
		Policy         backups.Policy
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil || len(list.Backups) != 0 || len(list.Trash) != 1 || list.Policy.KeepCount != 3 {
		t.Fatalf("list/trash separation: %s %v", rr.Body.String(), err)
	}
	rr = permissionRequest(t, handler, managerToken, "DELETE", "/api/servers/one/backups/trash", "")
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"purged":1`) {
		t.Fatalf("empty trash failed: %d %s", rr.Code, rr.Body.String())
	}
}
