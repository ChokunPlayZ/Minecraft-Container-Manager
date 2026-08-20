package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNoUsers is returned when no user accounts exist yet.
var ErrNoUsers = errors.New("no users exist")

// User is a row in the users table.
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
	TOTPEnabled  bool   `json:"totp_enabled"`
	WebAuthnID   string `json:"-"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Users provides user account persistence.
type Users struct {
	db *sql.DB
}

// NewUsers returns a Users repository backed by the database.
func NewUsers(db *sql.DB) *Users {
	return &Users{db: db}
}

// Create inserts a new user with the given email and argon2id password hash.
func (u *Users) Create(ctx context.Context, email, passwordHash string) (User, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	_, err := u.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, totp_secret, totp_enabled, webauthn_id, created_at, updated_at) VALUES (?, ?, ?, '', 0, '', ?, ?)`,
		id, email, passwordHash, now, now)
	if err != nil {
		return User{}, err
	}
	return u.GetByID(ctx, id)
}

// Count reports how many users exist.
func (u *Users) Count(ctx context.Context) (int, error) {
	var n int
	err := u.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users`).Scan(&n)
	return n, err
}

// List returns every user, ordered by creation time. Password hashes and other
// sensitive columns are excluded from the marshaled representation by the User
// struct's json tags.
func (u *Users) List(ctx context.Context) ([]User, error) {
	rows, err := u.db.QueryContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), created_at, updated_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var usr User
		var totpEnabled int
		if err := rows.Scan(&usr.ID, &usr.Email, &usr.PasswordHash, &usr.TOTPSecret, &totpEnabled, &usr.WebAuthnID, &usr.CreatedAt, &usr.UpdatedAt); err != nil {
			return nil, err
		}
		usr.TOTPEnabled = totpEnabled == 1
		users = append(users, usr)
	}
	return users, rows.Err()
}

// ErrEmailTaken is returned when an email update would violate the unique
// constraint on users.email.
var ErrEmailTaken = errors.New("email already in use")

// UpdateEmail changes a user's email address.
func (u *Users) UpdateEmail(ctx context.Context, id, email string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := u.db.ExecContext(ctx,
		`UPDATE users SET email = ?, updated_at = ? WHERE id = ?`, email, now, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdatePassword stores a new argon2id password hash for a user.
func (u *Users) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := u.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, passwordHash, now, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete removes a user along with their passkeys and sessions.
func (u *Users) Delete(ctx context.Context, id string) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM passkeys WHERE user_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// GetByEmail returns a user by email address.
func (u *Users) GetByEmail(ctx context.Context, email string) (User, error) {
	return u.scan(u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), created_at, updated_at FROM users WHERE email = ?`, email))
}

// GetByID returns a user by id.
func (u *Users) GetByID(ctx context.Context, id string) (User, error) {
	return u.scan(u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), created_at, updated_at FROM users WHERE id = ?`, id))
}

// SetTOTPSecret stores the base32 TOTP secret for a user without marking it
// enabled. Enrollment is enabled only once the code has been confirmed.
func (u *Users) SetTOTPSecret(ctx context.Context, userID, secret string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, updated_at = ? WHERE id = ?`, secret, now, userID)
	return err
}

// EnableTOTP marks TOTP as enabled for a user.
func (u *Users) EnableTOTP(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, updated_at = ? WHERE id = ?`, now, userID)
	return err
}

// DisableTOTP clears the TOTP secret and disabled flag for a user.
func (u *Users) DisableTOTP(ctx context.Context, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := u.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = '', totp_enabled = 0, updated_at = ? WHERE id = ?`, now, userID)
	return err
}

func (u *Users) scan(row *sql.Row) (User, error) {
	var usr User
	var totpEnabled int
	err := row.Scan(&usr.ID, &usr.Email, &usr.PasswordHash, &usr.TOTPSecret, &totpEnabled, &usr.WebAuthnID, &usr.CreatedAt, &usr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, sql.ErrNoRows
	}
	if err != nil {
		return User{}, err
	}
	usr.TOTPEnabled = totpEnabled == 1
	return usr, nil
}
