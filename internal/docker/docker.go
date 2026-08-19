// Package docker manages the lifecycle of per-server containers via the Docker
// Engine API, honoring DOCKER_HOST.
package docker

import (
	"context"
	"fmt"
	"io"
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
}

// New builds a Manager from a Docker host string (e.g. "unix:///...").
func New(host string) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host))
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	cli.NegotiateAPIVersion(context.Background())
	return &Manager{client: cli}, nil
}

// CreateOpts describes a server container to create.
type CreateOpts struct {
	ID         string
	HostPort   int
	DataDir    string
	ServerType string
	Version    string
	Build      string
	RAMMB      int
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
		PortBindings: portBindings(opts.HostPort),
		Binds:        []string{fmt.Sprintf("%s:%s", opts.DataDir, containerData)},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources: container.Resources{
			Memory: int64((opts.RAMMB + 512) * 1024 * 1024),
		},
	}

	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
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

func portBindings(hostPort int) nat.PortMap {
	return nat.PortMap{
		nat.Port(fmt.Sprintf("%d/%s", mcPort, mcProto)): []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", hostPort)},
		},
	}
}
