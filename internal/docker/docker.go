// Package docker manages the lifecycle of per-server containers via the Docker
// Engine API, honoring DOCKER_HOST.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	mcPort        = 25565
	mcProto       = "tcp"
	containerData = "/data"
	// errNoSuchImage is the substring Docker returns when a create references an
	// image that is not present locally. It is treated as a retry-after-pull
	// condition rather than a hard failure.
	errNoSuchImage = "No such image"
)

// Manager wraps a Docker client and owns the container lifecycle operations for
// MCM servers.
type Manager struct {
	client *client.Client
	host   string
	image  string
}

// New builds a Manager from a Docker host string (e.g. "unix:///...") and the
// runtime image used to launch server containers.
func New(host, image string) (*Manager, error) {
	if image == "" {
		image = "itzg/minecraft-server"
	}
	cli, err := client.NewClientWithOpts(client.WithHost(host))
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	cli.NegotiateAPIVersion(context.Background())
	return &Manager{client: cli, host: host, image: image}, nil
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

// RuntimeStatus describes the health of the Docker host relevant to MCM: whether
// the daemon is reachable and whether the runtime image needed to launch server
// containers is present locally.
type RuntimeStatus struct {
	Reachable  bool   `json:"reachable"`
	Image      string `json:"image"`
	ImageReady bool   `json:"image_ready"`
	Error      string `json:"error,omitempty"`
}

// RuntimeStatus checks the Docker daemon and the presence of the runtime image
// used to launch server containers. A reachable daemon with a missing image is
// a common reason a server fails to start, so both are surfaced here for
// diagnostics.
func (m *Manager) RuntimeStatus(ctx context.Context) RuntimeStatus {
	st := RuntimeStatus{Image: m.image}
	if _, err := m.client.Ping(ctx); err != nil {
		st.Error = fmt.Sprintf("docker daemon unreachable: %v", err)
		return st
	}
	st.Reachable = true

	f := filters.NewArgs()
	f.Add("reference", m.image)
	imgs, err := m.client.ImageList(ctx, image.ListOptions{Filters: f})
	if err != nil {
		st.Error = fmt.Sprintf("list images: %v", err)
		return st
	}
	st.ImageReady = len(imgs) > 0
	if !st.ImageReady {
		st.Error = fmt.Sprintf("runtime image %q is not present; run 'docker pull %s'", m.image, m.image)
	}
	return st
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
	ExtraPorts    []ExtraPort
	DataDir       string
	ServerType    string
	Version       string
	Build         string
	RAMMB         int
	CPULimit      float64
	MemoryLimitMB int
}

// ExtraPort describes an additional port to publish beyond the primary game
// port, e.g. a WebUI (tcp) or a Bedrock/Geyser adapter (udp).
type ExtraPort struct {
	ID            string
	Description   string
	HostPort      int
	ContainerPort int
	Protocol      string // tcp or udp
}

// Name returns the container name for a server ID.
func Name(id string) string {
	return "mcm-" + id
}

// Create provisions a stopped container for a server.
func (m *Manager) Create(ctx context.Context, opts CreateOpts) (string, error) {
	// The runtime image is a hard prerequisite: pulling it here keeps server
	// creation self-sufficient instead of failing with "No such image".
	if err := m.EnsureImage(ctx); err != nil {
		return "", fmt.Errorf("ensure image %s: %w", m.image, err)
	}

	name := Name(opts.ID)
	cfg := &container.Config{
		Image:        m.image,
		Env:          itzgEnv(opts),
		ExposedPorts: exposedPorts(opts.ExtraPorts),
	}
	hostCfg := &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:%s", opts.DataDir, containerData)},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources:    containerResources(opts),
		PortBindings: portBindings(opts),
	}

	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		// If the image vanished between the presence check and the create (or a
		// concurrent pull is still converging), pull again and retry once.
		if strings.Contains(err.Error(), errNoSuchImage) {
			if perr := m.pullImage(ctx); perr != nil {
				return "", fmt.Errorf("pull image after create failure: %w", perr)
			}
			resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
			if err != nil {
				return "", fmt.Errorf("create container: %w", err)
			}
			return resp.ID, nil
		}
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// EnsureImage makes sure the runtime image is present locally, pulling it from
// the registry when it is not. It is idempotent: when the image is already
// present, it returns without contacting the registry.
func (m *Manager) EnsureImage(ctx context.Context) error {
	has, err := m.imagePresent(ctx)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	return m.pullImage(ctx)
}

// imagePresent reports whether the runtime image is available locally.
func (m *Manager) imagePresent(ctx context.Context) (bool, error) {
	f := filters.NewArgs()
	f.Add("reference", m.image)
	imgs, err := m.client.ImageList(ctx, image.ListOptions{Filters: f})
	if err != nil {
		return false, fmt.Errorf("list images: %w", err)
	}
	return len(imgs) > 0, nil
}

// pullImage pulls the runtime image, reading (and discarding) the pull progress
// stream so the request does not block on an unconsumed response body.
func (m *Manager) pullImage(ctx context.Context) error {
	rc, err := m.client.ImagePull(ctx, m.image, image.PullOptions{})
	if err != nil {
		// If the image already exists locally, the pull error is benign (e.g. a
		// not-yet-tagged or concurrently-pulled image). Re-check presence.
		if m.imagePresentCheck(ctx) {
			return nil
		}
		return fmt.Errorf("pull image: %w", err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		if !errors.Is(err, context.Canceled) {
			return fmt.Errorf("read pull output: %w", err)
		}
	}
	return nil
}

// imagePresentCheck is a best-effort re-check used to swallow benign pull
// errors (e.g. "pull access denied") when the image already exists locally.
func (m *Manager) imagePresentCheck(ctx context.Context) bool {
	has, err := m.imagePresent(ctx)
	return err == nil && has
}

// itzgEnv maps MCM's create options onto the environment variables expected by
// the itzg/minecraft-server image. The image resolves and downloads the server
// jar itself, so MCM only needs to pass the platform, version, memory, EULA,
// and any platform-specific build/loader selector.
func itzgEnv(opts CreateOpts) []string {
	env := []string{
		"MCM_DATA_DIR=" + containerData,
		"TYPE=" + strings.ToUpper(opts.ServerType),
		"VERSION=" + opts.Version,
		"MEMORY=" + fmt.Sprintf("%dM", opts.RAMMB),
		"EULA=TRUE",
		// Let itzg create the named console input pipe so console commands can
		// be sent to the server stdin without requiring RCON.
		"CREATE_CONSOLE_IN_PIPE=true",
	}
	switch strings.ToLower(opts.ServerType) {
	case "paper":
		env = append(env, "BUILD_NUMBER="+opts.Build)
	case "fabric":
		env = append(env, "FABRIC_LOADER="+opts.Build)
	case "forge":
		env = append(env, "FORGE_VERSION="+opts.Build)
	case "neoforge":
		env = append(env, "NEOFORGE_VERSION="+opts.Build)
	}
	return env
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

// Exists reports whether a container with the given id is present on the
// daemon. A missing container (e.g. one deleted manually outside MCM) is not an
// error; it returns (false, nil) so callers can recreate it.
func (m *Manager) Exists(ctx context.Context, containerID string) (bool, error) {
	_, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect container: %w", err)
	}
	return true, nil
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

// SendConsole writes a command to a running server's console without requiring
// RCON. It execs the itzg image's stdin-helper (mc-send-to-console, falling
// back to the older rcon-cli name) inside the container, which pipes the line
// into the running Java server's stdin. Output is drained and discarded so a
// chatty response cannot fill the exec stream and block.
func (m *Manager) SendConsole(ctx context.Context, containerID, command string) error {
	for _, helper := range []string{"mc-send-to-console", "rcon-cli"} {
		err := m.execConsole(ctx, containerID, helper, command)
		if err == nil {
			return nil
		}
		// Only fall back when the first helper binary is missing; other
		// failures (container gone, non-zero exit) are real and should surface.
		if !errors.Is(err, errConsoleBinaryMissing) {
			return err
		}
	}
	return fmt.Errorf("no console helper found in runtime image")
}

// errConsoleBinaryMissing marks a console exec that failed because the helper
// binary is not present in the image (exec exits with 127, the shell's
// "command not found" status).
var errConsoleBinaryMissing = errors.New("console helper binary not found")

// ErrConsolePipeDisabled marks a console exec that failed because the server
// container was not started with itzg's CREATE_CONSOLE_IN_PIPE, so the named
// console input pipe does not exist. The container must be recreated (restarted
// with the env present) before console input can work.
var ErrConsolePipeDisabled = errors.New("console input pipe is not enabled on the server container")

func (m *Manager) execConsole(ctx context.Context, containerID, helper, command string) error {
	execID, err := m.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          []string{helper, command},
		User:         m.execUserForContainer(ctx, containerID),
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("create console exec: %w", err)
	}
	hij, err := m.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("attach console exec: %w", err)
	}
	var stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(io.Discard, &stderr, hij.Reader)
	hij.Close()
	insp, err := m.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("inspect console exec: %w", err)
	}
	if insp.ExitCode == 127 {
		return errConsoleBinaryMissing
	}
	if insp.ExitCode != 0 {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "CREATE_CONSOLE_IN_PIPE") || strings.Contains(msg, "Named pipe") {
			return ErrConsolePipeDisabled
		}
		if msg != "" {
			return fmt.Errorf("console command failed (code %d): %s", insp.ExitCode, msg)
		}
		return fmt.Errorf("console command failed (code %d)", insp.ExitCode)
	}
	return nil
}

// execUserForContainer returns the `uid:gid` the server process runs as, so the
// console-send helper can be exec'd as that same user. The itzg image names the
// runtime user via its UID/GID environment variables; it falls back to 1000:1000
// (the image default) when they are absent or the container cannot be inspected.
func (m *Manager) execUserForContainer(ctx context.Context, containerID string) string {
	uid, gid := "1000", "1000"
	insp, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return uid + ":" + gid
	}
	for _, e := range insp.Config.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		switch k {
		case "UID":
			if v != "" {
				uid = v
			}
		case "GID":
			if v != "" {
				gid = v
			}
		}
	}
	return uid + ":" + gid
}

// exposedPorts returns the set of container ports to mark exposed. It always
// includes the primary game port plus each extra port with its protocol.
func exposedPorts(extras []ExtraPort) nat.PortSet {
	ports := nat.PortSet{
		nat.Port(fmt.Sprintf("%d/%s", mcPort, mcProto)): struct{}{},
	}
	for _, e := range extras {
		ports[nat.Port(fmt.Sprintf("%d/%s", e.ContainerPort, normalizeProto(e.Protocol)))] = struct{}{}
	}
	return ports
}

// portBindings builds the host-to-container port bindings. The primary game
// port binds srv HostPort -> container 25565/tcp. Each extra port binds its
// host port to its container port/protocol. All bind on 0.0.0.0.
func portBindings(opts CreateOpts) nat.PortMap {
	bindings := nat.PortMap{
		nat.Port(fmt.Sprintf("%d/%s", mcPort, mcProto)): []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: strconv.Itoa(opts.HostPort)},
		},
	}
	for _, e := range opts.ExtraPorts {
		key := nat.Port(fmt.Sprintf("%d/%s", e.ContainerPort, normalizeProto(e.Protocol)))
		bindings[key] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: strconv.Itoa(e.HostPort)},
		}
	}
	return bindings
}

func normalizeProto(proto string) string {
	if proto != "udp" {
		return "tcp"
	}
	return "udp"
}
