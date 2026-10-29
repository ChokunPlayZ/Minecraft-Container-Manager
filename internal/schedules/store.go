// Package schedules persists recurring server tasks and calculates their next run.
package schedules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // IANA zones must also work on hosts without system tzdata.

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

var (
	ErrNotFound       = errors.New("schedule not found")
	ErrServerNotFound = errors.New("server not found")
	ErrInvalid        = errors.New("invalid schedule")
)

type Task struct {
	ID           string `json:"id"`
	ServerID     string `json:"server_id"`
	Name         string `json:"name"`
	Action       string `json:"action"`
	Command      string `json:"command"`
	BackupTarget string `json:"backup_target"`
	Cron         string `json:"cron"`
	Timezone     string `json:"timezone"`
	Enabled      bool   `json:"enabled"`
	NextRunAt    string `json:"next_run_at"`
	LastRunAt    string `json:"last_run_at"`
	LastStatus   string `json:"last_status"`
	LastError    string `json:"last_error"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type Input struct {
	Name         string `json:"name"`
	Action       string `json:"action"`
	Command      string `json:"command"`
	BackupTarget string `json:"backup_target"`
	Cron         string `json:"cron"`
	Timezone     string `json:"timezone"`
	Enabled      bool   `json:"enabled"`
}

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// DefaultTimezone shares the panel/container TZ setting, falling back to UTC.
func DefaultTimezone() string {
	zone := strings.TrimPrefix(strings.TrimSpace(os.Getenv("TZ")), ":")
	if zone != "" && zone != "Local" {
		if _, err := time.LoadLocation(zone); err == nil {
			return zone
		}
	}
	return "UTC"
}

// Parse accepts five-field cron, common names, and fixed whole-minute intervals.
// Timezones are separate from expressions so the saved zone is authoritative.
func Parse(expression, zone string) (cron.Schedule, error) {
	expression = strings.TrimSpace(expression)
	aliases := map[string]string{
		"midnight": "0 0 * * *", "daily": "0 0 * * *", "@midnight": "0 0 * * *", "@daily": "0 0 * * *",
		"hourly": "0 * * * *", "@hourly": "0 * * * *",
		"weekly": "0 0 * * 0", "@weekly": "0 0 * * 0",
		"monthly": "0 0 1 * *", "@monthly": "0 0 1 * *",
		"yearly": "0 0 1 1 *", "@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *",
	}
	if alias, ok := aliases[strings.ToLower(expression)]; ok {
		expression = alias
	}
	if zone == "" || zone == "Local" || len(zone) > 100 || strings.ContainsAny(zone, " \t\r\n") {
		return nil, fmt.Errorf("%w: choose an IANA timezone, such as Asia/Bangkok or UTC", ErrInvalid)
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return nil, fmt.Errorf("%w: unknown timezone %q", ErrInvalid, zone)
	}
	if strings.HasPrefix(expression, "@every ") {
		interval, err := time.ParseDuration(strings.TrimPrefix(expression, "@every "))
		if err != nil || interval < time.Minute || interval%time.Minute != 0 {
			return nil, fmt.Errorf("%w: intervals must be a whole number of minutes, at least 1 minute", ErrInvalid)
		}
		return cron.Every(interval), nil
	}
	if len(expression) > 120 || len(strings.Fields(expression)) != 5 {
		return nil, fmt.Errorf("%w: use five cron fields (minute hour day month weekday), midnight, hourly, daily, weekly, monthly, or @every 720m", ErrInvalid)
	}
	schedule, err := parser.Parse("CRON_TZ=" + zone + " " + expression)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return schedule, nil
}

func normalize(in Input, now time.Time) (Input, string, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Cron = strings.TrimSpace(in.Cron)
	in.Timezone = strings.TrimSpace(in.Timezone)
	if in.Timezone == "" {
		in.Timezone = DefaultTimezone()
	}
	in.Command = strings.TrimSpace(in.Command)
	if in.Name == "" || len(in.Name) > 100 {
		return in, "", fmt.Errorf("%w: task name must contain 1–100 characters", ErrInvalid)
	}
	switch in.Action {
	case "command":
		if in.Command == "" || len(in.Command) > 4096 || strings.ContainsAny(in.Command, "\r\n\x00") {
			return in, "", fmt.Errorf("%w: enter a single console command (up to 4096 characters)", ErrInvalid)
		}
	case "start", "stop", "restart", "backup":
		in.Command = ""
	default:
		return in, "", fmt.Errorf("%w: action must be command, start, stop, restart, or backup", ErrInvalid)
	}
	if in.Action == "backup" {
		switch in.BackupTarget {
		case "", "local", "s3":
		default:
			return in, "", fmt.Errorf("%w: backup target must be the panel default, local, or s3", ErrInvalid)
		}
	} else {
		in.BackupTarget = ""
	}
	schedule, err := Parse(in.Cron, in.Timezone)
	if err != nil {
		return in, "", err
	}
	next := schedule.Next(now)
	if next.IsZero() {
		return in, "", fmt.Errorf("%w: this expression has no upcoming run", ErrInvalid)
	}
	if !in.Enabled {
		return in, "", nil
	}
	return in, stamp(next), nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) serverExists(ctx context.Context, id string) error {
	var found int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM servers WHERE id = ?`, id).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServerNotFound
	}
	return err
}

const columns = `id, server_id, name, action, command, backup_target, cron, timezone, enabled, next_run_at, last_run_at, last_status, last_error, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ServerID, &t.Name, &t.Action, &t.Command, &t.BackupTarget, &t.Cron, &t.Timezone, &t.Enabled, &t.NextRunAt, &t.LastRunAt, &t.LastStatus, &t.LastError, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) List(ctx context.Context, serverID string) ([]Task, error) {
	if err := s.serverExists(ctx, serverID); err != nil {
		return nil, err
	}
	return s.query(ctx, `SELECT `+columns+` FROM scheduled_tasks WHERE server_id = ? ORDER BY created_at, id`, serverID)
}

func (s *Store) query(ctx context.Context, query string, args ...any) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (s *Store) Get(ctx context.Context, serverID, id string) (Task, error) {
	return scan(s.db.QueryRowContext(ctx, `SELECT `+columns+` FROM scheduled_tasks WHERE server_id = ? AND id = ?`, serverID, id))
}

func (s *Store) Create(ctx context.Context, serverID string, in Input) (Task, error) {
	if err := s.serverExists(ctx, serverID); err != nil {
		return Task{}, err
	}
	now := time.Now()
	in, next, err := normalize(in, now)
	if err != nil {
		return Task{}, err
	}
	id := uuid.NewString()
	_, err = s.db.ExecContext(ctx, `INSERT INTO scheduled_tasks (id, server_id, name, action, command, backup_target, cron, timezone, enabled, next_run_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, serverID, in.Name, in.Action, in.Command, in.BackupTarget, in.Cron, in.Timezone, in.Enabled, next, stamp(now), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Task{}, err
	}
	return s.Get(ctx, serverID, id)
}

func (s *Store) Update(ctx context.Context, serverID, id string, in Input) (Task, error) {
	now := time.Now()
	in, next, err := normalize(in, now)
	if err != nil {
		return Task{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET name=?, action=?, command=?, backup_target=?, cron=?, timezone=?, enabled=?, next_run_at=?, updated_at=? WHERE server_id=? AND id=?`, in.Name, in.Action, in.Command, in.BackupTarget, in.Cron, in.Timezone, in.Enabled, next, now.UTC().Format(time.RFC3339Nano), serverID, id)
	if err := affected(result, err); err != nil {
		return Task{}, err
	}
	return s.Get(ctx, serverID, id)
}

func (s *Store) SetEnabled(ctx context.Context, serverID, id string, enabled bool) (Task, error) {
	task, err := s.Get(ctx, serverID, id)
	if err != nil {
		return Task{}, err
	}
	next := ""
	if enabled {
		schedule, err := Parse(task.Cron, task.Timezone)
		if err != nil {
			return Task{}, err
		}
		t := schedule.Next(time.Now())
		if t.IsZero() {
			return Task{}, fmt.Errorf("%w: this expression has no upcoming run", ErrInvalid)
		}
		next = stamp(t)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET enabled=?, next_run_at=?, updated_at=? WHERE server_id=? AND id=?`, enabled, next, time.Now().UTC().Format(time.RFC3339Nano), serverID, id)
	if err := affected(result, err); err != nil {
		return Task{}, err
	}
	return s.Get(ctx, serverID, id)
}

func (s *Store) Delete(ctx context.Context, serverID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE server_id=? AND id=?`, serverID, id)
	return affected(result, err)
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
