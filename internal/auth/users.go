package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNoUsers is returned when no user accounts exist yet.
var ErrNoUsers = errors.New("no users exist")

// ErrUnknownServer is returned when a server access list names a server that
// does not exist.
var ErrUnknownServer = errors.New("unknown server")

// Panel roles. Admins manage everything; members manage only the servers
// assigned to them.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// ValidRole reports whether role is a known panel role.
func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleMember
}

// User is a row in the users table.
type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	TOTPSecret   string `json:"-"`
	TOTPEnabled  bool   `json:"totp_enabled"`
	WebAuthnID   string `json:"-"`
	Role         string `json:"role"`
	// ServerIDs lists the servers a member may manage. It is empty for admins.
	ServerIDs []string `json:"server_ids"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// Users provides user account persistence.
type Users struct {
	db *sql.DB
}

// NewUsers returns a Users repository backed by the database.
func NewUsers(db *sql.DB) *Users {
	return &Users{db: db}
}

// Create inserts a new admin with the given email and argon2id password hash.
func (u *Users) Create(ctx context.Context, email, passwordHash string) (User, error) {
	return u.CreateWithRole(ctx, email, passwordHash, RoleAdmin)
}

// CreateWithRole inserts a new user with the given role.
func (u *Users) CreateWithRole(ctx context.Context, email, passwordHash, role string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	now := time.Now().UTC().Format(time.RFC3339)
	id := uuid.NewString()
	_, err := u.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, totp_secret, totp_enabled, webauthn_id, role, created_at, updated_at) VALUES (?, ?, ?, '', 0, '', ?, ?, ?)`,
		id, email, passwordHash, role, now, now)
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
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), role, created_at, updated_at FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var usr User
		var totpEnabled int
		if err := rows.Scan(&usr.ID, &usr.Email, &usr.PasswordHash, &usr.TOTPSecret, &totpEnabled, &usr.WebAuthnID, &usr.Role, &usr.CreatedAt, &usr.UpdatedAt); err != nil {
			return nil, err
		}
		usr.TOTPEnabled = totpEnabled == 1
		users = append(users, usr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	access, err := u.allServerAccess(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		users[i].ServerIDs = access[users[i].ID]
		if users[i].ServerIDs == nil {
			users[i].ServerIDs = []string{}
		}
	}
	return users, nil
}

// Role returns a user's panel role.
func (u *Users) Role(ctx context.Context, id string) (string, error) {
	var role string
	err := u.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, id).Scan(&role)
	return role, err
}

// UpdateRole changes a user's panel role.
func (u *Users) UpdateRole(ctx context.Context, id, role string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := u.db.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = ? WHERE id = ?`, role, now, id)
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

// HasServerAccess reports whether a member has been assigned a server. It does
// not consider roles; callers grant admins access separately.
func (u *Users) HasServerAccess(ctx context.Context, userID, serverID string) (bool, error) {
	var n int
	err := u.db.QueryRowContext(ctx,
		`SELECT COUNT(1) FROM server_access WHERE user_id = ? AND server_id = ?`, userID, serverID).Scan(&n)
	return n > 0, err
}

// ServerIDs returns the servers assigned to a user.
func (u *Users) ServerIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := u.db.QueryContext(ctx,
		`SELECT server_id FROM server_access WHERE user_id = ? ORDER BY created_at, server_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetServerAccess replaces the servers assigned to a user. It returns
// ErrUnknownServer, and changes nothing, if any id does not name a server.
func (u *Users) SetServerAccess(ctx context.Context, userID string, serverIDs []string) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM server_access WHERE user_id = ?`, userID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, serverID := range serverIDs {
		res, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO server_access (user_id, server_id, created_at) SELECT ?, id, ? FROM servers WHERE id = ?`,
			userID, now, serverID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM servers WHERE id = ?`, serverID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return ErrUnknownServer
			}
		}
	}
	return tx.Commit()
}

func (u *Users) allServerAccess(ctx context.Context) (map[string][]string, error) {
	rows, err := u.db.QueryContext(ctx,
		`SELECT user_id, server_id FROM server_access ORDER BY created_at, server_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	access := map[string][]string{}
	for rows.Next() {
		var userID, serverID string
		if err := rows.Scan(&userID, &serverID); err != nil {
			return nil, err
		}
		access[userID] = append(access[userID], serverID)
	}
	return access, rows.Err()
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM server_access WHERE user_id = ?`, id); err != nil {
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
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), role, created_at, updated_at FROM users WHERE LOWER(email) = ?`, strings.ToLower(strings.TrimSpace(email))))
}

// GetByID returns a user by id, including their assigned servers.
func (u *Users) GetByID(ctx context.Context, id string) (User, error) {
	usr, err := u.scan(u.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(totp_secret,''), COALESCE(totp_enabled,0), COALESCE(webauthn_id,''), role, created_at, updated_at FROM users WHERE id = ?`, id))
	if err != nil {
		return User{}, err
	}
	usr.ServerIDs, err = u.ServerIDs(ctx, id)
	if err != nil {
		return User{}, err
	}
	return usr, nil
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
	err := row.Scan(&usr.ID, &usr.Email, &usr.PasswordHash, &usr.TOTPSecret, &totpEnabled, &usr.WebAuthnID, &usr.Role, &usr.CreatedAt, &usr.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, sql.ErrNoRows
	}
	if err != nil {
		return User{}, err
	}
	usr.TOTPEnabled = totpEnabled == 1
	usr.ServerIDs = []string{}
	return usr, nil
}

// CreateFirst is a single conditional database write: concurrent onboarding
// requests cannot both create the first administrator.
func (u *Users) CreateFirst(ctx context.Context, email, passwordHash string) (User, error) {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := u.db.ExecContext(ctx, `INSERT INTO users (id,email,password_hash,totp_secret,totp_enabled,webauthn_id,created_at,updated_at) SELECT ?,?,?,'',0,'',?,? WHERE NOT EXISTS (SELECT 1 FROM users)`, id, email, passwordHash, now, now)
	if err != nil {
		return User{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if n == 0 {
		return User{}, ErrOnboardingComplete
	}
	return u.GetByID(ctx, id)
}

var ErrOnboardingComplete = errors.New("onboarding already complete")

// UpdateAccount commits identity, password and session revocation together.
func (u *Users) UpdateAccount(ctx context.Context, id string, email, passwordHash *string, keepToken string) error {
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE users SET email=COALESCE(?,email),password_hash=COALESCE(?,password_hash),updated_at=? WHERE id=?`, email, passwordHash, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	if passwordHash != nil {
		if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, id, HashToken(keepToken)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
