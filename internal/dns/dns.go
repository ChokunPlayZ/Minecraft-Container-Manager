// Package dns publishes Cloudflare SRV records that point a domain at running
// MCM servers. Configuration is read from the settings table on each operation
// so an operator can toggle publishing and set the zone, domain, and API token
// without restarting MCM. When publishing is disabled or under-configured,
// operations are no-ops that tolerate the missing setup.
package dns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Settings keys read by the service. They live in the generic settings table.
const (
	KeyPublish  = "dns_publish"
	KeyDomain   = "dns_domain"
	KeyZone     = "dns_zone"
	KeyAPIToken = "dns_api_token"
	KeyHost     = "dns_host"
	KeyService  = "dns_service"
	KeyProto    = "dns_proto"
	KeyTTL      = "dns_ttl"
)

// Defaults applied when a setting is empty.
const (
	defaultService = "_minecraft"
	defaultProto   = "_tcp"
	defaultTTL     = 120
)

// Publisher manages SRV records for server addresses.
type Publisher interface {
	// Upsert creates or updates the SRV record for a server.
	Upsert(ctx context.Context, serverID, host string, port int) error
	// Remove deletes the SRV record for a server.
	Remove(ctx context.Context, serverID string) error
}

// ErrNotConfigured indicates DNS publishing is disabled or missing required
// settings. Callers can treat it as a benign, non-fatal condition.
var ErrNotConfigured = errors.New("DNS publishing is not configured")

// Service publishes and removes Cloudflare SRV records as servers start and
// stop. Operations are no-ops when publishing is disabled or the zone, domain,
// or API token are not configured.
type Service struct {
	db   *sql.DB
	http *http.Client
}

var _ Publisher = (*Service)(nil)

// New returns a Service that publishes records through Cloudflare. The service
// reads its config from the settings table on each operation, so it needs only
// the database handle.
func New(db *sql.DB) *Service {
	return &Service{
		db:   db,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// Upsert creates or updates the SRV record that points at host:port for a
// server. It is a no-op when DNS publishing is disabled or under-configured.
func (s *Service) Upsert(ctx context.Context, serverID, host string, port int) error {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}

	sub := safeLabel(serverID)
	target := cfg.Target(host)
	priority, weight := cfg.Priority, cfg.Weight

	// Existing record, if any.
	row, err := s.getRecord(ctx, serverID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read dns record: %w", err)
	}

	client := newCFClient(cfg.APIToken, cfg.Zone, s.http)
	name := cfg.recordName(sub)

	if row != nil && row.RecordID != "" {
		// Update the existing Cloudflare record in place.
		if err := client.updateRecord(ctx, row.RecordID, name, target, port, cfg.TTL, priority, weight); err != nil {
			return fmt.Errorf("update dns record: %w", err)
		}
	} else {
		// Create a new Cloudflare record.
		recordID, err := client.createRecord(ctx, name, target, port, cfg.TTL, priority, weight)
		if err != nil {
			return fmt.Errorf("create dns record: %w", err)
		}
		row = &record{
			ServerID: serverID,
			RecordID: recordID,
			Name:     name,
			Target:   target,
			Port:     port,
			Zone:     cfg.Zone,
		}
	}

	return s.upsertRecord(ctx, serverID, row, target, port, priority, weight, cfg.TTL, cfg.Zone)
}

// Remove deletes the SRV record for a server if one was published.
func (s *Service) Remove(ctx context.Context, serverID string) error {
	row, err := s.getRecord(ctx, serverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read dns record: %w", err)
	}

	cfg, cerr := s.loadConfig(ctx)
	// Even if config is missing, drop the tracking row so a later re-enable
	// starts from a clean slate. Only reach out to Cloudflare when configured.
	if cerr == nil && cfg.APIToken != "" && cfg.Zone != "" && row.RecordID != "" {
		client := newCFClient(cfg.APIToken, cfg.Zone, s.http)
		if err := client.deleteRecord(ctx, row.RecordID); err != nil {
			return fmt.Errorf("delete dns record: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM dns_records WHERE server_id = ?`, serverID)
	if err != nil {
		return fmt.Errorf("delete dns record row: %w", err)
	}
	return nil
}

// List returns the records currently tracked in the database (published or
// pending), newest first.
func (s *Service) List(ctx context.Context) ([]record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_id, record_id, name, target, port, priority, weight, ttl, zone, updated_at
		 FROM dns_records ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list dns records: %w", err)
	}
	defer rows.Close()

	var out []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Target, &r.Port,
			&r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// config is the effective DNS publishing configuration for one operation.
type config struct {
	Publish  bool
	Domain   string
	Zone     string
	APIToken string
	Host     string
	Service  string
	Proto    string
	TTL      int
	Priority int
	Weight   int
}

// Target returns the SRV target host, preferring the configured host then the
// caller-supplied host.
func (c *config) Target(host string) string {
	if c.Host != "" {
		return c.Host
	}
	if host != "" {
		return host
	}
	return c.Domain
}

// recordName builds the full SRV record name for a server subdomain, e.g.
// _minecraft._tcp.<sub>.<domain>.
func (c *config) recordName(sub string) string {
	return strings.Join([]string{c.Service, c.Proto, sub + "." + c.Domain}, ".")
}

func (s *Service) loadConfig(ctx context.Context) (*config, error) {
	values, err := s.readSettings(ctx, KeyPublish, KeyDomain, KeyZone, KeyAPIToken, KeyHost, KeyService, KeyProto, KeyTTL)
	if err != nil {
		return nil, err
	}

	cfg := buildConfig(values)
	if !cfg.Publish {
		return nil, ErrNotConfigured
	}
	if cfg.Domain == "" || cfg.Zone == "" || cfg.APIToken == "" {
		return nil, ErrNotConfigured
	}
	return cfg, nil
}

// buildConfig turns raw settings values into a config, applying defaults.
func buildConfig(values map[string]string) *config {
	cfg := &config{
		Publish:  strings.EqualFold(values[KeyPublish], "true"),
		Domain:   values[KeyDomain],
		Zone:     values[KeyZone],
		APIToken: values[KeyAPIToken],
		Host:     values[KeyHost],
		Service:  strings.TrimSpace(values[KeyService]),
		Proto:    strings.TrimSpace(values[KeyProto]),
		TTL:      defaultTTL,
	}
	if cfg.Service == "" {
		cfg.Service = defaultService
	}
	if cfg.Proto == "" {
		cfg.Proto = defaultProto
	}
	if ttl := strings.TrimSpace(values[KeyTTL]); ttl != "" {
		if n, err := strconv.Atoi(ttl); err == nil && n > 0 {
			cfg.TTL = n
		}
	}
	return cfg
}

func (s *Service) readSettings(ctx context.Context, keys ...string) (map[string]string, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM settings WHERE key IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// record is a row in the dns_records tracking table.
type record struct {
	ServerID  string `json:"server_id"`
	RecordID  string `json:"record_id"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	Port      int    `json:"port"`
	Priority  int    `json:"priority"`
	Weight    int    `json:"weight"`
	TTL       int    `json:"ttl"`
	Zone      string `json:"zone,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Service) getRecord(ctx context.Context, serverID string) (*record, error) {
	var r record
	err := s.db.QueryRowContext(ctx,
		`SELECT server_id, record_id, name, target, port, priority, weight, ttl, zone, updated_at
		 FROM dns_records WHERE server_id = ?`, serverID).
		Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Target, &r.Port, &r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) upsertRecord(ctx context.Context, serverID string, r *record, target string, port, priority, weight, ttl int, zone string) error {
	name := r.Name
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dns_records (server_id, record_id, name, target, port, priority, weight, ttl, zone, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(server_id) DO UPDATE SET
		   record_id=excluded.record_id,
		   name=excluded.name,
		   target=excluded.target,
		   port=excluded.port,
		   priority=excluded.priority,
		   weight=excluded.weight,
		   ttl=excluded.ttl,
		   zone=excluded.zone,
		   updated_at=excluded.updated_at`,
		serverID, r.RecordID, name, target, port, priority, weight, ttl, zone, now)
	return err
}

// safeLabel makes a server id safe to embed as a DNS label. UUIDs are already
// safe; this guards against any future non-hyphen-safe identifiers.
func safeLabel(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteRune(c)
		case c >= 'A' && c <= 'Z':
			b.WriteRune(c - 'A' + 'a')
		case c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == '-':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "server"
	}
	return b.String()
}
