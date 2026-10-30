// Package schedules persists recurring server tasks and calculates their next run.
package schedules

import (
	"context"
	"database/sql"
	"encoding/json"
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

const MaxSteps = 50

// Step is one action in a chain. Delays are explicit steps, in whole seconds.
type Step struct {
	Action       string `json:"action"`
	Command      string `json:"command,omitempty"`
	BackupTarget string `json:"backup_target,omitempty"`
	DelaySeconds int    `json:"delay_seconds,omitempty"`
}

type Task struct {
	ID           string `json:"id"`
	ServerID     string `json:"server_id"`
	Name         string `json:"name"`
	Action       string `json:"action"`
	Command      string `json:"command"`
	BackupTarget string `json:"backup_target"`
	Steps        []Step `json:"steps,omitempty"`
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
	Steps        []Step `json:"steps,omitempty"`
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
	if in.Name == "" || len(in.Name) > 100 {
		return in, "", fmt.Errorf("%w: task name must contain 1–100 characters", ErrInvalid)
	}
	if in.Action == "chain" {
		if len(in.Steps) < 1 || len(in.Steps) > MaxSteps {
			return in, "", fmt.Errorf("%w: an action chain must contain 1–%d steps", ErrInvalid, MaxSteps)
		}
		// Do not mutate a caller's slice while normalizing its configuration.
		in.Steps = append([]Step(nil), in.Steps...)
		for i, step := range in.Steps {
			normalized, err := normalizeStep(step, true)
			if err != nil {
				return in, "", fmt.Errorf("step %d: %w", i+1, err)
			}
			in.Steps[i] = normalized
		}
		in.Command, in.BackupTarget = "", ""
	} else {
		step, err := normalizeStep(Step{Action: in.Action, Command: in.Command, BackupTarget: in.BackupTarget}, false)
		if err != nil {
			return in, "", err
		}
		in.Command, in.BackupTarget, in.Steps = step.Command, step.BackupTarget, nil
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

func normalizeStep(step Step, allowDelay bool) (Step, error) {
	step.Command = strings.TrimSpace(step.Command)
	switch step.Action {
	case "command":
		if step.Command == "" || len(step.Command) > 4096 || strings.ContainsAny(step.Command, "\r\n\x00") {
			return step, fmt.Errorf("%w: enter a single console command (up to 4096 characters)", ErrInvalid)
		}
	case "start", "stop", "restart", "backup":
		step.Command = ""
	case "delay":
		if !allowDelay || step.DelaySeconds < 1 || step.DelaySeconds > 3600 {
			return step, fmt.Errorf("%w: wait steps are only allowed in chains and must be 1–3600 whole seconds", ErrInvalid)
		}
		step.Command = ""
	default:
		if allowDelay {
			return step, fmt.Errorf("%w: chain steps must be command, start, stop, restart, backup, or delay; nested chains are not allowed", ErrInvalid)
		}
		return step, fmt.Errorf("%w: action must be command, start, stop, restart, backup, or chain", ErrInvalid)
	}
	if step.Action == "backup" {
		switch step.BackupTarget {
		case "", "local", "s3":
		default:
			return step, fmt.Errorf("%w: backup target must be the panel default, local, or s3", ErrInvalid)
		}
	} else {
		step.BackupTarget = ""
	}
	if step.Action != "delay" {
		step.DelaySeconds = 0
	}
	return step, nil
}

func encodeSteps(steps []Step) string {
	if len(steps) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(steps) // Step only contains strings and integers.
	return string(data)
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

const columns = `id, server_id, name, action, command, backup_target, steps, cron, timezone, enabled, next_run_at, last_run_at, last_status, last_error, created_at, updated_at`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Task, error) {
	var t Task
	var steps string
	err := row.Scan(&t.ID, &t.ServerID, &t.Name, &t.Action, &t.Command, &t.BackupTarget, &steps, &t.Cron, &t.Timezone, &t.Enabled, &t.NextRunAt, &t.LastRunAt, &t.LastStatus, &t.LastError, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal([]byte(steps), &t.Steps); err != nil {
		return t, fmt.Errorf("decode schedule steps: %w", err)
	}
	return t, nil
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
	_, err = s.db.ExecContext(ctx, `INSERT INTO scheduled_tasks (id, server_id, name, action, command, backup_target, steps, cron, timezone, enabled, next_run_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, serverID, in.Name, in.Action, in.Command, in.BackupTarget, encodeSteps(in.Steps), in.Cron, in.Timezone, in.Enabled, next, stamp(now), now.UTC().Format(time.RFC3339Nano))
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
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET name=?, action=?, command=?, backup_target=?, steps=?, cron=?, timezone=?, enabled=?, next_run_at=?, updated_at=? WHERE server_id=? AND id=?`, in.Name, in.Action, in.Command, in.BackupTarget, encodeSteps(in.Steps), in.Cron, in.Timezone, in.Enabled, next, now.UTC().Format(time.RFC3339Nano), serverID, id)
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
