// Package docker manages the lifecycle of per-server containers via the Docker
// Engine API, honoring DOCKER_HOST.
package docker

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	mcPort        = 25565
	mcProto       = "tcp"
	imageName     = "mcm-server:latest"
	containerData = "/data"
)

// Manager wraps a Docker client and owns the container lifecycle operations for
// MCM servers.
type Manager struct {
	client *client.Client
	host   string
}

// New builds a Manager from a Docker host string (e.g. "unix:///...").
func New(host string) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host))
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	cli.NegotiateAPIVersion(context.Background())
	return &Manager{client: cli, host: host}, nil
}

// Ping verifies the Docker daemon is reachable and responds. It is used by the
// readiness probe.
func (m *Manager) Ping(ctx context.Context) error {
	_, err := m.client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}
	return nil
}

// HostAddress returns the network address on which the Docker daemon is
// reachable, suitable for reaching a container's published host port. It maps
// unix/local sockets to localhost and strips any transport scheme or path.
func (m *Manager) HostAddress() string {
	h := strings.TrimPrefix(m.host, "unix://")
	if idx := strings.Index(h, "://"); idx >= 0 {
		h = h[idx+3:]
	}
	if idx := strings.Index(h, "/"); idx >= 0 {
		h = h[:idx]
	}
	if h == "" {
		return "127.0.0.1"
	}
	if host, _, err := net.SplitHostPort(h); err == nil && host != "" {
		return host
	}
	return h
}

// CreateOpts describes a server container to create.
type CreateOpts struct {
	ID            string
	HostPort      int
	DataDir       string
	ServerType    string
	Version       string
	Build         string
	RAMMB         int
	CPULimit      float64
	MemoryLimitMB int
}

// Name returns the container name for a server ID.
func Name(id string) string {
	return "mcm-" + id
}

// Create provisions a stopped container for a server.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (string, error) {
	name := Name(opts.ID)
	cfg := &container.Config{
		Image: imageName,
		Env: []string{
			"SERVER_TYPE=" + opts.ServerType,
			"VERSION=" + opts.Version,
			"BUILD=" + opts.Build,
			"RAM_MB=" + fmt.Sprintf("%d", opts.RAMMB),
		},
		ExposedPorts: containerExposedPorts(),
	}
	hostCfg := &container.HostConfig{
		// The game port is exposed but NOT bound to the host. The gateway owns
		// the public port and relays to the container's internal address, so
		// nothing else may publish 25565 to the host.
		Binds: []string{fmt.Sprintf("%s:%s", opts.DataDir, containerData)},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources: containerResources(opts),
	}

	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// ContainerAddr resolves the address the gateway should dial to reach a
// server's game port. It prefers the container's internal IP on the exposed
// 25565 port (Docker inspect), falling back to the daemon host's published-port
// address for remote-daemon setups where the container IP is not reachable.
func (m *Manager) ContainerAddr(ctx context.Context, containerID string, hostPort int) (string, error) {
	insp, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container %s: %w", containerID, err)
	}
	if insp.NetworkSettings != nil {
		for _, network := range insp.NetworkSettings.Networks {
			if network.IPAddress != "" {
				return net.JoinHostPort(network.IPAddress, fmt.Sprintf("%d", mcPort)), nil
			}
		}
	}
	// Fall back to the daemon host's published port so cross-deployment setups
	// (e.g. a remote daemon where the container IP is unreachable) still work.
	return net.JoinHostPort(m.HostAddress(), fmt.Sprintf("%d", hostPort)), nil
}

// containerResources builds the container resource limits. The memory limit
// defaults to RAM + 512MB unless an explicit memory_limit_mb is set. A positive
// cpu_limit (cores) is translated to NanoCPUs.
func containerResources(opts CreateOpts) container.Resources {
	res := container.Resources{
		Memory: int64((opts.RAMMB + 512) * 1024 * 1024),
	}
	if opts.MemoryLimitMB > 0 {
		res.Memory = int64(opts.MemoryLimitMB) * 1024 * 1024
	}
	if opts.CPULimit > 0 {
		res.NanoCPUs = int64(opts.CPULimit * 1e9)
	}
	return res
}

// Start starts a stopped or created container.
func (m *Manager) Start(ctx context.Context, containerID string) error {
	if err := m.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	return nil
}

// Stop gracefully stops a container, falling back to a hard stop after timeout.
func (m *Manager) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	t := int(timeout.Seconds())
	if err := m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &t}); err != nil {
		return fmt.Errorf("stop container: %w", err)
	}
	return nil
}

// Remove deletes a container, killing it if necessary.
func (m *Manager) Remove(ctx context.Context, containerID string) error {
	if err := m.client.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}
	return nil
}

// Status returns the docker-reported state of a container.
func (m *Manager) Status(ctx context.Context, containerID string) (string, error) {
	insp, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	return insp.State.Status, nil
}

// Logs returns a stream of the container's combined stdout/stderr logs.
func (m *Manager) Logs(ctx context.Context, containerID string, follow bool) (io.ReadCloser, error) {
	rc, err := m.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: false,
	})
	if err != nil {
		return nil, fmt.Errorf("get container logs: %w", err)
	}
	return rc, nil
}

func containerExposedPorts() nat.PortSet {
	return nat.PortSet{
		nat.Port(fmt.Sprintf("%d/%s", mcPort, mcProto)): struct{}{},
	}
}
