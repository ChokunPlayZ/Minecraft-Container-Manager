package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func TestAccessPresetsAndValidation(t *testing.T) {
	for _, role := range RolePresets() {
		access := Access{Role: role.ID, AllServers: true}
		if err := access.Normalize(); err != nil {
			t.Fatal(err)
		}
		if access.Has("users.manage") != (role.ID == "administrator") {
			t.Fatalf("unexpected admin access for %s", role.ID)
		}
	}
	viewer := Access{Role: "viewer", ServerIDs: []string{"one"}}
	if !viewer.Can("console.read", "one") || viewer.Can("console.write", "one") || viewer.Can("console.read", "two") {
		t.Fatal("viewer or scope bypass")
	}
	viewer.Disabled = true
	if viewer.Can("console.read", "one") {
		t.Fatal("disabled viewer allowed")
	}
	for _, access := range []Access{
		{Role: "invalid"}, {Role: "custom", Permissions: []string{"*"}},
		{Role: "custom", Permissions: []string{"servers.create"}},
		{Role: "custom", Permissions: []string{"schedules.manage"}, AllServers: true},
	} {
		if err := access.Normalize(); err == nil {
			t.Fatalf("accepted invalid access: %+v", access)
		}
	}
}

func TestLastAdministratorAndAtomicAccessRevocation(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "permissions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	users := NewUsers(store.DB)
	sessions := NewManager(store.DB)
	admin, err := users.CreateFirst(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if admin.Role != "administrator" || !admin.Can("users.manage", "") {
		t.Fatal("onboarding lost admin")
	}
	for _, access := range []Access{{Role: "viewer"}, {Role: "administrator", Disabled: true}} {
		if err := users.UpdateAccountAccess(ctx, admin.ID, nil, nil, "", &access); !IsLastAdministrator(err) {
			t.Fatalf("last admin: %v", err)
		}
	}
	if err := users.Delete(ctx, admin.ID); !IsLastAdministrator(err) {
		t.Fatalf("delete last admin: %v", err)
	}
	user, err := users.CreateWithAccess(ctx, "viewer@example.test", "hash", Access{Role: "viewer", AllServers: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`CREATE TRIGGER fail_access_revoke BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'revoke failed'); END`); err != nil {
		t.Fatal(err)
	}
	access := Access{Role: "operator", AllServers: true}
	email := "changed@example.test"
	if err := users.UpdateAccountAccess(ctx, user.ID, &email, nil, "", &access); err == nil {
		t.Fatal("ignored revocation failure")
	}
	got, err := users.GetByID(ctx, user.ID)
	if err != nil || got.Role != "viewer" || got.Email != user.Email {
		t.Fatalf("partial access change: %+v %v", got, err)
	}
	if _, err := store.DB.Exec(`DROP TRIGGER fail_access_revoke`); err != nil {
		t.Fatal(err)
	}
	if err := users.UpdateAccountAccess(ctx, user.ID, nil, nil, "", &access); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Validate(ctx, token); err == nil {
		t.Fatal("session survived access change")
	}
	fresh, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	access.Disabled = true
	if err := users.UpdateAccountAccess(ctx, user.ID, nil, nil, fresh, &access); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Validate(ctx, fresh); err == nil {
		t.Fatal("disabled account retained acting session")
	}
}

func TestConcurrentDemotionsKeepAnAdministrator(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "concurrent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	users := NewUsers(store.DB)
	first, err := users.Create(ctx, "first@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	second, err := users.Create(ctx, "second@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{first.ID, second.ID} {
		go func(id string) {
			<-start
			access := Access{Role: "viewer"}
			results <- users.UpdateAccountAccess(ctx, id, nil, nil, "", &access)
		}(id)
	}
	close(start)
	successes, protected := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if IsLastAdministrator(err) {
			protected++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || protected != 1 {
		t.Fatalf("demotions: success=%d protected=%d", successes, protected)
	}
}
