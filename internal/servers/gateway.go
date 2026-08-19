package servers

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// GatewayState holds the gateway-related per-server state read from the rows
// added by migration 0007. Fields are populated lazily by the gateway package.
type GatewayState struct {
	WakeMessage     string
	LastMotd        string
	LastMotdUpdated time.Time
}

// GatewayInfo describes a server's gateway configuration and last-known-good
// MOTD, exposed through the API.
type GatewayInfo struct {
	Enabled         bool   `json:"enabled"`
	WakeMessage     string `json:"wake_message"`
	LastMotd        string `json:"last_motd"`
	LastMotdUpdated string `json:"last_motd_updated"`
}

// WakeMessage returns the per-server wait/void message, or an empty string when
// none is set (the global default then applies).
func (s *Store) WakeMessage(ctx context.Context, id string) (string, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT wake_message FROM servers WHERE id = ?`, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if !v.Valid {
		return "", nil
	}
	return v.String, nil
}

// SetWakeMessage records the per-server wait/void message. An empty string
// clears it so the server falls back to the global default.
func (s *Store) SetWakeMessage(ctx context.Context, id, message string) error {
	var val any
	if message == "" {
		val = nil
	} else {
		val = message
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET wake_message = ?, updated_at = ? WHERE id = ?`,
		val, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// LastMotd returns a server's last-known-good MOTD and when it was captured.
func (s *Store) LastMotd(ctx context.Context, id string) (motd string, updated time.Time, err error) {
	var m sql.NullString
	var u sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT last_motd, last_motd_updated FROM servers WHERE id = ?`, id).Scan(&m, &u)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, err
	}
	if m.Valid {
		motd = m.String
	}
	if u.Valid && u.String != "" {
		if t, perr := time.Parse(time.RFC3339, u.String); perr == nil {
			updated = t
		}
	}
	return motd, updated, nil
}

// SetLastMotd records a server's last-known-good MOTD and the capture time.
func (s *Store) SetLastMotd(ctx context.Context, id, motd string) error {
	if motd == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET last_motd = ?, last_motd_updated = ?, updated_at = ? WHERE id = ?`,
		motd, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// GatewayInfo returns a server's gateway configuration and last-known-good
// MOTD for the API. Enabled reflects the effective gateway activation state.
func (s *Store) GatewayInfo(ctx context.Context, id string, enabled bool) (GatewayInfo, error) {
	wake, err := s.WakeMessage(ctx, id)
	if err != nil {
		return GatewayInfo{}, err
	}
	motd, updated, err := s.LastMotd(ctx, id)
	if err != nil {
		return GatewayInfo{}, err
	}
	var updatedStr string
	if !updated.IsZero() {
		updatedStr = updated.Format(time.RFC3339)
	}
	return GatewayInfo{
		Enabled:         enabled,
		WakeMessage:     wake,
		LastMotd:        motd,
		LastMotdUpdated: updatedStr,
	}, nil
}

// GatewayEnabled reads the site-wide gateway_enabled setting. It defaults to
// false when unset.
func (s *Store) GatewayEnabled(ctx context.Context) (bool, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'gateway_enabled'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return v.Valid && v.String == "true", nil
}

// WakeMessageDefault reads the site-wide default wait message, falling back to
// def when unset.
func (s *Store) WakeMessageDefault(ctx context.Context, def string) (string, error) {
	var v sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'wake_message_default'`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil && v.Valid && v.String != "" {
		return v.String, nil
	}
	return def, nil
}
