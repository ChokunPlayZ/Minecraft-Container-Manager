package auth

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/db"
)

func TestUsersListOrderedByCreatedAt(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	users := NewUsers(store.DB)
	ctx := context.Background()
	for _, email := range []string{"a@example.test", "b@example.test", "c@example.test"} {
		if _, err := users.Create(ctx, email, "hash"); err != nil {
			t.Fatalf("create %s: %v", email, err)
		}
	}

	got, err := users.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 users, got %d", len(got))
	}
	for i, email := range []string{"a@example.test", "b@example.test", "c@example.test"} {
		if got[i].Email != email {
			t.Fatalf("list order: index %d = %q, want %q", i, got[i].Email, email)
		}
	}
	for _, u := range got {
		out, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("marshal user: %v", err)
		}
		if bytes.Contains(out, []byte("password_hash")) || bytes.Contains(out, []byte("hash")) {
			t.Fatalf("password hash leaked in marshaled output: %s", out)
		}
	}
}

func TestUpdateEmail(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	users := NewUsers(store.DB)
	ctx := context.Background()
	user, err := users.Create(ctx, "old@example.test", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := users.UpdateEmail(ctx, user.ID, "new@example.test"); err != nil {
		t.Fatalf("update email: %v", err)
	}
	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Email != "new@example.test" {
		t.Fatalf("email = %q, want new@example.test", got.Email)
	}

	// Updating a non-existent user reports ErrNoRows.
	if err := users.UpdateEmail(ctx, "missing", "x@example.test"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows, got %v", err)
	}
}

func TestUpdatePasswordInvalidatesOldHash(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	users := NewUsers(store.DB)
	ctx := context.Background()
	user, err := users.Create(ctx, "pw@example.test", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newHash, err := HashPassword("newpassword")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := users.UpdatePassword(ctx, user.ID, newHash); err != nil {
		t.Fatalf("update password: %v", err)
	}
	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := VerifyPassword("newpassword", got.PasswordHash); err != nil {
		t.Fatalf("new password should verify: %v", err)
	}
	if err := VerifyPassword("oldpassword", got.PasswordHash); !errors.Is(err, ErrMismatch) {
		t.Fatalf("expected mismatch for old password, got %v", err)
	}
}

func TestDeleteRevokesSessionsAndPasskeys(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	users := NewUsers(store.DB)
	sessions := NewManager(store.DB)
	passkeys := NewPasskeys(store.DB)

	user, err := users.Create(ctx, "del@example.test", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	cred := &webauthn.Credential{ID: []byte("cred-id"), PublicKey: []byte("pk")}
	if err := passkeys.Add(ctx, user.ID, "key", cred); err != nil {
		t.Fatalf("add passkey: %v", err)
	}

	if err := users.Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := users.GetByID(ctx, user.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows after delete, got %v", err)
	}
	if _, err := sessions.Validate(ctx, token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected session revoked, got %v", err)
	}
	if n, err := passkeys.CountByUser(ctx, user.ID); err != nil || n != 0 {
		t.Fatalf("expected 0 passkeys after delete, got %d (err %v)", n, err)
	}
}

func TestRevokeByUserExcept(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	users := NewUsers(store.DB)
	sessions := NewManager(store.DB)
	user, err := users.Create(ctx, "keep@example.test", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	tokenA, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session A: %v", err)
	}
	tokenB, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session B: %v", err)
	}

	if err := sessions.RevokeByUserExcept(ctx, user.ID, tokenB); err != nil {
		t.Fatalf("revoke except: %v", err)
	}
	if _, err := sessions.Validate(ctx, tokenA); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected token A revoked, got %v", err)
	}
	if _, err := sessions.Validate(ctx, tokenB); err != nil {
		t.Fatalf("expected token B kept, got %v", err)
	}
}
