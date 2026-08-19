package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func TestSessionLifecycle(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	users := NewUsers(store.DB)
	mgr := NewManager(store.DB)
	ctx := context.Background()

	user, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	token, err := mgr.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("expected 64-char token, got %d", len(token))
	}

	got, err := mgr.Validate(ctx, token)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if got != user.ID {
		t.Fatalf("expected user %s, got %s", user.ID, got)
	}

	// A garbage token must not validate.
	if _, err := mgr.Validate(ctx, "garbage-token"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound for garbage token, got %v", err)
	}

	// Deleting the session invalidates the token.
	if err := mgr.Delete(ctx, token); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := mgr.Validate(ctx, token); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after delete, got %v", err)
	}
}

func TestSessionStoresHashNotToken(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	users := NewUsers(store.DB)
	mgr := NewManager(store.DB)
	ctx := context.Background()

	user, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	token, err := mgr.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var storedHash string
	if err := store.DB.QueryRowContext(ctx, `SELECT token_hash FROM sessions`).Scan(&storedHash); err != nil {
		t.Fatalf("read stored hash: %v", err)
	}
	if storedHash != HashToken(token) {
		t.Fatalf("expected stored hash %s, got %s", HashToken(token), storedHash)
	}
	if storedHash == token {
		t.Fatal("token stored in plaintext")
	}
}
