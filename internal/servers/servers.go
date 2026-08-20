// Package servers implements server record CRUD and container orchestration.
package servers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/ports"
)

// dockerRuntime is the subset of the Docker manager used by the server store.
// It is an interface so tests can substitute a fake runtime.
type dockerRuntime interface {
	Ping(ctx context.Context) error
	RuntimeStatus(ctx context.Context) docker.RuntimeStatus
	Remove(ctx context.Context, containerID string) error
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	Status(ctx context.Context, containerID string) (string, error)
	Logs(ctx context.Context, containerID string, follow bool) (io.ReadCloser, error)
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
	ExtraPorts    []ExtraPort `json:"extra_ports"`
	ContainerID   string      `json:"container_id,omitempty"`
	State         string      `json:"state"`
	// Backup settings. BackupEnabled defaults to true; BackupIntervalMinutes
	// is the minutes between automatic backups (default 720).
	BackupEnabled         bool   `json:"backup_enabled"`
	BackupIntervalMinutes int    `json:"backup_interval_minutes"`
	CreatedAt             string `json:"created_at"`
	UpdatedAt             string `json:"updated_at"`
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
	CPULimit      float64      `json:"cpu_limit"`
	MemoryLimitMB int          `json:"memory_limit_mb"`
	ExtraPorts    []ExtraPort  `json:"extra_ports"`
}

// UpdateInput is the payload for updating a server.
type UpdateInput struct {
	Name                  *string       `json:"name"`
	ServerType            *jars.JarType `json:"server_type"`
	Version               *string       `json:"version"`
	Build                 *string       `json:"build"`
	RAMMB                 *int          `json:"ram_mb"`
	CPULimit              *float64      `json:"cpu_limit"`
	MemoryLimitMB         *int          `json:"memory_limit_mb"`
	BackupEnabled         *bool         `json:"backup_enabled"`
	BackupIntervalMinutes *int          `json:"backup_interval_minutes"`
	ExtraPorts            *[]ExtraPort  `json:"extra_ports"`
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
	// dns optionally publishes/removes SRV records as servers start and stop.
	dns dns.Publisher
}

// NewStore wires the server store together.
func NewStore(handle *db.Store, dm *docker.Manager, jr *jars.Resolver, start, end int, dataDir string) *Store {
	return &Store{
		db:      handle.DB,
		docker:  dm,
		jars:    jr,
		ports:   ports.NewPool(handle.DB, start, end),
		dataDir: dataDir,
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

// List returns all servers ordered by creation time.
func (s *Store) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, server_type, version, COALESCE(build,''), ram_mb, cpu_limit, memory_limit_mb, host_port, COALESCE(extra_ports,'[]'), COALESCE(container_id,''), state, backup_enabled, backup_interval_minutes, created_at, updated_at FROM servers ORDER BY created_at`)
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
		if err := rows.Scan(&srv.ID, &srv.Name, &srv.ServerType, &srv.Version, &srv.Build, &srv.RAMMB, &srv.CPULimit, &srv.MemoryLimitMB, &srv.HostPort, &extra, &srv.ContainerID, &srv.State, &srv.BackupEnabled, &srv.BackupIntervalMinutes, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
			return nil, err
		}
		srv.ExtraPorts = decodeExtraPorts(extra)
		out = append(out, srv)
	}
	return out, rows.Err()
}

// Get returns a single server by id.
func (s *Store) Get(ctx context.Context, id string) (Server, error) {
	var srv Server
	var extra string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, server_type, version, COALESCE(build,''), ram_mb, cpu_limit, memory_limit_mb, host_port, COALESCE(extra_ports,'[]'), COALESCE(container_id,''), state, backup_enabled, backup_interval_minutes, created_at, updated_at FROM servers WHERE id = ?`, id).
		Scan(&srv.ID, &srv.Name, &srv.ServerType, &srv.Version, &srv.Build, &srv.RAMMB, &srv.CPULimit, &srv.MemoryLimitMB, &srv.HostPort, &extra, &srv.ContainerID, &srv.State, &srv.BackupEnabled, &srv.BackupIntervalMinutes, &srv.CreatedAt, &srv.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Server{}, ErrNotFound
	}
	if err != nil {
		return Server{}, err
	}
	srv.ExtraPorts = decodeExtraPorts(extra)
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
	port, err := s.ports.Allocate(ctx)
	if err != nil {
		return Server{}, fmt.Errorf("allocate port: %w", err)
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?)`,
		id, in.Name, string(in.ServerType), resolved.Version, resolved.Build, in.RAMMB, in.CPULimit, in.MemoryLimitMB, port, encodeExtraPorts(in.ExtraPorts), StateStopped, now, now)
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

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`UPDATE servers SET name=?, server_type=?, version=?, build=?, ram_mb=?, cpu_limit=?, memory_limit_mb=?, backup_enabled=?, backup_interval_minutes=?, extra_ports=?, updated_at=? WHERE id=?`,
		srv.Name, srv.ServerType, srv.Version, srv.Build, srv.RAMMB, srv.CPULimit, srv.MemoryLimitMB, srv.BackupEnabled, srv.BackupIntervalMinutes, encodeExtraPorts(srv.ExtraPorts), now, id)
	if err != nil {
		return Server{}, fmt.Errorf("update server: %w", err)
	}
	return s.Get(ctx, id)
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
	if err := s.setState(ctx, id, StateRunning); err != nil {
		return Server{}, err
	}
	if s.dns != nil {
		_ = s.dns.Upsert(ctx, id, "", srv.HostPort)
	}
	return s.Get(ctx, id)
}

// Stop stops a running container if one exists.
func (s *Store) Stop(ctx context.Context, id string) (Server, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return Server{}, err
	}
	if err := s.setState(ctx, id, StateStopping); err != nil {
		return Server{}, err
	}
	if srv.ContainerID != "" {
		if err := s.docker.Stop(ctx, srv.ContainerID, 30*time.Second); err != nil {
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
	_, err = s.db.ExecContext(ctx, `UPDATE servers SET container_id='', state=?, updated_at=? WHERE id=?`,
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
	status, err := s.docker.Status(ctx, srv.ContainerID)
	if err != nil {
		return srv, nil
	}
	mapped := mapDockerState(status)
	if mapped != srv.State {
		_ = s.setState(ctx, id, mapped)
		srv.State = mapped
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
		return srv, nil
	}
	cid, err := s.docker.Create(ctx, docker.CreateOpts{
		ID:            srv.ID,
		HostPort:      srv.HostPort,
		ExtraPorts:    toDockerExtras(srv.ExtraPorts),
		DataDir:       s.dataPath(srv.ID),
		ServerType:    srv.ServerType,
		Version:       srv.Version,
		Build:         srv.Build,
		RAMMB:         srv.RAMMB,
		CPULimit:      srv.CPULimit,
		MemoryLimitMB: srv.MemoryLimitMB,
	})
	if err != nil {
		return Server{}, err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE servers SET container_id=?, updated_at=? WHERE id=?`, cid, time.Now().UTC().Format(time.RFC3339), srv.ID)
	if err != nil {
		return Server{}, err
	}
	srv.ContainerID = cid
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

func (s *Store) setState(ctx context.Context, id, state string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET state=?, updated_at=? WHERE id=?`, state, time.Now().UTC().Format(time.RFC3339), id)
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

func mapDockerState(state string) string {
	switch state {
	case "running":
		return StateRunning
	case "stopped", "exited", "created", "dead":
		return StateStopped
	case "restarting":
		return StateStarting
	case "paused":
		return StateRunning
	default:
		return StateError
	}
}
