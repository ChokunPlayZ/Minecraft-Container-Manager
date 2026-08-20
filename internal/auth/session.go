package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CookieName is the session cookie name used by the API layer.
const CookieName = "mcm_session"

// SessionLifetime is the cookie/session time-to-live.
const SessionLifetime = 7 * 24 * time.Hour

// ErrSessionNotFound is returned when a token does not map to a valid session.
var ErrSessionNotFound = errors.New("session not found")

// Session is a row in the sessions table.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Manager creates and validates sessions against the shared SQLite store.
type Manager struct {
	db *sql.DB
}

// NewManager returns a SessionManager backed by the given database.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Create issues a new 32-byte random session token and stores its SHA-256 hash.
// It returns the raw token, which the caller should present to the client.
func (m *Manager) Create(ctx context.Context, userID string) (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	raw := hex.EncodeToString(token)
	hash := HashToken(raw)
	id := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(SessionLifetime)

	_, err := m.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, userID, hash, expires.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("insert session: %w", err)
	}
	return raw, nil
}

// Validate looks up a session by its raw token hash, checks expiry, and returns
// the owning user's ID.
func (m *Manager) Validate(ctx context.Context, token string) (string, error) {
	hash := HashToken(token)
	var userID, expiresAt string
	err := m.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at FROM sessions WHERE token_hash = ?`, hash).Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", err
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return "", fmt.Errorf("parse session expiry: %w", err)
	}
	if time.Now().UTC().After(exp) {
		return "", ErrSessionNotFound
	}
	return userID, nil
}

// Delete removes the session associated with the raw token, if any.
func (m *Manager) Delete(ctx context.Context, token string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, HashToken(token))
	return err
}

// RevokeByUser deletes every session owned by the given user. This is used when
// a user is deleted or their password changes.
func (m *Manager) RevokeByUser(ctx context.Context, userID string) error {
	_, err := m.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// RevokeByUserExcept deletes a user's sessions except the one identified by
// keepToken. This preserves the acting admin's own session when they change
// their own password.
func (m *Manager) RevokeByUserExcept(ctx context.Context, userID, keepToken string) error {
	_, err := m.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, HashToken(keepToken))
	return err
}

// HashToken returns the lowercase hex SHA-256 digest of a session token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
