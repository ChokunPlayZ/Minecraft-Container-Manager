// Package notifications delivers optional Discord notifications without blocking server operations.
package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const SettingsKey = "discord_notifications"

const (
	ServerStarted   = "server_started"
	ServerStopped   = "server_stopped"
	ServerCrashed   = "server_crashed"
	ServerError     = "server_error"
	BackupCompleted = "backup_completed"
	BackupFailed    = "backup_failed"
)

var eventTitles = map[string]string{
	ServerStarted: "Server started", ServerStopped: "Server stopped",
	ServerCrashed: "Server crashed", ServerError: "Server error",
	BackupCompleted: "Backup completed", BackupFailed: "Backup failed",
}

type Config struct {
	Enabled    bool     `json:"enabled"`
	WebhookURL string   `json:"webhook_url"`
	Events     []string `json:"events"`
}

// Settings never returns the webhook token to the browser.
type Settings struct {
	Enabled    bool     `json:"enabled"`
	Configured bool     `json:"configured"`
	Events     []string `json:"events"`
}

type Update struct {
	Enabled    bool     `json:"enabled"`
	WebhookURL *string  `json:"webhook_url"`
	Events     []string `json:"events"`
}

type Event struct {
	Kind       string
	ServerID   string
	ServerName string
	Message    string
	Time       time.Time
}

type Notifier interface{ Notify(Event) }

type Service struct {
	db     *sql.DB
	client *http.Client
	logger *log.Logger
	queue  chan Event
}

func New(db *sql.DB, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.Default()
	}
	return &Service{db: db, logger: logger, queue: make(chan Event, 128), client: &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *Service) config(ctx context.Context) (Config, error) {
	c := Config{Events: []string{ServerStarted, ServerStopped, ServerCrashed, ServerError, BackupCompleted, BackupFailed}}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, SettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal([]byte(raw), &c)
	return c, err
}

func publicSettings(c Config) Settings {
	return Settings{Enabled: c.Enabled, Configured: c.WebhookURL != "", Events: c.Events}
}

func (s *Service) Settings(ctx context.Context) (Settings, error) {
	c, err := s.config(ctx)
	return publicSettings(c), err
}

var webhookPath = regexp.MustCompile(`^/api(?:/v[0-9]+)?/webhooks/[0-9]+/[A-Za-z0-9_-]+$`)
var threadID = regexp.MustCompile(`^[0-9]+$`)

// ValidateURL restricts requests to Discord's HTTPS webhook endpoints.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 2048 {
		return errors.New("enter a valid Discord webhook URL")
	}
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || !webhookPath.MatchString(u.EscapedPath()) {
		return errors.New("use an HTTPS Discord webhook URL copied from channel integrations")
	}
	switch u.Host {
	case "discord.com", "discordapp.com", "canary.discord.com", "ptb.discord.com":
	default:
		return errors.New("webhook URL must use a Discord domain")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return errors.New("invalid webhook URL query")
	}
	for key, values := range q {
		if len(values) != 1 || (key != "thread_id" && key != "wait") || (key == "thread_id" && !threadID.MatchString(values[0])) || (key == "wait" && values[0] != "true" && values[0] != "false") {
			return errors.New("webhook URL only supports thread_id and wait query parameters")
		}
	}
	return nil
}

func (s *Service) Save(ctx context.Context, in Update) (Settings, error) {
	c, err := s.config(ctx)
	if err != nil {
		return Settings{}, err
	}
	if in.WebhookURL != nil {
		c.WebhookURL = strings.TrimSpace(*in.WebhookURL)
	}
	c.Enabled = in.Enabled
	c.Events = make([]string, 0, len(in.Events))
	seen := map[string]bool{}
	for _, event := range in.Events {
		if _, ok := eventTitles[event]; !ok {
			return Settings{}, errors.New("unknown notification event")
		}
		if !seen[event] {
			c.Events = append(c.Events, event)
			seen[event] = true
		}
	}
	if c.WebhookURL != "" {
		if err := ValidateURL(c.WebhookURL); err != nil {
			return Settings{}, err
		}
	} else if c.Enabled {
		return Settings{}, errors.New("add a Discord webhook URL before enabling notifications")
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return Settings{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, SettingsKey, string(raw))
	return publicSettings(c), err
}

func (s *Service) Test(ctx context.Context, webhook *string) error {
	c, err := s.config(ctx)
	if err != nil {
		return err
	}
	if webhook != nil {
		c.WebhookURL = strings.TrimSpace(*webhook)
	}
	if c.WebhookURL == "" {
		return errors.New("add a Discord webhook URL first")
	}
	return s.send(ctx, c.WebhookURL, Event{Kind: "test", Message: "Discord notifications are connected. Save your settings to apply your event selections.", Time: time.Now()})
}

// Notify is bounded and never waits for Discord or the database.
func (s *Service) Notify(e Event) {
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	select {
	case s.queue <- e:
	default:
		s.logger.Printf("discord: notification queue full; dropped %s for %s", e.Kind, e.ServerID)
	}
}

// Run processes events serially, preserving order and bounding concurrent requests.
func (s *Service) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-s.queue:
			if ctx.Err() != nil {
				return
			}
			c, err := s.config(ctx)
			if err != nil {
				s.logger.Print("discord: could not read notification settings")
				continue
			}
			if !c.Enabled {
				continue
			}
			enabled := false
			for _, kind := range c.Events {
				if kind == e.Kind {
					enabled = true
					break
				}
			}
			if !enabled {
				continue
			}
			deliveryCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			err = s.send(deliveryCtx, c.WebhookURL, e)
			cancel()
			if err != nil {
				s.logger.Printf("discord: %s for %s: %v", e.Kind, e.ServerID, err)
			}
		}
	}
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func (s *Service) send(ctx context.Context, webhook string, e Event) error {
	if err := ValidateURL(webhook); err != nil {
		return err
	}
	u, _ := url.Parse(webhook)
	q := u.Query()
	q.Set("wait", "true")
	u.RawQuery = q.Encode()
	title := eventTitles[e.Kind]
	if title == "" {
		title = "MCM test notification"
	}
	color := 0x236d61
	if e.Kind == ServerCrashed || e.Kind == ServerError || e.Kind == BackupFailed {
		color = 0xb63842
	}
	fields := []map[string]any{}
	if e.ServerID != "" {
		fields = append(fields, map[string]any{"name": "Server", "value": truncate(e.ServerName, 1024)}, map[string]any{"name": "Server ID", "value": truncate(e.ServerID, 1024)})
	}
	payload, err := json.Marshal(map[string]any{
		"username": "MCM", "allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []any{map[string]any{"title": title, "description": truncate(e.Message, 4096), "color": color, "fields": fields, "timestamp": e.Time.UTC().Format(time.RFC3339)}},
	})
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
		if err != nil {
			return errors.New("could not create Discord request")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.client.Do(req)
		// Transport errors can contain the secret URL, so never expose them.
		if err != nil {
			return errors.New("could not reach Discord; check the connection and try again")
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			var limit struct {
				RetryAfter float64 `json:"retry_after"`
			}
			_ = json.Unmarshal(body, &limit)
			if limit.RetryAfter <= 0 {
				limit.RetryAfter, _ = strconv.ParseFloat(resp.Header.Get("Retry-After"), 64)
			}
			if limit.RetryAfter <= 0 || limit.RetryAfter > 30 || math.IsNaN(limit.RetryAfter) {
				return errors.New("Discord is rate limiting notifications; try again later")
			}
			timer := time.NewTimer(time.Duration(limit.RetryAfter * float64(time.Second)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.New("Discord delivery cancelled")
			case <-timer.C:
			}
			continue
		}
		if resp.StatusCode == 401 || resp.StatusCode == 404 {
			return errors.New("Discord webhook was rejected; replace the webhook URL and try again")
		}
		return fmt.Errorf("Discord returned HTTP %d; check the webhook and try again", resp.StatusCode)
	}
	return errors.New("Discord delivery failed")
}
