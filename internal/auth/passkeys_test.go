package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/db"
)

func TestPasskeyRepoLifecycle(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	users := NewUsers(store.DB)
	passkeys := NewPasskeys(store.DB)

	user, err := users.Create(ctx, "key@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	cred := &webauthn.Credential{
		ID:                []byte("credential-id-bytes"),
		PublicKey:         []byte("public-key-bytes"),
		AttestationType:   "none",
		AttestationFormat: "none",
	}
	if err := passkeys.Add(ctx, user.ID, "Laptop", cred); err != nil {
		t.Fatalf("add passkey: %v", err)
	}

	creds, err := passkeys.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list passkeys: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 passkey, got %d", len(creds))
	}
	if string(creds[0].ID) != "credential-id-bytes" {
		t.Fatalf("unexpected credential id: %s", creds[0].ID)
	}

	owner, stored, err := passkeys.GetByCredentialID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("get by credential id: %v", err)
	}
	if owner != user.ID {
		t.Fatalf("expected owner %s, got %s", user.ID, owner)
	}
	if string(stored.ID) != "credential-id-bytes" {
		t.Fatalf("unexpected stored id: %s", stored.ID)
	}

	// Update then re-read (simulates counter update after a login).
	stored.Authenticator.SignCount = 7
	if err := passkeys.Update(ctx, user.ID, stored); err != nil {
		t.Fatalf("update passkey: %v", err)
	}
	_, reread, err := passkeys.GetByCredentialID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("re-read passkey: %v", err)
	}
	if reread.Authenticator.SignCount != 7 {
		t.Fatalf("expected counter 7, got %d", reread.Authenticator.SignCount)
	}

	if err := passkeys.Delete(ctx, user.ID, cred.ID); err != nil {
		t.Fatalf("delete passkey: %v", err)
	}
	if n, err := passkeys.CountByUser(ctx, user.ID); err != nil || n != 0 {
		t.Fatalf("expected 0 passkeys after delete, got %d (err %v)", n, err)
	}
}

func TestUserTOTPFlow(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	users := NewUsers(store.DB)
	user, err := users.Create(ctx, "totp@example.test", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.TOTPEnabled {
		t.Fatal("expected totp disabled by default")
	}

	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if err := users.SetTOTPSecret(ctx, user.ID, secret); err != nil {
		t.Fatalf("set secret: %v", err)
	}

	// Secret should be storable but not yet enabled.
	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.TOTPSecret != secret {
		t.Fatalf("expected secret %q, got %q", secret, got.TOTPSecret)
	}
	if got.TOTPEnabled {
		t.Fatal("expected totp still disabled")
	}

	if err := users.EnableTOTP(ctx, user.ID); err != nil {
		t.Fatalf("enable totp: %v", err)
	}
	enabled, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !enabled.TOTPEnabled {
		t.Fatal("expected totp enabled")
	}

	if err := users.DisableTOTP(ctx, user.ID); err != nil {
		t.Fatalf("disable totp: %v", err)
	}
	disabled, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if disabled.TOTPEnabled || disabled.TOTPSecret != "" {
		t.Fatalf("expected totp disabled and secret cleared, got enabled=%v secret=%q",
			disabled.TOTPEnabled, disabled.TOTPSecret)
	}
}
