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
		image = "eclipse-temurin:21-jre-alpine"
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
	JavaVersion   int
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
	img := m.image
	if opts.JavaVersion > 0 {
		img = fmt.Sprintf("eclipse-temurin:%d-jre-alpine", opts.JavaVersion)
	}

	// The runtime image is a hard prerequisite: pulling it here keeps server
	// creation self-sufficient instead of failing with "No such image".
	if err := m.EnsureNamedImage(ctx, img); err != nil {
		return "", fmt.Errorf("ensure image %s: %w", img, err)
	}

	name := Name(opts.ID)
	cPort, _ := primaryContainerPort(opts.ServerType)

	entryScript := `FIFO="/tmp/console.in"
rm -f "$FIFO"
mkfifo -m 666 "$FIFO"
exec 3<> "$FIFO"

term_handler() {
  test -p "$FIFO" && printf '%s\n' "stop" "end" "shutdown" > "$FIFO" || true
  wait "$SERVER_PID" 2>/dev/null || true
  exit 0
}
trap term_handler TERM INT

if [ -f "/data/run.sh" ]; then
  sh /data/run.sh nogui < "$FIFO" &
elif [ -f "/data/server.jar" ]; then
  java -Xms512M -Xmx${RAM_MB:-2048}M ${JVM_OPTS} -jar /data/server.jar nogui < "$FIFO" &
else
  echo "No server.jar or run.sh found in /data"
  exit 1
fi
SERVER_PID=$!
wait "$SERVER_PID"
EXIT_CODE=$?
exit $EXIT_CODE
`

	cfg := &container.Config{
		Image:        img,
		WorkingDir:   containerData,
		Entrypoint:   []string{"sh", "-c", entryScript},
		Env: []string{
			fmt.Sprintf("RAM_MB=%d", opts.RAMMB),
			fmt.Sprintf("SERVER_PORT=%d", cPort),
			"MCM_DATA_DIR=" + containerData,
		},
		ExposedPorts: exposedPortsFor(opts.ServerType, opts.ExtraPorts),
		OpenStdin:    true,
		Tty:          false,
	}
	hostCfg := &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:%s", opts.DataDir, containerData)},
		RestartPolicy: container.RestartPolicy{
			// Server containers are created with no restart policy so that the
			// panel (and the user) retain explicit control over their lifecycle.
			Name: container.RestartPolicyDisabled,
		},
		Resources:    containerResources(opts),
		PortBindings: portBindings(opts),
	}

	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		// If the image vanished between the presence check and the create (or a
		// concurrent pull is still converging), pull again and retry once.
		if strings.Contains(err.Error(), errNoSuchImage) {
			if perr := m.pullNamedImage(ctx, img); perr != nil {
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

// EnsureImage makes sure the default runtime image is present locally, pulling it
// from the registry when it is not.
func (m *Manager) EnsureImage(ctx context.Context) error {
	return m.EnsureNamedImage(ctx, m.image)
}

// EnsureNamedImage makes sure the specified runtime image is present locally,
// pulling it from the registry when it is not.
func (m *Manager) EnsureNamedImage(ctx context.Context, imageName string) error {
	has, err := m.namedImagePresent(ctx, imageName)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	return m.pullNamedImage(ctx, imageName)
}

// namedImagePresent reports whether the specified image is available locally.
func (m *Manager) namedImagePresent(ctx context.Context, imageName string) (bool, error) {
	f := filters.NewArgs()
	f.Add("reference", imageName)
	imgs, err := m.client.ImageList(ctx, image.ListOptions{Filters: f})
	if err != nil {
		return false, fmt.Errorf("list images: %w", err)
	}
	return len(imgs) > 0, nil
}

// pullNamedImage pulls the specified image, reading (and discarding) the pull progress
// stream so the request does not block on an unconsumed response body.
func (m *Manager) pullNamedImage(ctx context.Context, imageName string) error {
	rc, err := m.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		if m.namedImagePresentCheck(ctx, imageName) {
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

// namedImagePresentCheck is a best-effort re-check used to swallow benign pull
// errors (e.g. "pull access denied") when the image already exists locally.
func (m *Manager) namedImagePresentCheck(ctx context.Context, imageName string) bool {
	has, err := m.namedImagePresent(ctx, imageName)
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
// defaults to RAM + 2GB unless an explicit memory_limit_mb is set. A positive
// cpu_limit (cores) is translated to NanoCPUs.
func containerResources(opts CreateOpts) container.Resources {
	res := container.Resources{
		Memory: int64((opts.RAMMB + 2048) * 1024 * 1024),
	}
	if opts.MemoryLimitMB > 0 {
		res.Memory = int64(opts.MemoryLimitMB) * 1024 * 1024
	}
	if opts.CPULimit > 0 {
		res.NanoCPUs = int64(opts.CPULimit * 1e9)
	}
	return res
}

// EnsureRestartPolicyDisabled updates the container's restart policy in Docker Engine
// to disabled ("no") so that when the server exits (from /stop or console), Docker Engine
// will not automatically restart it.
func (m *Manager) EnsureRestartPolicyDisabled(ctx context.Context, containerID string) error {
	_, err := m.client.ContainerUpdate(ctx, containerID, container.UpdateConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		},
	})
	if err != nil {
		return fmt.Errorf("update container restart policy: %w", err)
	}
	return nil
}

// Start starts a stopped or created container.
func (m *Manager) Start(ctx context.Context, containerID string) error {
	_ = m.EnsureRestartPolicyDisabled(ctx, containerID)
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

// Kill immediately stops a container without a graceful shutdown period. It
// sends SIGKILL right away, unlike Stop which first triggers SIGTERM. This is
// intended for unresponsive servers that will not exit cleanly.
func (m *Manager) Kill(ctx context.Context, containerID string) error {
	zero := 0
	if err := m.client.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &zero}); err != nil {
		return fmt.Errorf("kill container: %w", err)
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

// ContainerState captures runtime status and start time for a container.
type ContainerState struct {
	Status    string
	StartedAt string
	ExitCode  int
	OOMKilled bool
}

// Status returns the docker-reported state of a container.
func (m *Manager) Status(ctx context.Context, containerID string) (string, error) {
	insp, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspect container: %w", err)
	}
	return insp.State.Status, nil
}

// Inspect returns the container status and start timestamp if running.
func (m *Manager) Inspect(ctx context.Context, containerID string) (ContainerState, error) {
	insp, err := m.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return ContainerState{}, fmt.Errorf("inspect container: %w", err)
	}
	if insp.HostConfig != nil && insp.HostConfig.RestartPolicy.Name != "" && insp.HostConfig.RestartPolicy.Name != container.RestartPolicyDisabled {
		_ = m.EnsureRestartPolicyDisabled(ctx, containerID)
	}
	started := ""
	status := ""
	exitCode := 0
	oomKilled := false
	if insp.State != nil {
		status = insp.State.Status
		if insp.State.Running {
			started = insp.State.StartedAt
		}
		exitCode = insp.State.ExitCode
		oomKilled = insp.State.OOMKilled
	}
	return ContainerState{
		Status:    status,
		StartedAt: started,
		ExitCode:  exitCode,
		OOMKilled: oomKilled,
	}, nil
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
// RCON. It first tries writing directly to the named console pipe (/tmp/console.in)
// created in lightweight containers. If that is unavailable or fails, it falls back
// to exec-ing the itzg image's stdin-helper (mc-send-to-console / rcon-cli).
func (m *Manager) SendConsole(ctx context.Context, containerID, command string) error {
	// 1. Try named pipe inside the lightweight container
	err := m.execPipeConsole(ctx, containerID, command)
	if err == nil {
		return nil
	}

	// 2. Fall back to itzg image helpers if pipe is not present
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
	return fmt.Errorf("no console pipe or helper found in container")
}

func (m *Manager) execPipeConsole(ctx context.Context, containerID, command string) error {
	execID, err := m.client.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", `test -p /tmp/console.in && printf '%s\n' "$1" > /tmp/console.in`, "--", command},
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("create pipe console exec: %w", err)
	}
	hij, err := m.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return fmt.Errorf("attach pipe console exec: %w", err)
	}
	var stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(io.Discard, &stderr, hij.Reader)
	hij.Close()
	insp, err := m.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return fmt.Errorf("inspect pipe console exec: %w", err)
	}
	if insp.ExitCode != 0 {
		return fmt.Errorf("pipe console failed (code %d): %s", insp.ExitCode, strings.TrimSpace(stderr.String()))
	}
	return nil
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

// primaryContainerPort returns the primary container port and protocol for a server type.
func primaryContainerPort(serverType string) (int, string) {
	switch strings.ToLower(serverType) {
	case "geysermc":
		return 19132, "udp"
	case "waterfall", "bungeecord":
		return 25577, "tcp"
	default:
		return mcPort, mcProto
	}
}

// exposedPorts returns the set of container ports to mark exposed. It always
// includes the primary game port plus each extra port with its protocol.
func exposedPorts(extras []ExtraPort) nat.PortSet {
	return exposedPortsFor("", extras)
}

// exposedPortsFor returns the set of container ports to mark exposed for a given server type.
func exposedPortsFor(serverType string, extras []ExtraPort) nat.PortSet {
	cPort, cProto := primaryContainerPort(serverType)
	ports := nat.PortSet{
		nat.Port(fmt.Sprintf("%d/%s", cPort, cProto)): struct{}{},
	}
	for _, e := range extras {
		ports[nat.Port(fmt.Sprintf("%d/%s", e.ContainerPort, normalizeProto(e.Protocol)))] = struct{}{}
	}
	return ports
}

// portBindings builds the host-to-container port bindings. The primary game
// port binds srv HostPort -> container primary port. Each extra port binds its
// host port to its container port/protocol. All bind on 0.0.0.0.
func portBindings(opts CreateOpts) nat.PortMap {
	cPort, cProto := primaryContainerPort(opts.ServerType)
	bindings := nat.PortMap{
		nat.Port(fmt.Sprintf("%d/%s", cPort, cProto)): []nat.PortBinding{
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
