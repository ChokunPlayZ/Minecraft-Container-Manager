package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/mcm-panel/mcm/internal/config"
)

// podmanDaemon exercises the Docker-compatible API at Podman's baseline version,
// including rootless host metadata and lowercase missing-image responses.
type podmanDaemon struct {
	fakeDaemon
	rootless     bool
	failInfo     bool
	imageFilters []string
	createdHost  container.HostConfig
	createdNet   network.NetworkingConfig
	lifecycle    []string
}

func (d *podmanDaemon) RoundTrip(req *http.Request) (*http.Response, error) {
	p := req.URL.Path
	switch {
	case strings.HasSuffix(p, "/_ping"):
		return d.response(http.StatusOK, "OK")
	case strings.HasSuffix(p, "/info"):
		if d.failInfo {
			return d.response(http.StatusInternalServerError, `{"message":"info unavailable"}`)
		}
		if d.rootless {
			return d.response(http.StatusOK, `{"SecurityOptions":["name=seccomp,profile=default","name=rootless"]}`)
		}
		return d.response(http.StatusOK, `{"SecurityOptions":["name=selinux"]}`)
	case strings.HasSuffix(p, "/images/json"):
		d.imageFilters = append(d.imageFilters, req.URL.Query().Get("filters"))
	case strings.HasSuffix(p, "/networks/"+NetworkName):
		return d.response(http.StatusOK, `{"Name":"`+NetworkName+`"}`)
	case strings.HasSuffix(p, "/containers/create"):
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		var payload struct {
			HostConfig       container.HostConfig
			NetworkingConfig network.NetworkingConfig
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		d.createdHost, d.createdNet = payload.HostConfig, payload.NetworkingConfig
		req.Body = io.NopCloser(bytes.NewReader(body))
		if d.failFirstCreate.Load() && d.createCount.Load() == 0 {
			d.createCount.Add(1)
			return d.response(http.StatusNotFound, `{"message":"no such image: image not known"}`)
		}
	case strings.HasSuffix(p, "/containers/container-1234/update"):
		return d.response(http.StatusOK, `{}`)
	case strings.HasSuffix(p, "/networks/"+NetworkName+"/connect"):
		return d.response(http.StatusOK, `{}`)
	case strings.HasSuffix(p, "/containers/container-1234/start"):
		d.lifecycle = append(d.lifecycle, "start")
		return d.response(http.StatusNoContent, "")
	case strings.HasSuffix(p, "/containers/container-1234/stop"):
		d.lifecycle = append(d.lifecycle, "stop:"+req.URL.Query().Get("t"))
		return d.response(http.StatusNoContent, "")
	case req.Method == http.MethodDelete && strings.HasSuffix(p, "/containers/container-1234"):
		d.lifecycle = append(d.lifecycle, "remove:"+req.URL.Query().Get("force"))
		return d.response(http.StatusNoContent, "")
	}
	return d.fakeDaemon.RoundTrip(req)
}

func newPodmanTestManager(t *testing.T, d *podmanDaemon) *Manager {
	t.Helper()
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://podman"),
		client.WithHTTPClient(&http.Client{Transport: d}),
		client.WithVersion("1.40"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	m := &Manager{client: cli, host: "tcp://podman", runtime: "podman", cpuCache: make(map[string]cpuSample)}
	m.image = m.imageReference(testImage)
	return m
}

func TestPodmanCreateAndLifecycle(t *testing.T) {
	for _, rootless := range []bool{false, true} {
		t.Run(fmt.Sprintf("rootless=%t", rootless), func(t *testing.T) {
			d := &podmanDaemon{rootless: rootless}
			m := newPodmanTestManager(t, d)
			ctx := context.Background()
			id, err := m.Create(ctx, CreateOpts{
				ID: "podman", DataDir: "/srv/mcm/servers/podman", HostPort: 25565,
				ServerType: "paper", RAMMB: 2048, CPULimit: 1.5,
				ExtraPorts: []ExtraPort{{HostPort: 19132, ContainerPort: 19132, Protocol: "udp"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if d.createdConfig.Image != "docker.io/itzg/minecraft-server:test" {
				t.Errorf("image = %q", d.createdConfig.Image)
			}
			if len(d.createdHost.Binds) != 1 || d.createdHost.Binds[0] != "/srv/mcm/servers/podman:/data:z" {
				t.Errorf("shared bind = %v", d.createdHost.Binds)
			}
			wantNS, wantUser := "", ""
			if rootless {
				wantNS = fmt.Sprintf("keep-id:uid=%d,gid=%d", os.Geteuid(), os.Getegid())
				wantUser = "0:0"
			}
			if string(d.createdHost.UsernsMode) != wantNS || d.createdConfig.User != wantUser {
				t.Errorf("namespace/user = %q/%q, want %q/%q", d.createdHost.UsernsMode, d.createdConfig.User, wantNS, wantUser)
			}
			if d.createdHost.NanoCPUs != 1_500_000_000 || d.createdHost.Memory != 4096*1024*1024 {
				t.Errorf("resource limits = %+v", d.createdHost.Resources)
			}
			for port, want := range map[nat.Port]string{"25565/tcp": "25565", "19132/udp": "19132"} {
				bindings := d.createdHost.PortBindings[port]
				if len(bindings) != 1 || bindings[0].HostPort != want {
					t.Errorf("%s binding = %v", port, bindings)
				}
			}
			endpoint := d.createdNet.EndpointsConfig[NetworkName]
			if endpoint == nil || len(endpoint.Aliases) != 1 || endpoint.Aliases[0] != "mcm-podman" {
				t.Errorf("network endpoint = %+v", endpoint)
			}
			if err := m.Start(ctx, id); err != nil {
				t.Fatal(err)
			}
			if err := m.Stop(ctx, id, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err := m.Kill(ctx, id); err != nil {
				t.Fatal(err)
			}
			if err := m.Remove(ctx, id); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(d.lifecycle, ","); got != "start,stop:15,stop:0,remove:1" {
				t.Errorf("lifecycle = %s", got)
			}
		})
	}
}

func TestPodmanRuntimeImages(t *testing.T) {
	m := &Manager{runtime: "podman", image: "quay.io/example/java:custom"}
	for _, tc := range []struct {
		version    int
		kind, want string
	}{
		{21, "paper", "docker.io/library/eclipse-temurin:21-jre-alpine"},
		{17, "spigot", "docker.io/library/eclipse-temurin:17-jdk-alpine"},
		{8, "bukkit", "docker.io/library/eclipse-temurin:8-jdk-alpine"},
		{0, "paper", "quay.io/example/java:custom"},
	} {
		if got := m.RuntimeImage(tc.version, tc.kind); got != tc.want {
			t.Errorf("RuntimeImage(%d, %q) = %q, want %q", tc.version, tc.kind, got, tc.want)
		}
	}
	for _, name := range []string{"quay.io/example/java:21", "localhost:5000/java:21", "localhost/java:21", "sha256:" + strings.Repeat("a", 64), "ghcr.io/example/java@sha256:" + strings.Repeat("b", 64)} {
		if got := m.imageReference(name); got != name {
			t.Errorf("explicit reference %q changed to %q", name, got)
		}
	}
}

func TestPodmanRuntimeStatus(t *testing.T) {
	d := &podmanDaemon{}
	m := newPodmanTestManager(t, d)
	status := m.RuntimeStatus(context.Background())
	if status.Runtime != "podman" || !status.Reachable || status.ImageReady || !strings.Contains(status.Error, "podman pull docker.io/itzg/minecraft-server:test") {
		t.Fatalf("missing image status = %+v", status)
	}
	if err := m.EnsureImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	status = m.RuntimeStatus(context.Background())
	if !status.Reachable || !status.ImageReady || status.Error != "" {
		t.Fatalf("ready status = %+v", status)
	}
	for _, filter := range d.imageFilters {
		if !strings.Contains(filter, "docker.io/itzg/minecraft-server:test") {
			t.Errorf("image lookup used unqualified reference: %s", filter)
		}
	}
}

func TestPodmanRetriesLowercaseMissingImage(t *testing.T) {
	d := &podmanDaemon{}
	d.haveImage.Store(true)
	d.failFirstCreate.Store(true)
	m := newPodmanTestManager(t, d)
	if _, err := m.Create(context.Background(), CreateOpts{ID: "retry", DataDir: "/data", RAMMB: 1024}); err != nil {
		t.Fatal(err)
	}
	if d.createCount.Load() != 2 || d.pullCount.Load() != 1 {
		t.Errorf("creates/pulls = %d/%d, want 2/1", d.createCount.Load(), d.pullCount.Load())
	}
}

func TestPodmanInfoFailureStopsCreation(t *testing.T) {
	d := &podmanDaemon{failInfo: true}
	d.haveImage.Store(true)
	m := newPodmanTestManager(t, d)
	if _, err := m.Create(context.Background(), CreateOpts{ID: "info", DataDir: "/data"}); err == nil || !strings.Contains(err.Error(), "inspect podman host") {
		t.Fatalf("expected host inspection error, got %v", err)
	}
	if d.createCount.Load() != 0 {
		t.Fatal("container created without knowing the user namespace mapping")
	}
}

func TestNewRejectsUnknownRuntime(t *testing.T) {
	if _, err := NewWithRuntime("containerd", "unix:///unused.sock", ""); err == nil {
		t.Fatal("expected unsupported runtime error")
	}
}

func TestPodmanBareSocketEndpoint(t *testing.T) {
	// Keep the socket below the Unix path length limit, including on macOS.
	dir, err := os.MkdirTemp("", "mcm-sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "podman.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_ping" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("API-Version", "1.40")
		_, _ = io.WriteString(w, "OK")
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	t.Setenv(config.EnvContainerRuntime, "podman")
	t.Setenv(config.EnvContainerHost, socketPath)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := NewWithRuntime(cfg.ContainerRuntime, cfg.DockerHost, cfg.ServerImage)
	if err != nil {
		t.Fatalf("create client from bare socket path: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ping, err := mgr.client.Ping(ctx)
	if err != nil {
		t.Fatalf("connect to configured socket: %v", err)
	}
	if ping.APIVersion != "1.40" {
		t.Fatalf("API version = %q, want 1.40", ping.APIVersion)
	}
}
