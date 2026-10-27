// Package dns publishes SRV records that point a domain at running
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
	"sync"
	"time"
)

// Settings keys read by the service. They live in the generic settings table.
const (
	KeyProvider = "dns_provider"
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
	Provider  string `json:"provider"`
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

// JoinAddress is derived from the published name, even if the global domain
// has since been edited.
func (r *Record) JoinAddress() string {
	parts := strings.SplitN(r.Name, ".", 3)
	if len(parts) != 3 {
		return ""
	}
	return parts[2]
}

// ConfigInfo represents the safe public configuration of the DNS service.
type ConfigInfo struct {
	Provider   string `json:"provider"`
	Configured bool   `json:"configured"`
	Publish    bool   `json:"publish"`
	Domain     string `json:"domain"`
	Zone       string `json:"zone"`
	HasToken   bool   `json:"has_token"`
	Host       string `json:"host"`
	Service    string `json:"service"`
	Proto      string `json:"proto"`
	TTL        int    `json:"ttl"`
	Priority   int    `json:"priority"`
	Weight     int    `json:"weight"`
}

// VerifyResult details the result of testing DNS provider credentials.
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

// Service publishes and removes SRV records as servers start and
// stop. Operations are no-ops when publishing is disabled or the zone, domain,
// or API token are not configured.
type Service struct {
	db    *sql.DB
	http  *http.Client
	locks sync.Map
}

var _ Publisher = (*Service)(nil)

// New returns a Service that publishes records through the configured DNS provider. The service
// reads its config from the settings table on each operation, so it needs only
// the database handle.
func New(db *sql.DB) *Service {
	return &Service{
		db:   db,
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// SetHTTPClient overrides the HTTP client used to communicate with DNS providers (e.g. for testing).
func (s *Service) SetHTTPClient(hc *http.Client) {
	if hc != nil {
		s.http = hc
	}
}

// Serialize lifecycle operations for the same server without blocking DNS
// requests for other servers while a provider is slow.
func (s *Service) lockServer(serverID string) func() {
	value, _ := s.locks.LoadOrStore(serverID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
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
	unlock := s.lockServer(serverID)
	defer unlock()
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

	// A failed preference read must never replace the saved address with a default.
	if strings.TrimSpace(opts.Subdomain) == "" {
		opts.Subdomain, err = s.GetSubdomain(ctx, serverID)
		if err != nil {
			return nil, fmt.Errorf("read DNS preference: %w", err)
		}
	}
	subdomain, _, recordName := s.resolveNames(ctx, serverID, opts.Subdomain, existing, cfg)

	client, err := s.provider(cfg)
	if err != nil {
		return nil, err
	}

	rec := &Record{
		ServerID:  serverID,
		Provider:  cfg.Provider,
		Name:      recordName,
		Subdomain: subdomain,
		Target:    target,
		Port:      port,
		Priority:  priority,
		Weight:    weight,
		TTL:       cfg.TTL,
		Zone:      cfg.Zone,
	}

	// Save the address independently of remote publication and its tracking row.
	if err := s.saveSubdomain(ctx, serverID, subdomain); err != nil {
		return nil, err
	}
	if existing != nil && (existing.Provider != cfg.Provider || existing.Zone != cfg.Zone) {
		if err := s.remove(ctx, serverID); err != nil {
			return nil, fmt.Errorf("remove previous provider record: %w", err)
		}
		existing = nil
	}
	recordID, err := client.upsert(ctx, existing, rec)
	if err != nil {
		return nil, fmt.Errorf("publish DNS record: %w", err)
	}
	if recordID == "" {
		return nil, errors.New("DNS provider returned an empty record ID")
	}
	rec.RecordID = recordID

	if err := s.upsertRecordRow(ctx, rec); err != nil {
		return nil, fmt.Errorf("store dns record: %w", err)
	}

	return rec, nil
}

// Remove deletes the SRV record for a server if one was published.
func (s *Service) Remove(ctx context.Context, serverID string) error {
	unlock := s.lockServer(serverID)
	defer unlock()
	return s.remove(ctx, serverID)
}

func (s *Service) remove(ctx context.Context, serverID string) error {
	row, err := s.GetRecord(ctx, serverID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read DNS record: %w", err)
	}
	// Read the record's original provider credentials, even when publishing is
	// disabled or a different provider/zone has since been selected.
	values, err := s.readConfigSettings(ctx)
	if err != nil {
		return err
	}
	values[KeyProvider] = row.Provider
	cfg := buildConfig(values)
	cfg.Zone = row.Zone
	cfg.Domain = row.JoinAddress()
	if !cfg.hasCredentials() {
		return ErrNotConfigured
	}
	client, err := s.provider(cfg)
	if err != nil {
		return err
	}
	if err := client.remove(ctx, row); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM dns_records WHERE server_id = ?`, serverID)
	return err
}

// GetSubdomain returns the saved address, including while a server is stopped.
func (s *Service) GetSubdomain(ctx context.Context, serverID string) (string, error) {
	var sub string
	err := s.db.QueryRowContext(ctx, `SELECT subdomain FROM server_dns_preferences WHERE server_id = ?`, serverID).Scan(&sub)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sub, err
}

func (s *Service) saveSubdomain(ctx context.Context, serverID, subdomain string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO server_dns_preferences (server_id, subdomain) VALUES (?, ?)
		ON CONFLICT(server_id) DO UPDATE SET subdomain = excluded.subdomain`, serverID, subdomain)
	return err
}

// List returns the records currently tracked in the database, newest first.
func (s *Service) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, provider, updated_at
		 FROM dns_records ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list dns records: %w", err)
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Subdomain, &r.Target, &r.Port,
			&r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.Provider, &r.UpdatedAt); err != nil {
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
		`SELECT server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, provider, updated_at
		 FROM dns_records WHERE server_id = ?`, serverID).
		Scan(&r.ServerID, &r.RecordID, &r.Name, &r.Subdomain, &r.Target, &r.Port, &r.Priority, &r.Weight, &r.TTL, &r.Zone, &r.Provider, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetConfig reads the current DNS settings and returns safe metadata for callers.
func (s *Service) GetConfig(ctx context.Context) (*ConfigInfo, error) {
	values, err := s.readConfigSettings(ctx)
	if err != nil {
		return nil, err
	}
	cfg := buildConfig(values)
	return &ConfigInfo{
		Provider:   cfg.Provider,
		Configured: cfg.Publish && cfg.hasCredentials(),
		Publish:    cfg.Publish,
		Domain:     cfg.Domain,
		Zone:       cfg.Zone,
		HasToken:   cfg.APIToken != "",
		Host:       cfg.Host,
		Service:    cfg.Service,
		Proto:      cfg.Proto,
		TTL:        cfg.TTL,
		Priority:   cfg.Priority,
		Weight:     cfg.Weight,
	}, nil
}

// VerifyOptions tests supplied credentials, falling back to the selected
// provider's saved settings. Testing is available with publishing disabled.
type VerifyOptions struct {
	Provider     string `json:"provider"`
	APIToken     string `json:"api_token"`
	APISecret    string `json:"api_secret"`
	SessionToken string `json:"session_token"`
	Zone         string `json:"zone"`
	Domain       string `json:"domain"`
}

func (s *Service) Verify(ctx context.Context, token, zone, domain string) (*VerifyResult, error) {
	return s.VerifyWithOptions(ctx, VerifyOptions{APIToken: token, Zone: zone, Domain: domain})
}

func (s *Service) VerifyWithOptions(ctx context.Context, opts VerifyOptions) (*VerifyResult, error) {
	opts.Provider = strings.ToLower(strings.TrimSpace(opts.Provider))
	opts.APIToken = strings.TrimSpace(opts.APIToken)
	opts.APISecret = strings.TrimSpace(opts.APISecret)
	opts.SessionToken = strings.TrimSpace(opts.SessionToken)
	opts.Domain = strings.ToLower(strings.TrimSpace(opts.Domain))
	opts.Zone = strings.TrimSpace(opts.Zone)
	values, err := s.readConfigSettings(ctx)
	if err != nil {
		return nil, err
	}
	if opts.Provider != "" {
		values[KeyProvider] = opts.Provider
	}
	cfg := buildConfig(values)
	if opts.APIToken != "" {
		cfg.APIToken = opts.APIToken
	}
	if opts.APISecret != "" {
		cfg.APISecret = opts.APISecret
	}
	if opts.SessionToken != "" {
		cfg.SessionToken = opts.SessionToken
	}
	if opts.Domain != "" {
		cfg.Domain = strings.TrimSuffix(opts.Domain, ".")
	}
	if opts.Zone != "" {
		cfg.Zone = opts.Zone
	} else if (cfg.Provider == "digitalocean" || cfg.Provider == "porkbun") && strings.TrimSpace(values["dns_"+cfg.Provider+"_zone"]) == "" {
		cfg.Zone = cfg.Domain
	}
	if cfg.Provider == "route53" {
		cfg.Zone = strings.TrimPrefix(cfg.Zone, "/hostedzone/")
	}
	if !cfg.hasCredentials() {
		return &VerifyResult{Message: "Provider credentials, zone, and domain are required"}, nil
	}
	client, err := s.provider(cfg)
	if err != nil {
		return &VerifyResult{Message: err.Error()}, nil
	}
	info, err := client.verify(ctx)
	if err != nil {
		return &VerifyResult{Message: fmt.Sprintf("DNS verification failed: %v", err)}, nil
	}
	msg := fmt.Sprintf("Successfully verified zone %q (status: %s)", info.Name, info.Status)
	if !strings.EqualFold(cfg.Domain, info.Name) && !strings.HasSuffix(strings.ToLower(cfg.Domain), "."+strings.ToLower(info.Name)) {
		return &VerifyResult{ZoneName: info.Name, Status: info.Status, Message: fmt.Sprintf("Configured domain %q does not belong to zone %q", cfg.Domain, info.Name)}, nil
	}
	return &VerifyResult{OK: true, ZoneName: info.Name, Status: info.Status, Message: msg}, nil
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
		`INSERT INTO dns_records (server_id, record_id, name, subdomain, target, port, priority, weight, ttl, zone, provider, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		   provider=excluded.provider,
		   updated_at=excluded.updated_at`,
		r.ServerID, r.RecordID, r.Name, r.Subdomain, r.Target, r.Port, r.Priority, r.Weight, r.TTL, r.Zone, r.Provider, now)
	return err
}

// config is the effective DNS publishing configuration for one operation.
type config struct {
	Provider     string
	APISecret    string
	SessionToken string
	Publish      bool
	Domain       string
	Zone         string
	APIToken     string
	Host         string
	Service      string
	Proto        string
	TTL          int
	Priority     int
	Weight       int
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
	values, err := s.readConfigSettings(ctx)
	if err != nil {
		return nil, err
	}

	cfg := buildConfig(values)
	if !cfg.Publish {
		return nil, ErrNotConfigured
	}
	if !cfg.hasCredentials() {
		return nil, ErrNotConfigured
	}
	return cfg, nil
}

// buildConfig turns raw settings values into a config, applying defaults.
func buildConfig(values map[string]string) *config {
	cfg := &config{
		Provider: strings.ToLower(strings.TrimSpace(values[KeyProvider])),
		Publish:  strings.EqualFold(values[KeyPublish], "true"),
		Domain:   strings.TrimSuffix(strings.ToLower(strings.TrimSpace(values[KeyDomain])), "."),
		Zone:     strings.TrimSpace(values[KeyZone]),
		APIToken: strings.TrimSpace(values[KeyAPIToken]),
		Host:     strings.TrimSpace(values[KeyHost]),
		Service:  strings.TrimSpace(values[KeyService]),
		Proto:    strings.TrimSpace(values[KeyProto]),
		TTL:      defaultTTL,
		Priority: defaultPriority,
		Weight:   defaultWeight,
	}
	if cfg.Provider == "" {
		cfg.Provider = "cloudflare"
	}
	if cfg.Provider != "cloudflare" {
		prefix := "dns_" + cfg.Provider
		cfg.APIToken = strings.TrimSpace(values[prefix+"_api_token"])
		cfg.APISecret = strings.TrimSpace(values[prefix+"_api_secret"])
		cfg.SessionToken = strings.TrimSpace(values[prefix+"_session_token"])
		cfg.Zone = strings.TrimSpace(values[prefix+"_zone"])
		if cfg.Zone == "" && (cfg.Provider == "digitalocean" || cfg.Provider == "porkbun") {
			cfg.Zone = cfg.Domain
		}
	}
	if cfg.Provider == "route53" {
		cfg.Zone = strings.TrimPrefix(cfg.Zone, "/hostedzone/")
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
	if cfg.Provider == "porkbun" && cfg.TTL < 600 {
		cfg.TTL = 600
	}
	if cfg.Provider == "digitalocean" && cfg.TTL < 30 {
		cfg.TTL = 30
	}
	return cfg
}

func (c *config) hasCredentials() bool {
	if c.Domain == "" || c.Zone == "" || c.APIToken == "" {
		return false
	}
	switch c.Provider {
	case "cloudflare", "digitalocean":
		return true
	case "porkbun", "route53":
		return c.APISecret != ""
	default:
		return false
	}
}

func (s *Service) readConfigSettings(ctx context.Context) (map[string]string, error) {
	keys := []string{KeyProvider, KeyPublish, KeyDomain, KeyZone, KeyAPIToken, KeyHost, KeyService, KeyProto, KeyTTL, KeyPriority, KeyWeight}
	for _, provider := range []string{"digitalocean", "porkbun", "route53"} {
		for _, field := range []string{"api_token", "api_secret", "session_token", "zone"} {
			keys = append(keys, "dns_"+provider+"_"+field)
		}
	}
	return s.readSettings(ctx, keys...)
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
