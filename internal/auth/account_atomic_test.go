package auth

import (
	"context"
	"github.com/mcm-panel/mcm/internal/db"
	"path/filepath"
	"testing"
)

func TestAccountAndSessionChangesRollbackTogether(t *testing.T) {
	handle, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	ctx := context.Background()
	users := NewUsers(handle.DB)
	user, err := users.Create(ctx, "old@example.test", "old-hash")
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewManager(handle.DB)
	if _, err = sessions.Create(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = handle.DB.Exec(`CREATE TRIGGER fail_revoke BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'revocation failed'); END`); err != nil {
		t.Fatal(err)
	}
	email, hash := "new@example.test", "new-hash"
	if err = users.UpdateAccount(ctx, user.ID, &email, &hash, ""); err == nil {
		t.Fatal("session revocation failure ignored")
	}
	got, err := users.GetByID(ctx, user.ID)
	if err != nil || got.Email != user.Email || got.PasswordHash != user.PasswordHash {
		t.Fatalf("partial account update: %+v %v", got, err)
	}
}
