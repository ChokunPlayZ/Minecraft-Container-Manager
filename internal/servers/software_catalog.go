package servers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mcm-panel/mcm/internal/jars"
)

// rememberSoftwareCatalog keeps discoveries independently of fleet scans and
// server configuration. An older response cannot erase a newer known release.
// An empty version identifies a list of game versions; otherwise values are
// builds belonging only to that version.
func (s *Store) rememberSoftwareCatalog(ctx context.Context, kind, version string, values []string) error {
	latest := newestStable(values)
	if latest == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT latest FROM software_catalog WHERE server_type=? AND version=?`, kind, version).Scan(&previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if previous != "" && jars.CompareVersionTokens(latest, previous) <= 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO software_catalog (server_type,version,latest,observed_at) VALUES (?,?,?,?)
		ON CONFLICT(server_type,version) DO UPDATE SET latest=excluded.latest, observed_at=excluded.observed_at`, kind, version, latest, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// applySoftwareCatalog compares saved discoveries with the current fleet without
// contacting upstreams. New or reconfigured servers share the same knowledge.
func (s *Store) applySoftwareCatalog(ctx context.Context, status *UpdateCheckerStatus) error {
	rows, err := s.db.QueryContext(ctx, `SELECT s.id,s.server_type,s.version,COALESCE(s.build,''),c.version,c.latest,c.observed_at
		FROM servers s JOIN software_catalog c ON c.server_type=s.server_type AND (c.version='' OR c.version=s.version)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind, version, build, scope, latest, observedAt string
		if err := rows.Scan(&id, &kind, &version, &build, &scope, &latest, &observedAt); err != nil {
			return err
		}
		if version == "" || version == "custom" || (version == "latest" && kind != "bungeecord") {
			continue
		}
		software := &SoftwareUpdate{Supported: true, LatestVersion: version}
		if scope == "" {
			if jars.CompareVersionTokens(latest, version) > 0 {
				software.LatestVersion = latest
				software.Available = true
			}
		} else {
			if build == "" || build == "latest" || build == "custom" {
				continue
			}
			software.LatestBuild = latest
			software.Available = jars.CompareVersionTokens(latest, build) > 0
		}
		result := status.Servers[id]
		old := result.Software
		if old == nil || !old.Supported || jars.CompareVersionTokens(software.LatestVersion, old.LatestVersion) > 0 ||
			(software.LatestVersion == old.LatestVersion && jars.CompareVersionTokens(software.LatestBuild, old.LatestBuild) > 0) {
			result.Software = software
			if result.CheckedAt == "" || observedAt > result.CheckedAt {
				result.CheckedAt = observedAt
			}
			status.Servers[id] = result
		}
	}
	return rows.Err()
}
