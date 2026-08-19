package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// CredentialIDString encodes a passkey credential ID for safe storage/transport.
func CredentialIDString(credID []byte) string {
	return base64.RawURLEncoding.EncodeToString(credID)
}

// ParseCredentialIDString decodes a passkey credential ID from its string form.
func ParseCredentialIDString(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// Passkeys provides persistence for WebAuthn credentials keyed by user.
type Passkeys struct {
	db *sql.DB
}

// NewPasskeys returns a Passkeys repository backed by the database.
func NewPasskeys(db *sql.DB) *Passkeys {
	return &Passkeys{db: db}
}

// Add stores a new passkey credential for a user. name is an optional
// human-friendly label shown in the UI.
func (p *Passkeys) Add(ctx context.Context, userID string, name string, cred *webauthn.Credential) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	credID := CredentialIDString(cred.ID)
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO passkeys (id, user_id, name, credential_id, credential, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), userID, name, credID, string(raw), now)
	return err
}

// ListByUser returns every passkey credential owned by the user.
func (p *Passkeys) ListByUser(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT credential FROM passkeys WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []webauthn.Credential
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(raw), &cred); err != nil {
			return nil, err
		}
		creds = append(creds, cred)
	}
	return creds, rows.Err()
}

// GetByCredentialID returns the owning user ID for a passkey credential ID.
func (p *Passkeys) GetByCredentialID(ctx context.Context, credentialID []byte) (string, *webauthn.Credential, error) {
	id := CredentialIDString(credentialID)
	var userID, raw string
	err := p.db.QueryRowContext(ctx,
		`SELECT user_id, credential FROM passkeys WHERE credential_id = ?`, id).Scan(&userID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, sql.ErrNoRows
	}
	if err != nil {
		return "", nil, err
	}
	var cred webauthn.Credential
	if err := json.Unmarshal([]byte(raw), &cred); err != nil {
		return "", nil, err
	}
	return userID, &cred, nil
}

// Update stores a fresh copy of an existing credential, used to persist
// counter and flag updates after a successful login.
func (p *Passkeys) Update(ctx context.Context, userID string, cred *webauthn.Credential) error {
	raw, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	credID := CredentialIDString(cred.ID)
	_, err = p.db.ExecContext(ctx,
		`UPDATE passkeys SET credential = ?, user_id = ? WHERE credential_id = ? AND user_id = ?`,
		string(raw), userID, credID, userID)
	return err
}

// Delete removes a passkey credential from a user.
func (p *Passkeys) Delete(ctx context.Context, userID string, credentialID []byte) error {
	id := CredentialIDString(credentialID)
	_, err := p.db.ExecContext(ctx,
		`DELETE FROM passkeys WHERE credential_id = ? AND user_id = ?`, id, userID)
	return err
}

// CountByUser reports how many passkeys a user owns.
func (p *Passkeys) CountByUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM passkeys WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// WAUser adapts an auth.User and its credentials to the WebAuthn User
// interface used by the library.
type WAUser struct {
	user  User
	creds []webauthn.Credential
}

// NewWAUser builds a WebAuthn user adapter from an account and its passkeys.
func NewWAUser(user User, creds []webauthn.Credential) *WAUser {
	return &WAUser{user: user, creds: creds}
}

// UserID returns the underlying account ID.
func (w *WAUser) UserID() string {
	return w.user.ID
}

// WebAuthnID returns the user handle (the stable user ID).
func (w *WAUser) WebAuthnID() []byte {
	return []byte(w.user.ID)
}

// WebAuthnName returns the account name.
func (w *WAUser) WebAuthnName() string {
	return w.user.Email
}

// WebAuthnDisplayName returns the account display name.
func (w *WAUser) WebAuthnDisplayName() string {
	return w.user.Email
}

// WebAuthnCredentials returns the user's passkey credentials.
func (w *WAUser) WebAuthnCredentials() []webauthn.Credential {
	return w.creds
}
