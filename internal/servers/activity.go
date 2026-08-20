package servers

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// SpinState holds the spin-down related per-server state read from the rows
// added by migration 0004. Fields are populated lazily by the spindown package.
type SpinState struct {
	LastActivity       time.Time
	HasLastActivity    bool
	IdleTimeoutMinutes *int
}

// LastActivity returns the last recorded player-activity time for a server, or
// a zero time when none has been recorded yet.
func (s *Store) LastActivity(ctx context.Context, id string) (time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT last_activity FROM servers WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, err
	}
	if !raw.Valid || raw.String == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw.String)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// SetActivity records the last player-activity time for a server.
func (s *Store) SetActivity(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE servers SET last_activity = ?, updated_at = ? WHERE id = ?`,
		t.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// HasPlayers reports whether any players are currently connected to a server.
func (s *Store) HasPlayers(ctx context.Context, id string) (bool, error) {
	res, err := s.PlayerList(ctx, id)
	if err != nil {
		return false, err
	}
	return len(res.Players) > 0, nil
}

// IdleTimeoutOverride returns the per-server idle timeout override in minutes
// and whether one is configured. A cleared (NULL) value reports ok == false so
// callers fall back to the global default.
func (s *Store) IdleTimeoutOverride(ctx context.Context, id string) (minutes int, ok bool, err error) {
	var v sql.NullInt64
	err = s.db.QueryRowContext(ctx, `SELECT idle_timeout_minutes FROM servers WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, ErrNotFound
	}
	if err != nil {
		return 0, false, err
	}
	if !v.Valid {
		return 0, false, nil
	}
	return int(v.Int64), true, nil
}

// SetIdleTimeoutOverride records the per-server idle timeout override in
// minutes. A nil or non-positive value clears the override so the server falls
// back to the global default.
func (s *Store) SetIdleTimeoutOverride(ctx context.Context, id string, minutes *int) error {
	if minutes == nil || *minutes <= 0 {
		_, err := s.db.ExecContext(ctx,
			`UPDATE servers SET idle_timeout_minutes = NULL, updated_at = ? WHERE id = ?`,
			time.Now().UTC().Format(time.RFC3339), id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET idle_timeout_minutes = ?, updated_at = ? WHERE id = ?`,
		*minutes, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// DefaultIdleTimeout reads the site-wide idle timeout from the settings table
// (key idle_timeout_minutes), falling back to fallback when unset or invalid.
func (s *Store) DefaultIdleTimeout(ctx context.Context, fallback time.Duration) (time.Duration, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'idle_timeout_minutes'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil && v.Valid && v.String != "" {
		if mins, perr := strconv.Atoi(v.String); perr == nil && mins > 0 {
			return time.Duration(mins) * time.Minute, nil
		}
	}
	return fallback, nil
}
