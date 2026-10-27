package servers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/checkcache"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/schedules"
)

const UpdateCheckerEnabledKey = "update_checker_enabled"

var ErrUpdateCheckerDisabled = errors.New("enable automatic update checks first")

type SoftwareUpdate struct {
	Supported     bool   `json:"supported"`
	Available     bool   `json:"available"`
	LatestVersion string `json:"latest_version,omitempty"`
	LatestBuild   string `json:"latest_build,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ServerUpdateCheck struct {
	Software  *SoftwareUpdate           `json:"software,omitempty"`
	Mods      *ServerModUpdatesResponse `json:"mods,omitempty"`
	CheckedAt string                    `json:"checked_at"`
	Error     string                    `json:"error,omitempty"`
}

type UpdateCheckerStatus struct {
	Enabled     bool                         `json:"enabled"`
	Running     bool                         `json:"running"`
	Timezone    string                       `json:"timezone"`
	NextCheckAt string                       `json:"next_check_at,omitempty"`
	LastChecked string                       `json:"last_checked,omitempty"`
	Servers     map[string]ServerUpdateCheck `json:"servers"`
}

func (s *Store) updateChecksEnabled(ctx context.Context) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, UpdateCheckerEnabledKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return value == "true", err
}

func (s *Store) SetUpdateChecksEnabled(ctx context.Context, enabled bool) error {
	s.updateCheckMu.Lock()
	defer s.updateCheckMu.Unlock()
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, UpdateCheckerEnabledKey, fmt.Sprint(enabled))
	if err == nil && !enabled && s.updateCheckCancel != nil {
		s.updateCheckCancel()
	}
	return err
}

func nextUpdateCheck(now time.Time) time.Time {
	schedule, _ := schedules.Parse("midnight", schedules.DefaultTimezone())
	return schedule.Next(now)
}

// UpdateCheckerStatus reads saved results only; opening a page never starts a scan.
func (s *Store) UpdateCheckerStatus(ctx context.Context) (*UpdateCheckerStatus, error) {
	enabled, err := s.updateChecksEnabled(ctx)
	if err != nil {
		return nil, err
	}
	s.updateCheckMu.Lock()
	running := s.updateCheckRunning
	s.updateCheckMu.Unlock()
	status := &UpdateCheckerStatus{Enabled: enabled, Running: running, Timezone: schedules.DefaultTimezone(), Servers: map[string]ServerUpdateCheck{}}
	if !enabled {
		return status, nil
	}
	status.NextCheckAt = nextUpdateCheck(time.Now()).UTC().Format(time.RFC3339)
	err = s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'update_checker_last_checked'`).Scan(&status.LastChecked)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.server_id, c.software, c.mods, c.checked_at, c.error FROM server_update_checks c JOIN servers s ON s.id=c.server_id WHERE s.server_type=c.server_type AND s.version=c.version AND COALESCE(s.build,'')=c.build`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, software, mods string
		var result ServerUpdateCheck
		if err := rows.Scan(&id, &software, &mods, &result.CheckedAt, &result.Error); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(software), &result.Software); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(mods), &result.Mods); err != nil {
			return nil, err
		}
		status.Servers[id] = result
	}
	return status, rows.Err()
}

func (s *Store) CachedModUpdates(ctx context.Context, id string) (*ServerModUpdatesResponse, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT c.mods FROM server_update_checks c JOIN servers s ON s.id=c.server_id WHERE c.server_id=? AND s.server_type=c.server_type AND s.version=c.version AND COALESCE(s.build,'')=c.build`, id).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var result *ServerModUpdatesResponse
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &result); err != nil {
			return nil, err
		}
	}
	if result != nil {
		return result, nil
	}
	s.updatesMu.RLock()
	defer s.updatesMu.RUnlock()
	if result := s.updatesCache[id]; result != nil {
		return result, nil
	}
	return &ServerModUpdatesResponse{Updates: map[string]ModUpdateInfo{}}, nil
}

// beginUpdateCheck serializes manual and scheduled passes. Opting out cancels
// pending upstream requests; the next pass always receives a new response cache.
func (s *Store) beginUpdateCheck(parent context.Context) (context.Context, context.CancelFunc, error) {
	s.updateCheckMu.Lock()
	defer s.updateCheckMu.Unlock()
	enabled, err := s.updateChecksEnabled(parent)
	if err != nil {
		return nil, nil, err
	}
	if !enabled {
		return nil, nil, ErrUpdateCheckerDisabled
	}
	if s.updateCheckRunning {
		return nil, nil, nil
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	s.updateCheckRunning = true
	s.updateCheckCancel = cancel
	done := func() {
		cancel()
		s.updateCheckMu.Lock()
		s.updateCheckRunning = false
		s.updateCheckCancel = nil
		s.updateCheckMu.Unlock()
	}
	return checkcache.NewContext(ctx), done, nil
}

func (s *Store) StartUpdateCheck(logger *log.Logger) error {
	ctx, done, err := s.beginUpdateCheck(context.Background())
	if err != nil || done == nil {
		return err
	}
	if logger == nil {
		logger = log.Default()
	}
	go func() {
		defer done()
		if err := s.runUpdateCheck(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("update checker: %v", err)
		}
	}()
	return nil
}

func (s *Store) CheckUpdates(ctx context.Context) error {
	pass, done, err := s.beginUpdateCheck(ctx)
	if err != nil || done == nil {
		return err
	}
	defer done()
	return s.runUpdateCheck(pass)
}

func (s *Store) RunUpdateChecker(ctx context.Context, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}
	for {
		timer := time.NewTimer(time.Until(nextUpdateCheck(time.Now())))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if err := s.CheckUpdates(ctx); err != nil && !errors.Is(err, ErrUpdateCheckerDisabled) && !errors.Is(err, context.Canceled) {
				logger.Printf("update checker: %v", err)
			}
		}
	}
}

func (s *Store) runUpdateCheck(ctx context.Context) error {
	fleet, err := s.List(ctx)
	if err != nil {
		return err
	}
	previous, err := s.UpdateCheckerStatus(ctx)
	if err != nil {
		return err
	}
	for _, srv := range fleet {
		if err := ctx.Err(); err != nil {
			return err
		}
		result := ServerUpdateCheck{CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		var problems []string
		result.Software, err = s.checkSoftwareUpdate(ctx, srv)
		if err != nil {
			result.Software = previous.Servers[srv.ID].Software
			problems = append(problems, "Software check failed: "+err.Error())
		}
		result.Mods, err = s.CheckModUpdates(ctx, srv.ID, true)
		if err != nil {
			problems = append(problems, "Mod/plugin check failed: "+err.Error())
		} else if len(result.Mods.Errors) > 0 {
			problems = append(problems, result.Mods.Errors...)
		}
		if err != nil || (result.Mods != nil && len(result.Mods.Errors) > 0) {
			// Keep the last known badges when an upstream is unavailable.
			if old := previous.Servers[srv.ID].Mods; old != nil {
				result.Mods = old
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		result.Error = strings.Join(problems, "; ")
		if err := s.saveUpdateCheck(ctx, srv, result); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key,value) VALUES ('update_checker_last_checked',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) saveUpdateCheck(ctx context.Context, srv Server, result ServerUpdateCheck) error {
	software, err := json.Marshal(result.Software)
	if err != nil {
		return err
	}
	mods, err := json.Marshal(result.Mods)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO server_update_checks (server_id,server_type,version,build,software,mods,checked_at,error)
		SELECT id,server_type,version,COALESCE(build,''),?,?,?,? FROM servers WHERE id=? AND server_type=? AND version=? AND COALESCE(build,'')=?
		ON CONFLICT(server_id) DO UPDATE SET server_type=excluded.server_type, version=excluded.version, build=excluded.build, software=excluded.software, mods=excluded.mods, checked_at=excluded.checked_at, error=excluded.error`, string(software), string(mods), result.CheckedAt, result.Error, srv.ID, srv.ServerType, srv.Version, srv.Build)
	return err
}

var prereleasePattern = regexp.MustCompile(`(?i)(alpha|beta|snapshot|preview|experimental|dev|\d{2}w\d{2}|(?:^|[.\s_-])(pre|rc)[.\s_\d-])`)

func newestStable(values []string) string {
	latest := ""
	for _, v := range values {
		if v == "" || v == "latest" || v == "custom" || prereleasePattern.MatchString(v) {
			continue
		}
		if latest == "" || jars.CompareVersionTokens(v, latest) > 0 {
			latest = v
		}
	}
	return latest
}

func (s *Store) checkSoftwareUpdate(ctx context.Context, srv Server) (*SoftwareUpdate, error) {
	result := &SoftwareUpdate{}
	if s.jars == nil {
		return nil, errors.New("software catalog is unavailable")
	}
	ctx, upstreamError := checkcache.Observe(ctx)
	resolver := s.jars.ForUpdateCheck()
	versions, err := resolver.CatalogVersions(ctx, srv.ServerType)
	if errors.Is(err, jars.ErrUpdateCheckUnsupported) {
		result.Reason = err.Error()
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if err := upstreamError(); err != nil {
		return nil, err
	}
	if srv.Version == "latest" && srv.ServerType != "bungeecord" {
		result.Reason = "Pin the installed version to check for software updates."
		return result, nil
	}
	result.Supported = true
	result.LatestVersion = newestStable(versions)
	if result.LatestVersion == "" {
		result.LatestVersion = srv.Version
	}
	if jars.CompareVersionTokens(result.LatestVersion, srv.Version) > 0 {
		result.Available = true
		return result, nil
	}
	// Builds are compared within the installed version, never across game versions.
	result.LatestVersion = srv.Version
	if srv.Build == "" || srv.Build == "latest" {
		return result, nil
	}
	builds, err := resolver.CatalogBuilds(ctx, srv.ServerType, srv.Version)
	if err != nil {
		return nil, err
	}
	if err := upstreamError(); err != nil {
		return nil, err
	}
	result.LatestBuild = newestStable(builds)
	result.Available = result.LatestBuild != "" && jars.CompareVersionTokens(result.LatestBuild, srv.Build) > 0
	return result, nil
}
