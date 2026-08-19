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
		`INSERT INTO users (id, email, password_hash, totp_secret, webauthn_id, created_at, updated_at) VALUES (?, ?, ?, '', '', ?, ?)`,
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

// GetByEmail returns a user by email address.
func (u *Users) GetByEmail(ctx context.Context, email string) (User, error) {
	return u.scan(u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(webauthn_id,''), created_at, updated_at FROM users WHERE email = ?`, email))
}

// GetByID returns a user by id.
func (u *Users) GetByID(ctx context.Context, id string) (User, error) {
	return u.scan(u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(webauthn_id,''), created_at, updated_at FROM users WHERE id = ?`, id))
}

func (u *Users) scan(row *sql.Row) (User, error) {
	var usr User
	err := row.Scan(&usr.ID, &usr.Email, &usr.PasswordHash, &usr.TOTPSecret, &usr.WebAuthnID, &usr.CreatedAt, &usr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, sql.ErrNoRows
	}
	if err != nil {
		return User{}, err
	}
	return usr, nil
}
