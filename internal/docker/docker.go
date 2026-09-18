// Package docker manages the lifecycle of per-server containers via the Docker
// Engine API, honoring DOCKER_HOST.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	// NetworkName is the Docker bridge network used for inter-container communication.
	NetworkName = "mcm-network"

	mcPort        = 25565
	mcProto       = "tcp"
	containerData = "/data"
	// errNoSuchImage is the substring Docker returns when a create references an
	// image that is not present locally. It is treated as a retry-after-pull
	// condition rather than a hard failure.
	errNoSuchImage = "No such image"
)

// DefaultEntryScript is the container entrypoint script used to handle console FIFO,
// graceful shutdown traps, installer execution (e.g. for Forge/NeoForge), and server process lifecycle.
const DefaultEntryScript = `FIFO="/tmp/console.in"
rm -f "$FIFO"
mkfifo -m 666 "$FIFO"
exec 3<> "$FIFO"

term_handler() {
  test -p "$FIFO" && printf '%s\n' "stop" "end" "shutdown" "geyser stop" > "$FIFO" || true
  wait "$SERVER_PID" 2>/dev/null || true
  exit 0
}
trap term_handler TERM INT

if [ ! -f "/data/run.sh" ] && [ ! -f "/data/server.jar" ]; then
  INSTALLER=""
  if [ -f "/data/installer.jar" ]; then
    INSTALLER="/data/installer.jar"
  else
    for f in /data/*installer*.jar; do
      if [ -f "$f" ]; then
        INSTALLER="$f"
        break
      fi
    done
  fi

  if [ -n "$INSTALLER" ]; then
    echo "Running server installer ($INSTALLER)..."
    IS_QUILT=0
    case "$INSTALLER" in *quilt*) IS_QUILT=1;; esac
    if [ "$SERVER_TYPE" = "quilt" ] || [ $IS_QUILT -eq 1 ]; then
      QUILT_ARGS="install server"
      if [ -n "$SERVER_VERSION" ]; then
        QUILT_ARGS="$QUILT_ARGS $SERVER_VERSION"
        if [ -n "$SERVER_BUILD" ] && [ "$SERVER_BUILD" != "latest" ]; then
          QUILT_ARGS="$QUILT_ARGS $SERVER_BUILD"
        fi
      fi
      java -Djava.awt.headless=true -jar "$INSTALLER" $QUILT_ARGS --download-server --install-dir=/data
    else
      java -Djava.awt.headless=true -jar "$INSTALLER" --installServer
    fi
    INSTALL_EXIT=$?
    if [ $INSTALL_EXIT -ne 0 ]; then
      echo "Server installer failed with exit code $INSTALL_EXIT"
      exit $INSTALL_EXIT
    fi
    echo "Server installer completed successfully."

    # In older Forge (<= 1.16.5), the installer creates forge-*.jar instead of run.sh
    if [ ! -f "/data/run.sh" ] && [ ! -f "/data/server.jar" ]; then
      for f in /data/forge-*.jar /data/*forge*.jar; do
        case "$f" in
          *installer*) ;;
          *)
            if [ -f "$f" ]; then
              echo "Found Forge server jar $f, linking to /data/server.jar"
              ln -sf "$f" /data/server.jar
              break
            fi
            ;;
        esac
      done
    fi
    if [ -f "/data/quilt-server-launch.jar" ] && [ ! -f "/data/server.jar" ]; then
      ln -sf /data/quilt-server-launch.jar /data/server.jar
    fi
  fi
fi

SERVER_ARGS="--nogui nogui"
STYPE="${SERVER_TYPE:-}"
if [ -z "$STYPE" ]; then
  if [ -f "/data/velocity.toml" ]; then
    STYPE="velocity"
  elif [ -f "/data/waterfall.yml" ] || [ -f "/data/BungeeCord.jar" ]; then
    STYPE="waterfall"
  elif [ -f "/data/geysermc.jar" ] || [ -f "/data/Geyser.jar" ]; then
    STYPE="geysermc"
  fi
fi

case "$STYPE" in
  velocity)
    SERVER_ARGS="-p ${SERVER_PORT:-25577}"
    ;;
  waterfall|bungeecord|limbo|nanolimbo|geysermc)
    SERVER_ARGS=""
    ;;
  ketting)
    SERVER_ARGS="-minecraftVersion ${SERVER_VERSION:-1.20.1} -accepteula -noui nogui"
    ;;
esac

TARGET_JAR=""
if [ -f "/data/server.jar" ]; then
  TARGET_JAR="/data/server.jar"
elif [ -f "/data/quilt-server-launch.jar" ]; then
  TARGET_JAR="/data/quilt-server-launch.jar"
fi

if [ -f "/data/run.sh" ]; then
  if [ -f "/data/user_jvm_args.txt" ]; then
    if ! grep -q "^-Xmx" /data/user_jvm_args.txt 2>/dev/null; then
      printf '\n-Xms512M\n-Xmx%sM\n' "${RAM_MB:-2048}" >> /data/user_jvm_args.txt
    fi
    if ! grep -q "java.awt.headless" /data/user_jvm_args.txt 2>/dev/null; then
      printf '\n-Djava.awt.headless=true\n' >> /data/user_jvm_args.txt
    fi
  else
    printf -- '-Xms512M\n-Xmx%sM\n-Djava.awt.headless=true\n' "${RAM_MB:-2048}" > /data/user_jvm_args.txt
  fi
  chmod +x /data/run.sh 2>/dev/null || true
  sh /data/run.sh --nogui nogui < "$FIFO" &
elif [ -n "$TARGET_JAR" ]; then
  java -Djava.awt.headless=true -Xms512M -Xmx${RAM_MB:-2048}M ${JVM_OPTS} -jar "$TARGET_JAR" $SERVER_ARGS < "$FIFO" &
else
  echo "No server.jar or run.sh found in /data"
  exit 1
fi
SERVER_PID=$!
wait "$SERVER_PID"
EXIT_CODE=$?
exit $EXIT_CODE
`

type cpuSample struct {
	readAt      time.Time
	totalUsage  uint64
	systemUsage uint64
}

// ContainerStats represents resource consumption metrics for a container.
type ContainerStats struct {
	CPUPercent       float64 `json:"cpu_percent"`
	CPUCores         int     `json:"cpu_cores"`
	MemoryUsageBytes uint64  `json:"memory_usage_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
	MemoryPercent    float64 `json:"memory_percent"`
	DiskReadBytes    uint64  `json:"disk_read_bytes"`
	DiskWriteBytes   uint64  `json:"disk_write_bytes"`
	NetRxBytes       uint64  `json:"net_rx_bytes"`
	NetTxBytes       uint64  `json:"net_tx_bytes"`
}

// Manager wraps a Docker client and owns the container lifecycle operations for
// MCM servers.
type Manager struct {
	client   *client.Client
	host     string
	image    string
	cpuMu    sync.Mutex
	cpuCache map[string]cpuSample
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
	return &Manager{
		client:   cli,
		host:     host,
		image:    image,
		cpuCache: make(map[string]cpuSample),
	}, nil
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
	// If a container with this name already exists, reuse it instead of conflicting.
	if insp, err := m.client.ContainerInspect(ctx, name); err == nil {
		return insp.ID, nil
	}

	cPort, _ := primaryContainerPort(opts.ServerType)

	env := []string{
		fmt.Sprintf("RAM_MB=%d", opts.RAMMB),
		fmt.Sprintf("SERVER_PORT=%d", cPort),
		"MCM_DATA_DIR=" + containerData,
		"SERVER_TYPE=" + opts.ServerType,
		"SERVER_VERSION=" + opts.Version,
		"SERVER_BUILD=" + opts.Build,
	}
	if opts.ServerType == "ketting" {
		env = append(env,
			fmt.Sprintf("kettinglauncher_minecraftVersion=%s", opts.Version),
			"kettinglauncher_accepteula=true",
			"kettinglauncher_noui=true",
		)
	}

	cfg := &container.Config{
		Image:        img,
		WorkingDir:   containerData,
		Entrypoint:   []string{"sh", "-c", DefaultEntryScript},
		Env:          env,
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

	_ = m.EnsureNetwork(ctx)
	netCfg := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			NetworkName: {
				Aliases: []string{name},
			},
		},
	}

	resp, err := m.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
	if err != nil && (strings.Contains(err.Error(), "network") || strings.Contains(err.Error(), NetworkName)) {
		// Fallback to default networking if custom network creation/attachment fails
		resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	}
	if err != nil {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "conflict") || strings.Contains(errLower, "already in use") {
			if insp, errInsp := m.client.ContainerInspect(ctx, name); errInsp == nil {
				return insp.ID, nil
			}
			if insp, errInsp := m.client.ContainerInspect(ctx, "/"+name); errInsp == nil {
				return insp.ID, nil
			}
			_ = m.client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
			resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
			if err == nil {
				return resp.ID, nil
			}
		}
		// If the image vanished between the presence check and the create (or a
		// concurrent pull is still converging), pull again and retry once.
		if strings.Contains(err.Error(), errNoSuchImage) {
			if perr := m.pullNamedImage(ctx, img); perr != nil {
				return "", fmt.Errorf("pull image after create failure: %w", perr)
			}
			resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
			if err != nil {
				resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
			}
			if err != nil {
				errLower := strings.ToLower(err.Error())
				if strings.Contains(errLower, "conflict") || strings.Contains(errLower, "already in use") {
					if insp, errInsp := m.client.ContainerInspect(ctx, name); errInsp == nil {
						return insp.ID, nil
					}
					if insp, errInsp := m.client.ContainerInspect(ctx, "/"+name); errInsp == nil {
						return insp.ID, nil
					}
					_ = m.client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
					resp, err = m.client.ContainerCreate(ctx, cfg, hostCfg, netCfg, nil, name)
					if err == nil {
						return resp.ID, nil
					}
				}
				return "", fmt.Errorf("create container: %w", err)
			}
			return resp.ID, nil
		}
		return "", fmt.Errorf("create container: %w", err)
	}
	return resp.ID, nil
}

// EnsureNetwork verifies that the panel's bridge network (NetworkName) exists,
// creating it if missing so that containers can resolve each other by name (mcm-<id>).
func (m *Manager) EnsureNetwork(ctx context.Context) error {
	if m.client == nil {
		return nil
	}
	_, err := m.client.NetworkInspect(ctx, NetworkName, network.InspectOptions{})
	if err == nil {
		return nil
	}
	_, err = m.client.NetworkCreate(ctx, NetworkName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return fmt.Errorf("create network %s: %w", NetworkName, err)
	}
	return nil
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
	if m.client != nil {
		_ = m.EnsureNetwork(ctx)
		_ = m.client.NetworkConnect(ctx, NetworkName, containerID, nil)
	}
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

// ContainerPortUsage captures a port bound by a Docker container.
type ContainerPortUsage struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	State         string `json:"state"`
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol"`
}

// RunningContainerPorts returns all host ports bound by existing/running containers.
func (m *Manager) RunningContainerPorts(ctx context.Context) ([]ContainerPortUsage, error) {
	if m == nil || m.client == nil {
		return nil, nil
	}
	containers, err := m.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	var usages []ContainerPortUsage
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				usages = append(usages, ContainerPortUsage{
					ContainerID:   c.ID,
					ContainerName: name,
					State:         c.State,
					HostPort:      int(p.PublicPort),
					ContainerPort: int(p.PrivatePort),
					Protocol:      strings.ToLower(p.Type),
				})
			}
		}
	}
	return usages, nil
}

// IsHostPortBound tests whether a port is currently listening on the host system.
func IsHostPortBound(port int, protocol string) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	proto := strings.ToLower(protocol)
	if proto == "" || proto == "tcp" {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return true
		}
		_ = l.Close()
	}
	if proto == "udp" {
		l, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return true
		}
		_ = l.Close()
	}
	return false
}

// ParsePortConflictError inspects Docker start errors to extract conflicting port information.
func ParsePortConflictError(err error) (int, string, bool) {
	if err == nil {
		return 0, "", false
	}
	msg := err.Error()
	msgLower := strings.ToLower(msg)
	if !strings.Contains(msgLower, "port is already allocated") &&
		!strings.Contains(msgLower, "address already in use") &&
		!strings.Contains(msgLower, "bind: address already in use") &&
		!strings.Contains(msgLower, "failed programming external connectivity") {
		return 0, "", false
	}
	re := regexp.MustCompile(`:(\d{2,5})\s+failed|:(\d{2,5}):\s+bind|port\s+(\d{2,5})`)
	if m := re.FindStringSubmatch(msg); len(m) > 1 {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				if p, pErr := strconv.Atoi(m[i]); pErr == nil {
					return p, msg, true
				}
			}
		}
	}
	return 0, msg, true
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
		Tail:       "1000",
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
	case "velocity", "waterfall", "bungeecord":
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
// If HostPort <= 0, no host binding is created (e.g. for servers behind a proxy).
func portBindings(opts CreateOpts) nat.PortMap {
	cPort, cProto := primaryContainerPort(opts.ServerType)
	bindings := nat.PortMap{}
	if opts.HostPort > 0 {
		bindings[nat.Port(fmt.Sprintf("%d/%s", cPort, cProto))] = []nat.PortBinding{
			{HostIP: "0.0.0.0", HostPort: strconv.Itoa(opts.HostPort)},
		}
	}
	for _, e := range opts.ExtraPorts {
		if e.HostPort > 0 {
			key := nat.Port(fmt.Sprintf("%d/%s", e.ContainerPort, normalizeProto(e.Protocol)))
			bindings[key] = []nat.PortBinding{
				{HostIP: "0.0.0.0", HostPort: strconv.Itoa(e.HostPort)},
			}
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

// Stats returns resource consumption statistics (CPU, Memory, Disk I/O, Network) for a container.
func (m *Manager) Stats(ctx context.Context, containerID string) (ContainerStats, error) {
	resp, err := m.client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return ContainerStats{}, fmt.Errorf("container stats: %w", err)
	}
	defer resp.Body.Close()

	var raw container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ContainerStats{}, fmt.Errorf("decode container stats: %w", err)
	}

	return m.CalculateStats(containerID, &raw), nil
}

// CalculateStats computes normalized resource metrics from a Docker StatsResponse.
func (m *Manager) CalculateStats(containerID string, raw *container.StatsResponse) ContainerStats {
	if raw == nil {
		return ContainerStats{}
	}

	// 1. CPU Cores
	onlineCPUs := int(raw.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = len(raw.CPUStats.CPUUsage.PercpuUsage)
	}
	if onlineCPUs == 0 {
		onlineCPUs = runtime.NumCPU()
	}
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}

	// 2. CPU Percentage
	var cpuPercent float64
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemUsage) - float64(raw.PreCPUStats.SystemUsage)

	if raw.PreCPUStats.SystemUsage > 0 && systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(onlineCPUs) * 100.0
	} else if containerID != "" {
		m.cpuMu.Lock()
		prev, hasPrev := m.cpuCache[containerID]
		m.cpuCache[containerID] = cpuSample{
			readAt:      time.Now(),
			totalUsage:  raw.CPUStats.CPUUsage.TotalUsage,
			systemUsage: raw.CPUStats.SystemUsage,
		}
		m.cpuMu.Unlock()

		if hasPrev {
			cDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(prev.totalUsage)
			sDelta := float64(raw.CPUStats.SystemUsage) - float64(prev.systemUsage)
			if raw.CPUStats.SystemUsage > 0 && prev.systemUsage > 0 && sDelta > 0 && cDelta > 0 {
				cpuPercent = (cDelta / sDelta) * float64(onlineCPUs) * 100.0
			} else {
				elapsedNs := float64(time.Since(prev.readAt).Nanoseconds())
				if elapsedNs > 0 && cDelta > 0 {
					cpuPercent = (cDelta / elapsedNs) * 100.0
				}
			}
		}
	}
	if cpuPercent < 0 {
		cpuPercent = 0
	}
	cpuPercent = math.Round(cpuPercent*100) / 100

	// 3. Memory
	memUsage := raw.MemoryStats.Usage
	if inactive, ok := raw.MemoryStats.Stats["inactive_file"]; ok && memUsage > inactive {
		memUsage -= inactive
	} else if inactive, ok := raw.MemoryStats.Stats["total_inactive_file"]; ok && memUsage > inactive {
		memUsage -= inactive
	}

	memLimit := raw.MemoryStats.Limit
	var memPercent float64
	if memLimit > 0 && memLimit < (1<<60) {
		memPercent = (float64(memUsage) / float64(memLimit)) * 100.0
		memPercent = math.Round(memPercent*100) / 100
	}

	// 4. Disk I/O (Blkio)
	var diskRead, diskWrite uint64
	for _, entry := range raw.BlkioStats.IoServiceBytesRecursive {
		op := strings.ToLower(entry.Op)
		if strings.Contains(op, "read") {
			diskRead += entry.Value
		} else if strings.Contains(op, "write") {
			diskWrite += entry.Value
		}
	}

	// 5. Networks
	var netRx, netTx uint64
	for _, netStat := range raw.Networks {
		netRx += netStat.RxBytes
		netTx += netStat.TxBytes
	}

	return ContainerStats{
		CPUPercent:       cpuPercent,
		CPUCores:         onlineCPUs,
		MemoryUsageBytes: memUsage,
		MemoryLimitBytes: memLimit,
		MemoryPercent:    memPercent,
		DiskReadBytes:    diskRead,
		DiskWriteBytes:   diskWrite,
		NetRxBytes:       netRx,
		NetTxBytes:       netTx,
	}
}
