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
	KeyPriority = "dns_priority"
	KeyWeight   = "dns_weight"
)

// Defaults applied when a setting is empty.
const (
	defaultService  = "_minecraft"
	defaultProto    = "_tcp"
	defaultTTL      = 120
	defaultPriority = 0
	defaultWeight   = 5
)

// PublishOptions configures the SRV record publication for a server.
type PublishOptions struct {
	Subdomain string `json:"subdomain,omitempty"`
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	Priority  *int   `json:"priority,omitempty"`
	Weight    *int   `json:"weight,omitempty"`
}

// Record is a row in the dns_records tracking table.
type Record struct {
	ServerID  string `json:"server_id"`
	RecordID  string `json:"record_id"`
	Name      string `json:"name"`
	Subdomain string `json:"subdomain"`
	Target    string `json:"target"`
	Port      int    `json:"port"`
	Priority  int    `json:"priority"`
	Weight    int    `json:"weight"`
	TTL       int    `json:"ttl"`
	Zone      string `json:"zone,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

// ConfigInfo represents the safe public configuration of the DNS service.
type ConfigInfo struct {
	Publish  bool   `json:"publish"`
	Domain   string `json:"domain"`
	Zone     string `json:"zone"`
	HasToken bool   `json:"has_token"`
	Host     string `json:"host"`
	Service  string `json:"service"`
	Proto    string `json:"proto"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
	Weight   int    `json:"weight"`
}

// VerifyResult details the result of testing Cloudflare credentials.
type VerifyResult struct {
	OK       bool   `json:"ok"`
	ZoneName string `json:"zone_name,omitempty"`
	Status   string `json:"status,omitempty"`
	Message  string `json:"message"`
}

// Publisher manages SRV records for server addresses.
type Publisher interface {
	// Upsert creates or updates the SRV record for a server using defaults.
	Upsert(ctx context.Context, serverID, host string, port int) error
	// UpsertWithOptions creates or updates the SRV record with specific parameters.
	UpsertWithOptions(ctx context.Context, serverID string, opts PublishOptions) (*Record, error)
	// Remove deletes the SRV record for a server.
	Remove(ctx context.Context, serverID string) error
	// GetRecord returns the tracked DNS record for a server, if one exists.
	GetRecord(ctx context.Context, serverID string) (*Record, error)
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

// SetHTTPClient overrides the HTTP client used to communicate with Cloudflare (e.g. for testing).
func (s *Service) SetHTTPClient(hc *http.Client) {
	if hc != nil {
		s.http = hc
	}
}

// Upsert creates or updates the SRV record that points at host:port for a
// server using default options. It is a no-op when DNS publishing is disabled
// or under-configured.
func (s *Service) Upsert(ctx context.Context, serverID, host string, port int) error {
	_, err := s.UpsertWithOptions(ctx, serverID, PublishOptions{
		Host: host,
		Port: port,
	})
	return err
}

// UpsertWithOptions creates or updates the SRV record for a server with specific
// options (subdomain, target host, port, priority, weight).
func (s *Service) UpsertWithOptions(ctx context.Context, serverID string, opts PublishOptions) (*Record, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Existing record, if any.
	existing, err := s.GetRecord(ctx, serverID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read dns record: %w", err)
	}

	// Determine port.
	port := opts.Port
	if port <= 0 {
		if existing != nil && existing.Port > 0 {
			port = existing.Port
		} else {
			// Query server host_port.
			var dbPort int
			if qerr := s.db.QueryRowContext(ctx, `SELECT host_port FROM servers WHERE id = ?`, serverID).Scan(&dbPort); qerr == nil && dbPort > 0 {
				port = dbPort
			} else {
				port = 25565
			}
		}
	}

	// Determine target host.
	target := cfg.Target(opts.Host)
	if opts.Host == "" && existing != nil && existing.Target != "" && cfg.Host == "" {
		target = existing.Target
	}

	// Determine priority and weight.
	priority := cfg.Priority
	if opts.Priority != nil {
		priority = *opts.Priority
	} else if existing != nil {
		priority = existing.Priority
	}

	weight := cfg.Weight
	if opts.Weight != nil {
		weight = *opts.Weight
	} else if existing != nil {
		weight = existing.Weight
	}

	// Determine subdomain and names.
	subdomain, srvName, recordName := s.resolveNames(ctx, serverID, opts.Subdomain, existing, cfg)

	client := newCFClient(cfg.APIToken, cfg.Zone, s.http)

	var recordID string
	if existing != nil && existing.RecordID != "" {
		recordID = existing.RecordID
		if uerr := client.updateRecord(ctx, recordID, cfg.Service, cfg.Proto, srvName, recordName, target, port, cfg.TTL, priority, weight); uerr != nil {
			// If Cloudflare returned 404 or record was removed externally, attempt creation.
			recordID = ""
		}
	}

	if recordID == "" {
		// Check if an SRV record with this exact name already exists in Cloudflare.
		if foundID, ferr := client.findSRVRecord(ctx, recordName); ferr == nil && foundID != "" {
			recordID = foundID
			if err := client.updateRecord(ctx, recordID, cfg.Service, cfg.Proto, srvName, recordName, target, port, cfg.TTL, priority, weight); err != nil {
				return nil, fmt.Errorf("update existing cloudflare record: %w", err)
			}
		} else {
			createdID, err := client.createRecord(ctx, cfg.Service, cfg.Proto, srvName, recordName, target, port, cfg.TTL, priority, weight)
			if err != nil {
				return nil, fmt.Errorf("create dns record: %w", err)
			}
			recordID = createdID
		}
	}

	rec := &Record{
		ServerID:  serverID,
		RecordID:  recordID,
		Name:      recordName,
		Subdomain: subdomain,
		Target:    target,
		Port:      port,
		Priority:  priority,
		Weight:    weight,
		TTL:       cfg.TTL,
		Zone:      cfg.Zone,
	}

	if err := s.upsertRecordRow(ctx, rec); err != nil {
		return nil, fmt.Errorf("store dns record: %w", err)
	}

	return rec, nil
}

// Remove deletes the SRV record for a server if one was published.
func (s *Service) Remove(ctx context.Context, serverID string) error {
	row, err := s.GetRecord(ctx, serverID)
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
		_ = client.deleteRecord(ctx, row.RecordID)
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM dns_records WHERE server_id = ?`, serverID)
	if err != nil {
		return fmt.Errorf("delete dns record row: %w", err)
	}
	return nil
}

// List returns the records currently tracked in the database, newest first.
func (s *Service) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, updated_at
		 FROM dns_records ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list dns records: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Subdomain, &r.Target, &r.Port,
			&r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRecord returns the tracked DNS record for a server if present.
func (s *Service) GetRecord(ctx context.Context, serverID string) (*Record, error) {
	var r Record
	err := s.db.QueryRowContext(ctx,
		`SELECT server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, updated_at
		 FROM dns_records WHERE server_id = ?`, serverID).
		Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Subdomain, &r.Target, &r.Port, &r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetConfig reads the current DNS settings and returns safe metadata for callers.
func (s *Service) GetConfig(ctx context.Context) (*ConfigInfo, error) {
	values, err := s.readSettings(ctx, KeyPublish, KeyDomain, KeyZone, KeyAPIToken, KeyHost, KeyService, KeyProto, KeyTTL, KeyPriority, KeyWeight)
	if err != nil {
		return nil, err
	}
	cfg := buildConfig(values)
	return &ConfigInfo{
		Publish:  cfg.Publish,
		Domain:   cfg.Domain,
		Zone:     cfg.Zone,
		HasToken: cfg.APIToken != "",
		Host:     cfg.Host,
		Service:  cfg.Service,
		Proto:    cfg.Proto,
		TTL:      cfg.TTL,
		Priority: cfg.Priority,
		Weight:   cfg.Weight,
	}, nil
}

// Verify tests Cloudflare credentials against the Cloudflare API.
func (s *Service) Verify(ctx context.Context, token, zone, domain string) (*VerifyResult, error) {
	if token == "" || zone == "" {
		// Try loading from saved settings.
		cfg, err := s.loadConfig(ctx)
		if err != nil {
			return &VerifyResult{OK: false, Message: "Cloudflare token and zone ID are required"}, nil
		}
		if token == "" {
			token = cfg.APIToken
		}
		if zone == "" {
			zone = cfg.Zone
		}
		if domain == "" {
			domain = cfg.Domain
		}
	}

	client := newCFClient(token, zone, s.http)
	info, err := client.verifyZone(ctx)
	if err != nil {
		return &VerifyResult{
			OK:      false,
			Message: fmt.Sprintf("Cloudflare verification failed: %v", err),
		}, nil
	}

	msg := fmt.Sprintf("Successfully verified zone %q (status: %s)", info.Name, info.Status)
	if domain != "" && !strings.EqualFold(domain, info.Name) && !strings.HasSuffix(strings.ToLower(domain), "."+strings.ToLower(info.Name)) {
		msg += fmt.Sprintf(". Warning: configured domain %q does not match or belong to zone %q", domain, info.Name)
	}

	return &VerifyResult{
		OK:       true,
		ZoneName: info.Name,
		Status:   info.Status,
		Message:  msg,
	}, nil
}

// resolveNames computes the subdomain, data.name (srvName), and full DNS record name.
func (s *Service) resolveNames(ctx context.Context, serverID, reqSub string, existing *Record, cfg *config) (subdomain, srvName, recordName string) {
	reqSub = strings.TrimSpace(reqSub)

	// Explicit request for root domain: "@" or "root"
	if reqSub == "@" || strings.EqualFold(reqSub, "root") {
		return "@", cfg.Domain, cfg.recordName("")
	}

	if reqSub != "" {
		clean := safeLabel(reqSub)
		return clean, clean + "." + cfg.Domain, cfg.recordName(clean)
	}

	// Check if existing record had a configured subdomain.
	if existing != nil && existing.Subdomain != "" {
		if existing.Subdomain == "@" {
			return "@", cfg.Domain, cfg.recordName("")
		}
		clean := safeLabel(existing.Subdomain)
		return clean, clean + "." + cfg.Domain, cfg.recordName(clean)
	}

	// Try using server name as friendly default subdomain.
	var serverName string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM servers WHERE id = ?`, serverID).Scan(&serverName); err == nil && serverName != "" {
		clean := safeLabel(serverName)
		if clean != "" && clean != "server" {
			return clean, clean + "." + cfg.Domain, cfg.recordName(clean)
		}
	}

	// Fallback to server ID.
	clean := safeLabel(serverID)
	return clean, clean + "." + cfg.Domain, cfg.recordName(clean)
}

func (s *Service) upsertRecordRow(ctx context.Context, r *Record) error {
	now := time.Now().UTC().Format(time.RFC3339)
	r.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dns_records (server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(server_id) DO UPDATE SET
		   record_id=excluded.record_id,
		   name=excluded.name,
		   subdomain=excluded.subdomain,
		   target=excluded.target,
		   port=excluded.port,
		   priority=excluded.priority,
		   weight=excluded.weight,
		   ttl=excluded.ttl,
		   zone=excluded.zone,
		   updated_at=excluded.updated_at`,
		r.ServerID, r.RecordID, r.Name, r.Subdomain, r.Target, r.Port, r.Priority, r.Weight, r.TTL, r.Zone, now)
	return err
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

// recordName builds the full SRV record name for a server subdomain.
func (c *config) recordName(sub string) string {
	svc := c.Service
	if !strings.HasPrefix(svc, "_") {
		svc = "_" + svc
	}
	prt := c.Proto
	if !strings.HasPrefix(prt, "_") {
		prt = "_" + prt
	}
	if sub == "" || sub == "@" {
		return strings.Join([]string{svc, prt, c.Domain}, ".")
	}
	return strings.Join([]string{svc, prt, sub + "." + c.Domain}, ".")
}

func (s *Service) loadConfig(ctx context.Context) (*config, error) {
	values, err := s.readSettings(ctx, KeyPublish, KeyDomain, KeyZone, KeyAPIToken, KeyHost, KeyService, KeyProto, KeyTTL, KeyPriority, KeyWeight)
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
		Domain:   strings.TrimSpace(values[KeyDomain]),
		Zone:     strings.TrimSpace(values[KeyZone]),
		APIToken: strings.TrimSpace(values[KeyAPIToken]),
		Host:     strings.TrimSpace(values[KeyHost]),
		Service:  strings.TrimSpace(values[KeyService]),
		Proto:    strings.TrimSpace(values[KeyProto]),
		TTL:      defaultTTL,
		Priority: defaultPriority,
		Weight:   defaultWeight,
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
	if p := strings.TrimSpace(values[KeyPriority]); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 {
			cfg.Priority = n
		}
	}
	if w := strings.TrimSpace(values[KeyWeight]); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n >= 0 {
			cfg.Weight = n
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

// safeLabel makes a label safe to embed as a DNS hostname label.
// Lowercases alphanumeric characters and replaces spaces/underscores with hyphens.
func safeLabel(s string) string {
	var b strings.Builder
	prevDash := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteRune(c)
			prevDash = false
		case c >= 'A' && c <= 'Z':
			b.WriteRune(c - 'A' + 'a')
			prevDash = false
		case c >= '0' && c <= '9':
			b.WriteRune(c)
			prevDash = false
		case c == '-' || c == '_' || c == ' ':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return "server"
	}
	return res
}
