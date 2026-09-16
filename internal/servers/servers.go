// Package servers implements server record CRUD and container orchestration.
package servers

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
	"github.com/mcm-panel/mcm/internal/proxy"
)

// dockerRuntime is the subset of the Docker manager used by the server store.
// It is an interface so tests can substitute a fake runtime.
type dockerRuntime interface {
	Ping(ctx context.Context) error
	RuntimeStatus(ctx context.Context) docker.RuntimeStatus
	Remove(ctx context.Context, containerID string) error
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	Kill(ctx context.Context, containerID string) error
	Status(ctx context.Context, containerID string) (string, error)
	Inspect(ctx context.Context, containerID string) (docker.ContainerState, error)
	Exists(ctx context.Context, containerID string) (bool, error)
	Logs(ctx context.Context, containerID string, follow bool) (io.ReadCloser, error)
	SendConsole(ctx context.Context, containerID, command string) error
	Create(ctx context.Context, opts docker.CreateOpts) (string, error)
	HostAddress() string
}

var _ dockerRuntime = (*docker.Manager)(nil)

// Server state values.
const (
	StateStopped  = "stopped"
	StateStarting = "starting"
	StateRunning  = "running"
	StateStopping = "stopping"
	StateError    = "error"
)

// ErrNotFound is returned when a server id does not exist.
var ErrNotFound = errors.New("server not found")

// ErrInvalidJar is returned when a requested jar type, version, or build cannot
// be resolved/validated because it is unsupported or unknown.
var ErrInvalidJar = errors.New("invalid or unsupported jar")

// ErrUpstream is returned when resolving jar metadata fails because an upstream
// provider is unreachable or misbehaving.
var ErrUpstream = errors.New("upstream provider error")

// ErrPortInUse is returned when the requested host port is already allocated.
var ErrPortInUse = errors.New("port already in use")

// Server is the public representation of a server record.
type Server struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ServerType string `json:"server_type"`
	Version    string `json:"version"`
	Build      string `json:"build,omitempty"`
	RAMMB      int    `json:"ram_mb"`
	// CPULimit is the CPU quota in cores (0 = no limit). MemoryLimitMB is an
	// explicit memory cap in MB; 0 falls back to the RAM-derived default.
	CPULimit      float64     `json:"cpu_limit"`
	MemoryLimitMB int         `json:"memory_limit_mb"`
	HostPort      int         `json:"host_port"`
	JavaVersion   int         `json:"java_version"`
	ExtraPorts    []ExtraPort `json:"extra_ports"`
	ContainerID   string      `json:"container_id,omitempty"`
	State         string      `json:"state"`
	// Backup settings. BackupEnabled defaults to true; BackupIntervalMinutes
	// is the minutes between automatic backups (default 720).
	BackupEnabled         bool     `json:"backup_enabled"`
	BackupIntervalMinutes int      `json:"backup_interval_minutes"`
	StartedAt             string   `json:"started_at,omitempty"`
	UptimeSeconds         int64    `json:"uptime_seconds,omitempty"`
	NeedsRebuild          bool     `json:"needs_rebuild"`
	RebuildReasons        []string `json:"rebuild_reasons,omitempty"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
}

// ExtraPort describes an additional port published for a server beyond the
// primary game port (e.g. a WebUI or Bedrock/Geyser adapter).
type ExtraPort struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"` // tcp or udp
}

// CreateInput is the payload for creating a server.
type CreateInput struct {
	Name          string       `json:"name"`
	ServerType    jars.JarType `json:"server_type"`
	Version       string       `json:"version"`
	Build         string       `json:"build,omitempty"`
	RAMMB         int          `json:"ram_mb"`
	HostPort      int          `json:"host_port,omitempty"`
	CPULimit      float64      `json:"cpu_limit"`
	MemoryLimitMB int          `json:"memory_limit_mb"`
	JavaVersion   int          `json:"java_version"`
	ExtraPorts    []ExtraPort  `json:"extra_ports"`
}

// UpdateInput is the payload for updating a server.
type UpdateInput struct {
	Name                  *string       `json:"name"`
	ServerType            *jars.JarType `json:"server_type"`
	Version               *string       `json:"version"`
	Build                 *string       `json:"build"`
	RAMMB                 *int          `json:"ram_mb"`
	HostPort              *int          `json:"host_port"`
	CPULimit              *float64      `json:"cpu_limit"`
	MemoryLimitMB         *int          `json:"memory_limit_mb"`
	BackupEnabled         *bool         `json:"backup_enabled"`
	BackupIntervalMinutes *int          `json:"backup_interval_minutes"`
	JavaVersion           *int          `json:"java_version"`
	ExtraPorts            *[]ExtraPort  `json:"extra_ports"`
}

// CopyInput is the payload for duplicating an existing server.
type CopyInput struct {
	Name              string   `json:"name"`
	HostPort          int      `json:"host_port,omitempty"`
	RAMMB             int      `json:"ram_mb,omitempty"`
	CPULimit          float64  `json:"cpu_limit,omitempty"`
	MemoryLimitMB     int      `json:"memory_limit_mb,omitempty"`
	JavaVersion       int      `json:"java_version,omitempty"`
	IncludeWorld      *bool    `json:"include_world,omitempty"`
	IncludeConfig     *bool    `json:"include_config,omitempty"`
	IncludePlugins    *bool    `json:"include_plugins,omitempty"`
	IncludeMods       *bool    `json:"include_mods,omitempty"`
	IncludePlayerData *bool    `json:"include_player_data,omitempty"`
	IncludeLogs       *bool    `json:"include_logs,omitempty"`
	CustomExcludes    []string `json:"custom_excludes,omitempty"`
}

// encodeExtraPorts serializes an extra-ports slice for storage. Nil or empty
// slices produce "[]" so the DB column stays a valid empty JSON array.
func encodeExtraPorts(ports []ExtraPort) string {
	if len(ports) == 0 {
		return "[]"
	}
	b, err := json.Marshal(ports)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// decodeExtraPorts parses stored JSON back into an extra-ports slice. It always
// returns a non-nil slice so the API serializes as [] when empty.
func decodeExtraPorts(data string) []ExtraPort {
	out := make([]ExtraPort, 0)
	if data == "" {
		return out
	}
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return make([]ExtraPort, 0)
	}
	return out
}

// ContainerConfig captures the container-dependent configuration applied to
// the container at creation time.
type ContainerConfig struct {
	ServerType    string      `json:"server_type"`
	Version       string      `json:"version"`
	Build         string      `json:"build"`
	RAMMB         int         `json:"ram_mb"`
	CPULimit      float64     `json:"cpu_limit"`
	MemoryLimitMB int         `json:"memory_limit_mb"`
	HostPort          int         `json:"host_port"`
	JavaVersion       int         `json:"java_version"`
	ExtraPorts        []ExtraPort `json:"extra_ports"`
	EntrypointVersion int         `json:"entrypoint_version,omitempty"`
}

func encodeContainerConfig(c ContainerConfig) string {
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(b)
}

func currentContainerConfig(srv *Server) ContainerConfig {
	ports := make([]ExtraPort, len(srv.ExtraPorts))
	copy(ports, srv.ExtraPorts)
	return ContainerConfig{
		ServerType:    srv.ServerType,
		Version:       srv.Version,
		Build:         srv.Build,
		RAMMB:         srv.RAMMB,
		CPULimit:      srv.CPULimit,
		MemoryLimitMB: srv.MemoryLimitMB,
		HostPort:          srv.HostPort,
		JavaVersion:       srv.JavaVersion,
		ExtraPorts:        ports,
		EntrypointVersion: 2,
	}
}

func extraPortsEqual(a, b []ExtraPort) bool {
	if len(a) != len(b) {
		return false
	}
	mapA := make(map[string]ExtraPort, len(a))
	for _, p := range a {
		mapA[p.ID] = p
	}
	for _, p := range b {
		other, ok := mapA[p.ID]
		if !ok {
			return false
		}
		if other.HostPort != p.HostPort || other.ContainerPort != p.ContainerPort ||
			strings.ToLower(other.Protocol) != strings.ToLower(p.Protocol) || other.Description != p.Description {
			return false
		}
	}
	return true
}

func checkRebuildNeeded(srv *Server, rawConfig string) (bool, []string) {
	if srv.ContainerID == "" {
		return false, nil
	}
	if rawConfig == "" {
		return false, nil
	}
	var applied ContainerConfig
	if err := json.Unmarshal([]byte(rawConfig), &applied); err != nil {
		return false, nil
	}
	var reasons []string
	if srv.RAMMB != applied.RAMMB {
		reasons = append(reasons, fmt.Sprintf("RAM changed from %d MB to %d MB", applied.RAMMB, srv.RAMMB))
	}
	if srv.CPULimit != applied.CPULimit {
		reasons = append(reasons, fmt.Sprintf("CPU limit changed from %.1f to %.1f cores", applied.CPULimit, srv.CPULimit))
	}
	if srv.MemoryLimitMB != applied.MemoryLimitMB {
		reasons = append(reasons, fmt.Sprintf("Memory limit changed from %d MB to %d MB", applied.MemoryLimitMB, srv.MemoryLimitMB))
	}
	if srv.HostPort != applied.HostPort {
		reasons = append(reasons, fmt.Sprintf("Host port changed from %d to %d", applied.HostPort, srv.HostPort))
	}
	if srv.ServerType != applied.ServerType {
		reasons = append(reasons, fmt.Sprintf("Server type changed from %s to %s", applied.ServerType, srv.ServerType))
	}
	if srv.Version != applied.Version {
		reasons = append(reasons, fmt.Sprintf("Version changed from %s to %s", applied.Version, srv.Version))
	}
	if srv.Build != applied.Build {
		reasons = append(reasons, fmt.Sprintf("Build changed from %s to %s", applied.Build, srv.Build))
	}
	if applied.JavaVersion > 0 && srv.JavaVersion > 0 && srv.JavaVersion != applied.JavaVersion {
		reasons = append(reasons, fmt.Sprintf("Java version changed from %d to %d", applied.JavaVersion, srv.JavaVersion))
	}
	if !extraPortsEqual(srv.ExtraPorts, applied.ExtraPorts) {
		reasons = append(reasons, "Additional ports configuration changed")
	}
	if applied.EntrypointVersion < 2 {
		reasons = append(reasons, "Container runtime script updated to support clean process exit on crash")
	}
	return len(reasons) > 0, reasons
}

// InstallResult describes a server's resolved install configuration.
type InstallResult struct {
	Server   Server        `json:"server"`
	Resolved jars.Resolved `json:"resolved"`
	DataDir  string        `json:"data_dir"`
}

// Store coordinates the database, docker, jar resolution, and port allocation.
type Store struct {
	db      *sql.DB
	docker  dockerRuntime
	jars    *jars.Resolver
	ports   *ports.Pool
	dataDir string
	// dataDirHost is the host-visible absolute path used as the Docker bind
	// source. When empty it falls back to dataDir (bare-metal installs, where
	// the process path and the host path are the same).
	dataDirHost string
	// dns optionally publishes/removes SRV records as servers start and stop.
	dns dns.Publisher

	proxyMu         sync.Mutex
	proxy           *proxy.Service
	updatesMu       sync.RWMutex
	updatesCache    map[string]*ServerModUpdatesResponse
	updatesFlightMu sync.Mutex
	updatesInFlight map[string]*updatesFlightCall
}

type updatesFlightCall struct {
	wg   sync.WaitGroup
	resp *ServerModUpdatesResponse
	err  error
}

// NewStore wires the server store together.
func NewStore(handle *db.Store, dm *docker.Manager, jr *jars.Resolver, start, end int, dataDir, dataDirHost string) *Store {
	return &Store{
		db:              handle.DB,
		docker:          dm,
		jars:            jr,
		ports:           ports.NewPool(handle.DB, start, end),
		dataDir:         dataDir,
		dataDirHost:     dataDirHost,
		updatesInFlight: make(map[string]*updatesFlightCall),
	}
}

// SetDNS wires a DNS publisher so Start/Stop publish and remove SRV records.
func (s *Store) SetDNS(d dns.Publisher) {
	s.dns = d
}

// Reachable reports whether the underlying Docker daemon is responsive. It is
// used by the readiness probe.
func (s *Store) Reachable(ctx context.Context) error {
	return s.docker.Ping(ctx)
}

// DockerStatus returns the runtime health of the Docker host (daemon reachability
// and runtime-image presence) for diagnostics.
func (s *Store) DockerStatus(ctx context.Context) docker.RuntimeStatus {
	return s.docker.RuntimeStatus(ctx)
}

// Pool exposes the underlying port pool for the available-ports endpoint.
func (s *Store) Pool() *ports.Pool {
	return s.ports
}

// calcUptime computes the uptime duration in seconds for a running server.
func calcUptime(srv *Server) {
	if srv.State != StateRunning || srv.StartedAt == "" {
		srv.UptimeSeconds = 0
		if srv.State != StateRunning {
			srv.StartedAt = ""
		}
		return
	}
	t, err := time.Parse(time.RFC3339Nano, srv.StartedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, srv.StartedAt)
	}
	if err == nil {
		sec := int64(time.Since(t).Seconds())
		if sec < 0 {
			sec = 0
		}
		srv.UptimeSeconds = sec
	}
}

// List returns all servers ordered by creation time.
func (s *Store) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, server_type, version, COALESCE(build,''), ram_mb, cpu_limit, memory_limit_mb, host_port, COALESCE(extra_ports,'[]'), COALESCE(container_id,''), state, backup_enabled, backup_interval_minutes, created_at, updated_at, COALESCE(started_at,''), COALESCE(container_config,''), COALESCE(java_version, 21) FROM servers ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Initialize to an empty (non-nil) slice so an empty result serializes as
	// [] rather than null in the JSON API.
	out := make([]Server, 0)
	for rows.Next() {
		var srv Server
		var extra string
		var rawConfig string
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.ServerType, &srv.Version, &srv.Build, &srv.RAMMB, &srv.CPULimit, &srv.MemoryLimitMB, &srv.HostPort, &extra, &srv.ContainerID, &srv.State, &srv.BackupEnabled, &srv.BackupIntervalMinutes, &srv.CreatedAt, &srv.UpdatedAt, &srv.StartedAt, &rawConfig, &srv.JavaVersion); err != nil {
			return nil, err
		}
		srv.ExtraPorts = decodeExtraPorts(extra)
		calcUptime(&srv)
		srv.NeedsRebuild, srv.RebuildReasons = checkRebuildNeeded(&srv, rawConfig)
		out = append(out, srv)
	}
	return out, rows.Err()
}

// Get returns a single server by id.
func (s *Store) Get(ctx context.Context, id string) (Server, error) {
	var srv Server
	var extra string
	var rawConfig string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, server_type, version, COALESCE(build,''), ram_mb, cpu_limit, memory_limit_mb, host_port, COALESCE(extra_ports,'[]'), COALESCE(container_id,''), state, backup_enabled, backup_interval_minutes, created_at, updated_at, COALESCE(started_at,''), COALESCE(container_config,''), COALESCE(java_version, 21) FROM servers WHERE id = ?`, id).
		Scan(&srv.ID, &srv.Name, &srv.ServerType, &srv.Version, &srv.Build, &srv.RAMMB, &srv.CPULimit, &srv.MemoryLimitMB, &srv.HostPort, &extra, &srv.ContainerID, &srv.State, &srv.BackupEnabled, &srv.BackupIntervalMinutes, &srv.CreatedAt, &srv.UpdatedAt, &srv.StartedAt, &rawConfig, &srv.JavaVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	if err != nil {
		return Server{}, err
	}
	srv.ExtraPorts = decodeExtraPorts(extra)
	calcUptime(&srv)
	srv.NeedsRebuild, srv.RebuildReasons = checkRebuildNeeded(&srv, rawConfig)
	return srv, nil
}

// Create validates the requested jar, allocates a host port, and persists a new
// stopped server. The container itself is created lazily on install/start.
func (s *Store) Create(ctx context.Context, in CreateInput) (Server, error) {
	if err := validateLimits(in.CPULimit, in.MemoryLimitMB); err != nil {
		return Server{}, err
	}
	resolved, err := s.jars.Validate(ctx, in.ServerType, in.Version, in.Build)
	if err != nil {
		if errors.Is(err, jars.ErrUpstream) {
			return Server{}, fmt.Errorf("%w: validate jar: %v", ErrUpstream, err)
		}
		return Server{}, fmt.Errorf("%w: validate jar: %v", ErrInvalidJar, err)
	}
	var port int
	if in.HostPort > 0 {
		if in.HostPort < 1 || in.HostPort > 65535 {
			return Server{}, fmt.Errorf("host_port must be between 1 and 65535")
		}
		if err := s.ensurePortFree(ctx, "", in.HostPort); err != nil {
			return Server{}, err
		}
		port = in.HostPort
	} else {
		var err error
		port, err = s.ports.Allocate(ctx)
		if err != nil {
			return Server{}, fmt.Errorf("allocate port: %w", err)
		}
	}

	javaVer := in.JavaVersion
	if javaVer <= 0 {
		javaVer = jars.RecommendJavaVersion(resolved.Version)
		if javaVer <= 0 {
			javaVer = 21
		}
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at, java_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?)`,
		id, in.Name, string(in.ServerType), resolved.Version, resolved.Build, in.RAMMB, in.CPULimit, in.MemoryLimitMB, port, encodeExtraPorts(in.ExtraPorts), StateStopped, now, now, javaVer)
	if err != nil {
		return Server{}, fmt.Errorf("insert server: %w", err)
	}
	return s.Get(ctx, id)
}

// Update applies non-nil fields from the input to a server record.
func (s *Store) Update(ctx context.Context, id string, in UpdateInput) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if in.Name != nil {
		srv.Name = *in.Name
	}
	if in.Version != nil && in.ServerType != nil {
		resolved, verr := s.jars.Validate(ctx, *in.ServerType, *in.Version, ptrStr(in.Build))
		if verr != nil {
			return Server{}, fmt.Errorf("validate jar: %w", verr)
		}
		srv.ServerType = string(*in.ServerType)
		srv.Version = resolved.Version
		srv.Build = resolved.Build
	} else if in.Version != nil {
		resolved, verr := s.jars.Validate(ctx, jars.JarType(srv.ServerType), *in.Version, ptrStr(in.Build))
		if verr != nil {
			return Server{}, fmt.Errorf("validate jar: %w", verr)
		}
		srv.Version = resolved.Version
		srv.Build = resolved.Build
	} else if in.Build != nil {
		resolved, verr := s.jars.Validate(ctx, jars.JarType(srv.ServerType), srv.Version, *in.Build)
		if verr != nil {
			return Server{}, fmt.Errorf("validate jar: %w", verr)
		}
		srv.Build = resolved.Build
	}
	if in.RAMMB != nil {
		srv.RAMMB = *in.RAMMB
	}
	if in.HostPort != nil && *in.HostPort != srv.HostPort {
		port := *in.HostPort
		if port < 1 || port > 65535 {
			return Server{}, fmt.Errorf("host_port must be between 1 and 65535")
		}
		if err := s.ensurePortFree(ctx, id, port); err != nil {
			return Server{}, err
		}
		srv.HostPort = port
		// Docker binds host ports at container creation, so a changed primary
		// game port requires a fresh container. Detach the existing one so the
		// next Start provisions a new container bound to the new port.
		if srv.ContainerID != "" {
			_ = s.docker.Remove(ctx, srv.ContainerID)
			srv.ContainerID = ""
			srv.State = StateStopped
			_, _ = s.db.ExecContext(ctx, `UPDATE servers SET container_config=NULL WHERE id=?`, id)
		}
	}
	if in.CPULimit != nil {
		if *in.CPULimit < 0 {
			return Server{}, fmt.Errorf("cpu_limit must be non-negative")
		}
		srv.CPULimit = *in.CPULimit
	}
	if in.MemoryLimitMB != nil {
		if *in.MemoryLimitMB < 0 {
			return Server{}, fmt.Errorf("memory_limit_mb must be non-negative")
		}
		srv.MemoryLimitMB = *in.MemoryLimitMB
	}
	if in.BackupEnabled != nil {
		srv.BackupEnabled = *in.BackupEnabled
	}
	if in.BackupIntervalMinutes != nil {
		srv.BackupIntervalMinutes = *in.BackupIntervalMinutes
	}
	if in.ExtraPorts != nil {
		srv.ExtraPorts = *in.ExtraPorts
	}
	if in.JavaVersion != nil && *in.JavaVersion > 0 {
		srv.JavaVersion = *in.JavaVersion
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE servers SET name=?, server_type=?, version=?, build=?, ram_mb=?, host_port=?, cpu_limit=?, memory_limit_mb=?, backup_enabled=?, backup_interval_minutes=?, extra_ports=?, container_id=?, state=?, updated_at=?, java_version=? WHERE id=?`,
		srv.Name, srv.ServerType, srv.Version, srv.Build, srv.RAMMB, srv.HostPort, srv.CPULimit, srv.MemoryLimitMB, srv.BackupEnabled, srv.BackupIntervalMinutes, encodeExtraPorts(srv.ExtraPorts), srv.ContainerID, srv.State, now, srv.JavaVersion, id)
	if err != nil {
		return Server{}, fmt.Errorf("update server: %w", err)
	}
	return s.Get(ctx, id)
}

// ensurePortFree verifies that port is not already assigned to another server.
// In the context of the port pool, a port that was previously allocated to this
// server is freed when the old host_port is overwritten below.
func (s *Store) ensurePortFree(ctx context.Context, id string, port int) error {
	var other string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM servers WHERE host_port = ? AND id != ?`, port, id).Scan(&other)
	if err == nil {
		return fmt.Errorf("%w: host_port %d is already in use by another server", ErrPortInUse, port)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check host_port: %w", err)
	}
	return nil
}

// Delete removes a server record and its container if one exists.
func (s *Store) Delete(ctx context.Context, id string) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if srv.ContainerID != "" {
		_ = s.docker.Remove(ctx, srv.ContainerID)
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	if s.dns != nil {
		_ = s.dns.Remove(ctx, id)
	}
	return nil
}

// Start ensures a container exists then starts it.
func (s *Store) Start(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	srv, err = s.ensureContainer(ctx, srv)
	if err != nil {
		return Server{}, err
	}
	if err := s.setState(ctx, id, StateStarting); err != nil {
		return Server{}, err
	}
	if err := s.docker.Start(ctx, srv.ContainerID); err != nil {
		_ = s.setState(ctx, id, StateError)
		return Server{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.setStateWithStartedAt(ctx, id, StateRunning, &now); err != nil {
		return Server{}, err
	}
	if s.dns != nil {
		_ = s.dns.Upsert(ctx, id, "", srv.HostPort)
	}
	return s.Get(ctx, id)
}

// stopCommandFor returns the graceful console stop command for a given server type.
func stopCommandFor(serverType string) string {
	switch strings.ToLower(serverType) {
	case "bungeecord", "waterfall":
		return "end"
	case "velocity":
		return "shutdown"
	default:
		return "stop"
	}
}

// Stop stops a running container if one exists.
func (s *Store) Stop(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if srv.State == StateStopped && srv.ContainerID == "" {
		return srv, nil
	}
	if err := s.setState(ctx, id, StateStopping); err != nil {
		return Server{}, err
	}
	if srv.ContainerID != "" {
		insp, err := s.docker.Inspect(ctx, srv.ContainerID)
		if err == nil && insp.Status == StateRunning {
			stopCmd := stopCommandFor(srv.ServerType)
			sendErr := s.docker.SendConsole(ctx, srv.ContainerID, stopCmd)
			if sendErr != nil {
				// Console pipe not available; fall back to docker Stop directly
				if err := s.docker.Stop(ctx, srv.ContainerID, 15*time.Second); err != nil {
					_ = s.setState(ctx, id, StateError)
					return Server{}, err
				}
			} else {
				// Wait for container to cleanly exit after console stop
				waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
				defer cancel()
				ticker := time.NewTicker(250 * time.Millisecond)
				defer ticker.Stop()

				stopped := false
				for !stopped {
					select {
					case <-waitCtx.Done():
						stopped = false
						goto doneWaiting
					case <-ticker.C:
						insp, err := s.docker.Inspect(ctx, srv.ContainerID)
						if err != nil || insp.Status != StateRunning {
							stopped = true
							goto doneWaiting
						}
					}
				}
			doneWaiting:
				if !stopped {
					// Fallback to docker Stop if graceful console stop timed out
					stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
					defer stopCancel()
					if err := s.docker.Stop(stopCtx, srv.ContainerID, 10*time.Second); err != nil {
						_ = s.setState(stopCtx, id, StateError)
						return Server{}, err
					}
				}
			}
		} else if err == nil && insp.Status != StateStopped && insp.Status != "exited" {
			stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer stopCancel()
			if err := s.docker.Stop(stopCtx, srv.ContainerID, 10*time.Second); err != nil {
				_ = s.setState(stopCtx, id, StateError)
				return Server{}, err
			}
		}
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cleanupCancel()
	if err := s.setState(cleanupCtx, id, StateStopped); err != nil {
		return Server{}, err
	}
	if s.dns != nil {
		_ = s.dns.Remove(cleanupCtx, id)
	}
	return s.Get(cleanupCtx, id)
}

// Kill force-stops a running container without waiting for a graceful
// shutdown. It is the hard-stop counterpart to Stop and is useful when a
// server is hung and will not stop cleanly.
func (s *Store) Kill(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if err := s.setState(ctx, id, StateStopping); err != nil {
		return Server{}, err
	}
	if srv.ContainerID != "" {
		if err := s.docker.Kill(ctx, srv.ContainerID); err != nil {
			_ = s.setState(ctx, id, StateError)
			return Server{}, err
		}
	}
	if err := s.setState(ctx, id, StateStopped); err != nil {
		return Server{}, err
	}
	if s.dns != nil {
		_ = s.dns.Remove(ctx, id)
	}
	return s.Get(ctx, id)
}

// Restart restarts a running container, starting it if it is stopped.
func (s *Store) Restart(ctx context.Context, id string) (Server, error) {
	if _, err := s.Stop(ctx, id); err != nil {
		return Server{}, err
	}
	return s.Start(ctx, id)
}

// Recreate tears down an existing server container and clears its
// container_id so the next Start provisions a fresh container. This is used to
// rebind an existing server onto a new runtime image (e.g. the switch to
// itzg/minecraft-server) without losing its data directory or settings.
func (s *Store) Recreate(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if srv.ContainerID != "" {
		// Best-effort: the container may already be gone; we still clear the
		// recorded id so ensureContainer rebuilds it on next start.
		_ = s.docker.Remove(ctx, srv.ContainerID)
	}
	_, err = s.db.ExecContext(ctx, `UPDATE servers SET container_id='', container_config=NULL, state=?, started_at=NULL, updated_at=? WHERE id=?`,
		StateStopped, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return Server{}, err
	}
	if s.dns != nil {
		_ = s.dns.Remove(ctx, id)
	}
	return s.Get(ctx, id)
}

// Status returns the current server state, reconciling from docker when a
// container exists.
func (s *Store) Status(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if srv.ContainerID == "" {
		return srv, nil
	}
	insp, err := s.docker.Inspect(ctx, srv.ContainerID)
	if err != nil {
		return srv, nil
	}
	mapped := mapDockerState(insp.Status, insp.ExitCode)
	if mapped != srv.State {
		if mapped == StateRunning {
			started := insp.StartedAt
			if started == "" {
				started = time.Now().UTC().Format(time.RFC3339)
			}
			_ = s.setStateWithStartedAt(ctx, id, mapped, &started)
		} else {
			_ = s.setState(ctx, id, mapped)
			if s.dns != nil && (mapped == StateStopped || mapped == StateError) {
				_ = s.dns.Remove(ctx, id)
			}
		}
		return s.Get(ctx, id)
	}
	if mapped == StateRunning && srv.StartedAt == "" && insp.StartedAt != "" {
		_ = s.setStateWithStartedAt(ctx, id, mapped, &insp.StartedAt)
		return s.Get(ctx, id)
	}
	return srv, nil
}

// Console streams container logs for a server.
func (s *Store) Console(ctx context.Context, id string, follow bool) (io.ReadCloser, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if srv.ContainerID == "" {
		srv, err = s.ensureContainer(ctx, srv)
		if err != nil {
			return nil, err
		}
	}
	return s.docker.Logs(ctx, srv.ContainerID, follow)
}

// Install resolves and (for POST) provisions the server's container. GET returns
// the resolution without creating anything.
func (s *Store) Install(ctx context.Context, id string, provision bool) (InstallResult, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return InstallResult{}, err
	}
	resolved, err := s.jars.Validate(ctx, jars.JarType(srv.ServerType), srv.Version, srv.Build)
	if err != nil {
		if errors.Is(err, jars.ErrUpstream) {
			return InstallResult{}, fmt.Errorf("%w: validate jar: %v", ErrUpstream, err)
		}
		return InstallResult{}, fmt.Errorf("%w: validate jar: %v", ErrInvalidJar, err)
	}
	if provision {
		srv, err = s.ensureContainer(ctx, srv)
		if err != nil {
			return InstallResult{}, err
		}
	}
	return InstallResult{Server: srv, Resolved: resolved, DataDir: s.dataPath(srv.ID)}, nil
}

func (s *Store) ensureContainer(ctx context.Context, srv Server) (Server, error) {
	if srv.ContainerID != "" {
		// Verify the recorded container still exists. It may have been removed
		// outside MCM (e.g. `docker rm`); if so, drop the stale id and create a
		// replacement so the server keeps working instead of failing forever.
		exists, err := s.docker.Exists(ctx, srv.ContainerID)
		if err != nil {
			return Server{}, err
		}
		if exists {
			return srv, nil
		}
		if err := s.clearContainerID(ctx, srv.ID); err != nil {
			return Server{}, err
		}
		srv.ContainerID = ""
	}

	// Ensure server.jar exists in data directory (download if missing)
	dataDir := s.dataPath(srv.ID)
	serverJar := filepath.Join(dataDir, "server.jar")
	runSh := filepath.Join(dataDir, "run.sh")
	if _, err := os.Stat(serverJar); os.IsNotExist(err) {
		if _, errSh := os.Stat(runSh); os.IsNotExist(errSh) {
			if s.jars != nil && srv.ServerType != "custom" {
				jt, err := jars.ParseJarType(srv.ServerType)
				if err == nil {
					_ = s.jars.DownloadServerJar(ctx, jt, srv.Version, srv.Build, dataDir)
				}
			}
		}
	}

	cid, err := s.docker.Create(ctx, docker.CreateOpts{
		ID:            srv.ID,
		HostPort:      srv.HostPort,
		ExtraPorts:    toDockerExtras(srv.ExtraPorts),
		DataDir:       s.dockerDataPath(srv.ID),
		ServerType:    srv.ServerType,
		Version:       srv.Version,
		Build:         srv.Build,
		RAMMB:         srv.RAMMB,
		CPULimit:      srv.CPULimit,
		MemoryLimitMB: srv.MemoryLimitMB,
		JavaVersion:   srv.JavaVersion,
	})
	if err != nil {
		return Server{}, err
	}
	cfgStr := encodeContainerConfig(currentContainerConfig(&srv))
	_, err = s.db.ExecContext(ctx, `UPDATE servers SET container_id=?, container_config=?, updated_at=? WHERE id=?`, cid, cfgStr, time.Now().UTC().Format(time.RFC3339), srv.ID)
	if err != nil {
		return Server{}, err
	}
	srv.ContainerID = cid
	srv.NeedsRebuild = false
	srv.RebuildReasons = nil
	return srv, nil
}

func toDockerExtras(ports []ExtraPort) []docker.ExtraPort {
	out := make([]docker.ExtraPort, 0, len(ports))
	for _, p := range ports {
		out = append(out, docker.ExtraPort{
			ID:            p.ID,
			Description:   p.Description,
			HostPort:      p.HostPort,
			ContainerPort: p.ContainerPort,
			Protocol:      p.Protocol,
		})
	}
	return out
}

func (s *Store) dataPath(id string) string {
	return filepath.Join(s.dataDir, "servers", id)
}

// dockerDataPath returns the bind-mount source used when provisioning a server
// container. Docker resolves bind sources against the daemon host, so when MCM
// runs inside its own container (dataDir is a container path) the host-visible
// path must be used instead. On bare metal it is identical to dataPath.
func (s *Store) dockerDataPath(id string) string {
	if s.dataDirHost != "" {
		return filepath.Join(s.dataDirHost, "servers", id)
	}
	return s.dataPath(id)
}

func (s *Store) setState(ctx context.Context, id, state string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if state != StateRunning {
		_, err := s.db.ExecContext(ctx,
			`UPDATE servers SET state=?, started_at=NULL, updated_at=? WHERE id=?`, state, now, id)
		return err
	}
	return s.setStateWithStartedAt(ctx, id, state, &now)
}

func (s *Store) setStateWithStartedAt(ctx context.Context, id, state string, startedAt *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if startedAt == nil {
		_, err := s.db.ExecContext(ctx,
			`UPDATE servers SET state=?, started_at=NULL, updated_at=? WHERE id=?`, state, now, id)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET state=?, started_at=?, updated_at=? WHERE id=?`, state, *startedAt, now, id)
	return err
}

// clearContainerID forgets a server's recorded container id so the next
// ensureContainer provisions a fresh container. Used when the recorded
// container no longer exists (e.g. it was deleted outside MCM).
func (s *Store) clearContainerID(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET container_id='', container_config=NULL, updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// validateLimits rejects negative CPU or memory limits. A zero value means "no
// explicit limit" and is allowed.
func validateLimits(cpu float64, memoryMB int) error {
	if cpu < 0 {
		return fmt.Errorf("cpu_limit must be non-negative")
	}
	if memoryMB < 0 {
		return fmt.Errorf("memory_limit_mb must be non-negative")
	}
	return nil
}

func mapDockerState(state string, exitCode int) string {
	switch state {
	case "running":
		return StateRunning
	case "stopped", "exited", "dead":
		if exitCode != 0 && exitCode != 143 && exitCode != 130 {
			return StateError
		}
		return StateStopped
	case "created":
		return StateStopped
	case "restarting":
		return StateStarting
	case "paused":
		return StateRunning
	default:
		return StateError
	}
}

// Export streams a zip archive of the server's data directory to w.
func (s *Store) Export(ctx context.Context, id string, w io.Writer) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	_ = srv

	srcDir := s.dataPath(id)
	if fi, err := os.Stat(srcDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("server data directory %s is not accessible", srcDir)
	}

	zw := zip.NewWriter(w)
	err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Exclude socket files or ephemeral locks
		if d.Type()&fs.ModeSocket != 0 {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			hdr.Name += "/"
		} else {
			hdr.Method = zip.Deflate
		}
		writer, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if !d.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, f)
			_ = f.Close()
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// Copy creates a duplicate of an existing server with the specified configuration
// and file inclusion/exclusion options.
func (s *Store) Copy(ctx context.Context, id string, in CopyInput) (Server, error) {
	src, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}

	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Server{}, errors.New("name is required")
	}

	ramMB := in.RAMMB
	if ramMB <= 0 {
		ramMB = src.RAMMB
	}
	cpuLimit := in.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = src.CPULimit
	}
	memLimit := in.MemoryLimitMB
	if memLimit <= 0 {
		memLimit = src.MemoryLimitMB
	}
	if err := validateLimits(cpuLimit, memLimit); err != nil {
		return Server{}, err
	}

	var port int
	if in.HostPort > 0 {
		if in.HostPort < 1 || in.HostPort > 65535 {
			return Server{}, fmt.Errorf("host_port must be between 1 and 65535")
		}
		if err := s.ensurePortFree(ctx, "", in.HostPort); err != nil {
			return Server{}, err
		}
		port = in.HostPort
	} else {
		var err error
		port, err = s.ports.Allocate(ctx)
		if err != nil {
			return Server{}, fmt.Errorf("allocate port: %w", err)
		}
	}

	newID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	javaVer := in.JavaVersion
	if javaVer <= 0 {
		javaVer = src.JavaVersion
		if javaVer <= 0 {
			javaVer = 21
		}
	}

	// Persist server record in stopped state.
	// ExtraPorts are not automatically copied to avoid port conflicts with the source container.
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, backup_enabled, backup_interval_minutes, created_at, updated_at, java_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '[]', '', ?, ?, ?, ?, ?, ?)`,
		newID, in.Name, src.ServerType, src.Version, src.Build, ramMB, cpuLimit, memLimit, port, StateStopped, src.BackupEnabled, src.BackupIntervalMinutes, now, now, javaVer)
	if err != nil {
		return Server{}, fmt.Errorf("insert server: %w", err)
	}

	srcDir := s.dataPath(src.ID)
	dstDir := s.dataPath(newID)

	if err := s.copyServerFiles(ctx, srcDir, dstDir, in); err != nil {
		_ = os.RemoveAll(dstDir)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, newID)
		return Server{}, fmt.Errorf("copy server files: %w", err)
	}

	return s.Get(ctx, newID)
}

func (s *Store) copyServerFiles(ctx context.Context, srcDir, dstDir string, in CopyInput) error {
	fi, err := os.Stat(srcDir)
	if err != nil || !fi.IsDir() {
		return os.MkdirAll(dstDir, 0o755)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	includeWorld := in.IncludeWorld == nil || *in.IncludeWorld
	includeConfig := in.IncludeConfig == nil || *in.IncludeConfig
	includePlugins := in.IncludePlugins == nil || *in.IncludePlugins
	includeMods := in.IncludeMods == nil || *in.IncludeMods
	includePlayerData := in.IncludePlayerData == nil || *in.IncludePlayerData
	includeLogs := in.IncludeLogs != nil && *in.IncludeLogs
	customExcludes := in.CustomExcludes

	levelName := "world"
	if props, err := readProps(filepath.Join(srcDir, "server.properties")); err == nil {
		if ln := strings.TrimSpace(props["level-name"]); ln != "" {
			levelName = ln
		}
	}

	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		// Always exclude socket files and ephemeral lock files
		if d.Type()&fs.ModeSocket != 0 || d.Name() == "session.lock" || d.Name() == ".session.lock" {
			return nil
		}

		// Check custom excludes
		if matchesCustomExclude(rel, d.Name(), customExcludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check player data exclusion
		if !includePlayerData && isPlayerData(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check world exclusion
		if !includeWorld && isWorldDirectory(srcDir, rel, levelName) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check plugins exclusion
		if !includePlugins && isPlugin(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check mods exclusion
		if !includeMods && isMod(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check configs exclusion
		if !includeConfig && isConfig(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check logs exclusion
		if !includeLogs && isLog(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		targetPath := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}

		return dstFile.Close()
	})
}

func getFirstPathSegment(rel string) string {
	slash := filepath.ToSlash(rel)
	if idx := strings.Index(slash, "/"); idx >= 0 {
		return slash[:idx]
	}
	return slash
}

func isWorldDirectory(srcDir, rel, levelName string) bool {
	first := getFirstPathSegment(rel)
	lowerFirst := strings.ToLower(first)

	if strings.EqualFold(first, levelName) ||
		strings.EqualFold(first, levelName+"_nether") ||
		strings.EqualFold(first, levelName+"_the_end") ||
		strings.HasPrefix(lowerFirst, "world") ||
		first == "DIM-1" || first == "DIM1" {
		return true
	}

	// Check if the top-level directory contains a level.dat
	if _, err := os.Stat(filepath.Join(srcDir, first, "level.dat")); err == nil {
		return true
	}

	// Standalone level.dat or region files in root
	if first == rel {
		if first == "level.dat" || first == "level.dat_old" || first == "uid.dat" {
			return true
		}
	}
	return false
}

func isPlayerData(rel string) bool {
	slash := "/" + filepath.ToSlash(rel) + "/"
	if strings.Contains(slash, "/playerdata/") ||
		strings.Contains(slash, "/stats/") ||
		strings.Contains(slash, "/advancements/") {
		return true
	}

	base := filepath.Base(rel)
	switch base {
	case "usercache.json", "ops.json", "whitelist.json", "banned-players.json", "banned-ips.json":
		return true
	}
	return false
}

func isPlugin(rel string) bool {
	return getFirstPathSegment(rel) == "plugins"
}

func isMod(rel string) bool {
	return getFirstPathSegment(rel) == "mods"
}

func isConfig(rel string, isDir bool) bool {
	first := getFirstPathSegment(rel)
	if first == "config" || first == "defaultconfigs" {
		return true
	}
	// Root configuration files
	if first == rel && !isDir {
		if rel == "server.properties" {
			return true
		}
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".properties", ".yml", ".yaml", ".toml":
			return true
		case ".json":
			// player data json files are classified separately
			if !isPlayerData(rel) {
				return true
			}
		}
	}
	return false
}

func isLog(rel string, isDir bool) bool {
	first := getFirstPathSegment(rel)
	if first == "logs" || first == "crash-reports" {
		return true
	}
	lower := strings.ToLower(rel)
	return strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".log.gz")
}

func matchesCustomExclude(rel, name string, customExcludes []string) bool {
	if len(customExcludes) == 0 {
		return false
	}
	slashRel := filepath.ToSlash(rel)
	for _, raw := range customExcludes {
		pat := filepath.ToSlash(strings.TrimSpace(raw))
		if pat == "" {
			continue
		}
		if matched, _ := filepath.Match(pat, slashRel); matched {
			return true
		}
		if matched, _ := filepath.Match(pat, name); matched {
			return true
		}
		trimmed := strings.TrimSuffix(pat, "/*")
		trimmed = strings.TrimSuffix(trimmed, "/")
		if slashRel == trimmed || strings.HasPrefix(slashRel, trimmed+"/") {
			return true
		}
	}
	return false
}

