package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParsePortRange(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
		start   int
		end     int
	}{
		{in: "25565-25665", start: 25565, end: 25665},
		{in: "30000-30000", start: 30000, end: 30000},
		{in: "10-20", start: 10, end: 20},
		{in: "abc-def", wantErr: true},
		{in: "25665-25565", wantErr: true},
		{in: "1-70000", wantErr: true},
		{in: "0-100", wantErr: true},
		{in: "10", wantErr: true},
	}
	for _, tc := range tests {
		got, err := ParsePortRange(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParsePortRange(%q): expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParsePortRange(%q): %v", tc.in, err)
			continue
		}
		if got.Start != tc.start || got.End != tc.end {
			t.Errorf("ParsePortRange(%q) = %d-%d, want %d-%d", tc.in, got.Start, got.End, tc.start, tc.end)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv(EnvPortRange, "")
	t.Setenv(EnvDataDir, "")
	t.Setenv(EnvBackupDir, "")
	t.Setenv(EnvDBPath, "")
	t.Setenv(EnvDockerHost, "")
	t.Setenv(EnvContainerRuntime, "")
	t.Setenv(EnvContainerHost, "")
	t.Setenv(EnvPodmanHost, "")
	t.Setenv(EnvSessionSecret, "")
	t.Setenv(EnvAddr, "")
	t.Setenv(EnvTLS, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.PortRange.Start != 25565 || cfg.PortRange.End != 25665 {
		t.Errorf("PortRange = %d-%d", cfg.PortRange.Start, cfg.PortRange.End)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.BackupDir != filepath.Join(cfg.DataDir, "backups") {
		t.Errorf("BackupDir = %q, want data directory's backups folder", cfg.BackupDir)
	}
	if cfg.DataDirHost != "" {
		t.Errorf("DataDirHost = %q, want empty by default", cfg.DataDirHost)
	}
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.ContainerRuntime != "docker" {
		t.Errorf("ContainerRuntime = %q, want docker", cfg.ContainerRuntime)
	}
	if cfg.DBPath != filepath.Join("./data", "mcm.db") {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if len(cfg.SessionSecret) == 0 {
		t.Error("expected a non-empty ephemeral session secret")
	}
	if cfg.HTTPOnly || !cfg.SecureCookies {
		t.Error("native HTTPS and secure cookies should be enabled by default")
	}
	if len(cfg.WebAuthn.RPOrigins) != 1 || cfg.WebAuthn.RPOrigins[0] != "https://localhost:8080" {
		t.Errorf("WebAuthn RPOrigins = %v, want [https://localhost:8080]", cfg.WebAuthn.RPOrigins)
	}
	if cfg.WebAuthn.RPID != "localhost" {
		t.Errorf("WebAuthn RPID = %q, want localhost", cfg.WebAuthn.RPID)
	}
}

func TestContainerRuntimeEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, runtime, mcmHost, dockerHost, podmanHost, want string
	}{
		{name: "Docker default", want: defaultDockerHost},
		{name: "Docker ignores Podman host", podmanHost: "unix:///podman.sock", want: defaultDockerHost},
		{name: "Docker existing override", dockerHost: "tcp://localhost:2375", want: "tcp://localhost:2375"},
		{name: "Docker bare socket path", dockerHost: "/var/run/docker.sock", want: "unix:///var/run/docker.sock"},
		{name: "Podman host", runtime: "podman", podmanHost: "unix:///custom/podman.sock", want: "unix:///custom/podman.sock"},
		{name: "Podman bare socket path", runtime: "podman", podmanHost: "/run/user/1000/podman/podman.sock", want: "unix:///run/user/1000/podman/podman.sock"},
		{name: "Podman bare Docker host", runtime: "podman", dockerHost: "/run/user/1000/podman/podman.sock", podmanHost: "unix:///ignored.sock", want: "unix:///run/user/1000/podman/podman.sock"},
		{name: "MCM bare socket priority", runtime: "podman", mcmHost: "/run/user/1000/podman/podman.sock", dockerHost: "unix:///ignored.sock", podmanHost: "unix:///also-ignored.sock", want: "unix:///run/user/1000/podman/podman.sock"},
		{name: "Docker host compatibility", runtime: "podman", dockerHost: "unix:///compat.sock", podmanHost: "unix:///ignored.sock", want: "unix:///compat.sock"},
		{name: "MCM host priority", runtime: "podman", mcmHost: "unix:///mcm.sock", dockerHost: "unix:///ignored.sock", podmanHost: "unix:///also-ignored.sock", want: "unix:///mcm.sock"},
		{name: "Case and whitespace", runtime: " Podman ", mcmHost: "unix:///mcm.sock", want: "unix:///mcm.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvContainerRuntime, tc.runtime)
			t.Setenv(EnvContainerHost, tc.mcmHost)
			t.Setenv(EnvDockerHost, tc.dockerHost)
			t.Setenv(EnvPodmanHost, tc.podmanHost)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DockerHost != tc.want {
				t.Errorf("endpoint = %q, want %q", cfg.DockerHost, tc.want)
			}
		})
	}
}

func TestPodmanDefaultEndpoint(t *testing.T) {
	t.Setenv(EnvContainerRuntime, "podman")
	t.Setenv(EnvContainerHost, "")
	t.Setenv(EnvDockerHost, "")
	t.Setenv(EnvPodmanHost, "")
	for _, runtimeDir := range []string{"/custom/run", ""} {
		t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if runtimeDir == "" {
			runtimeDir = fmt.Sprintf("/run/user/%d", os.Geteuid())
		}
		want := "unix://" + filepath.Join(runtimeDir, "podman/podman.sock")
		if os.Geteuid() == 0 {
			want = "unix:///run/podman/podman.sock"
		}
		if cfg.DockerHost != want {
			t.Errorf("endpoint = %q, want %q", cfg.DockerHost, want)
		}
	}
}

func TestLoadRejectsUnknownContainerRuntime(t *testing.T) {
	t.Setenv(EnvContainerRuntime, "containerd")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), EnvContainerRuntime) {
		t.Fatalf("expected runtime validation error, got %v", err)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv(EnvAddr, ":9090")
	t.Setenv(EnvPortRange, "31000-31100")
	t.Setenv(EnvDataDir, "/tmp/mcm")
	t.Setenv(EnvBackupDir, "/mnt/backup-media/mcm")
	t.Setenv(EnvDataDirHost, "/host/mcm")
	t.Setenv(EnvDBPath, "/tmp/custom.db")
	t.Setenv(EnvDockerHost, "tcp://127.0.0.1:2375")
	t.Setenv(EnvSessionSecret, "super-secret")
	t.Setenv(EnvTLS, "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":9090" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.DataDirHost != "/host/mcm" {
		t.Errorf("DataDirHost = %q, want /host/mcm", cfg.DataDirHost)
	}
	if cfg.BackupDir != "/mnt/backup-media/mcm" {
		t.Errorf("BackupDir = %q, want separate backup media", cfg.BackupDir)
	}
	if cfg.PortRange.Start != 31000 || cfg.PortRange.End != 31100 {
		t.Errorf("PortRange = %d-%d", cfg.PortRange.Start, cfg.PortRange.End)
	}
	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if !cfg.SecureCookies {
		t.Error("SecureCookies should be true when MCM_TLS=true")
	}
	t.Setenv(EnvWebAuthnRPID, "mc.example.com")
	t.Setenv(EnvWebAuthnOrigin, "https://mc.example.com")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.WebAuthn.RPID != "mc.example.com" {
		t.Errorf("WebAuthn RPID = %q, want mc.example.com", cfg.WebAuthn.RPID)
	}
	if len(cfg.WebAuthn.RPOrigins) != 1 || cfg.WebAuthn.RPOrigins[0] != "https://mc.example.com" {
		t.Errorf("WebAuthn RPOrigins = %v, want [https://mc.example.com]", cfg.WebAuthn.RPOrigins)
	}
}

func TestLoadRejectsBadPortRange(t *testing.T) {
	t.Setenv(EnvPortRange, "bad")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid port range")
	}
}

func TestDefaultDBPathUsesDataDir(t *testing.T) {
	t.Setenv(EnvDataDir, "/srv/mcm")
	t.Setenv(EnvBackupDir, "")
	t.Setenv(EnvDBPath, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(cfg.DBPath, "/srv/mcm") {
		t.Errorf("DBPath = %q, expected under /srv/mcm", cfg.DBPath)
	}
	if cfg.BackupDir != "/srv/mcm/backups" {
		t.Errorf("BackupDir = %q, want /srv/mcm/backups", cfg.BackupDir)
	}
}

func TestLoadHardeningDefaults(t *testing.T) {
	t.Setenv(EnvTLSCert, "")
	t.Setenv(EnvTLSKey, "")
	t.Setenv(EnvLoginMaxAttempts, "")
	t.Setenv(EnvLoginLockout, "")
	t.Setenv(EnvRateLimitMax, "")
	t.Setenv(EnvRateLimitWindow, "")
	t.Setenv(EnvDefaultCPULimit, "")
	t.Setenv(EnvDefaultMemoryMB, "")
	t.Setenv(EnvAddr, ":8443")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		t.Errorf("TLS cert/key should default to empty, got %q / %q", cfg.TLSCert, cfg.TLSKey)
	}
	if cfg.LoginMaxAttempts != 5 {
		t.Errorf("LoginMaxAttempts = %d, want 5", cfg.LoginMaxAttempts)
	}
	if cfg.LoginLockout != 15*time.Minute {
		t.Errorf("LoginLockout = %v, want 15m", cfg.LoginLockout)
	}
	if cfg.RateLimitMax != 100 {
		t.Errorf("RateLimitMax = %d, want 100", cfg.RateLimitMax)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Errorf("RateLimitWindow = %v, want 1m", cfg.RateLimitWindow)
	}
	if cfg.DefaultCPULimit != 0 || cfg.DefaultMemoryMB != 0 {
		t.Errorf("default resource limits = %v / %d, want 0/0", cfg.DefaultCPULimit, cfg.DefaultMemoryMB)
	}
	if cfg.ServerImage != "eclipse-temurin:21-jre-alpine" {
		t.Errorf("ServerImage = %q, want eclipse-temurin:21-jre-alpine", cfg.ServerImage)
	}
}

func TestLoadHardeningOverrides(t *testing.T) {
	t.Setenv(EnvAddr, ":9090")
	t.Setenv(EnvTLSCert, "/certs/tls.crt")
	t.Setenv(EnvTLSKey, "/certs/tls.key")
	t.Setenv(EnvLoginMaxAttempts, "10")
	t.Setenv(EnvLoginLockout, "2m")
	t.Setenv(EnvRateLimitMax, "250")
	t.Setenv(EnvRateLimitWindow, "5m")
	t.Setenv(EnvDefaultCPULimit, "2.5")
	t.Setenv(EnvDefaultMemoryMB, "4096")
	t.Setenv(EnvServerImage, "itzg/minecraft-server:latest")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLSCert != "/certs/tls.crt" || cfg.TLSKey != "/certs/tls.key" {
		t.Errorf("TLS cert/key = %q / %q", cfg.TLSCert, cfg.TLSKey)
	}
	if cfg.LoginMaxAttempts != 10 {
		t.Errorf("LoginMaxAttempts = %d, want 10", cfg.LoginMaxAttempts)
	}
	if cfg.LoginLockout != 2*time.Minute {
		t.Errorf("LoginLockout = %v, want 2m", cfg.LoginLockout)
	}
	if cfg.RateLimitMax != 250 {
		t.Errorf("RateLimitMax = %d, want 250", cfg.RateLimitMax)
	}
	if cfg.RateLimitWindow != 5*time.Minute {
		t.Errorf("RateLimitWindow = %v, want 5m", cfg.RateLimitWindow)
	}
	if cfg.DefaultCPULimit != 2.5 {
		t.Errorf("DefaultCPULimit = %v, want 2.5", cfg.DefaultCPULimit)
	}
	if cfg.DefaultMemoryMB != 4096 {
		t.Errorf("DefaultMemoryMB = %d, want 4096", cfg.DefaultMemoryMB)
	}
	if cfg.ServerImage != "itzg/minecraft-server:latest" {
		t.Errorf("ServerImage = %q, want itzg/minecraft-server:latest", cfg.ServerImage)
	}
}

func TestLoadTLSModes(t *testing.T) {
	for _, tc := range []struct {
		name, httpOnly, forceSecure, cert, key string
		wantHTTP, wantSecure, wantError        bool
	}{
		{name: "self-signed default", wantSecure: true},
		{name: "supplied certificates", cert: "/cert.pem", key: "/key.pem", wantSecure: true},
		{name: "HTTP opt-out", httpOnly: "true", wantHTTP: true},
		{name: "HTTPS reverse proxy", httpOnly: "true", forceSecure: "true", wantHTTP: true, wantSecure: true},
		{name: "native HTTPS always secures cookies", forceSecure: "false", wantSecure: true},
		{name: "missing key", cert: "/cert.pem", wantError: true},
		{name: "missing certificate", key: "/key.pem", wantError: true},
		{name: "conflicting HTTP and certificates", httpOnly: "true", cert: "/cert.pem", key: "/key.pem", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(EnvHTTPOnly, tc.httpOnly)
			t.Setenv(EnvTLS, tc.forceSecure)
			t.Setenv(EnvTLSCert, tc.cert)
			t.Setenv(EnvTLSKey, tc.key)
			t.Setenv(EnvAddr, ":8080")
			t.Setenv(EnvWebAuthnOrigin, "")
			cfg, err := Load()
			if tc.wantError {
				if err == nil {
					t.Fatal("expected invalid TLS configuration to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.HTTPOnly != tc.wantHTTP || cfg.SecureCookies != tc.wantSecure {
				t.Errorf("HTTPOnly/SecureCookies = %v/%v, want %v/%v", cfg.HTTPOnly, cfg.SecureCookies, tc.wantHTTP, tc.wantSecure)
			}
			wantOrigin := "https://localhost:8080"
			if tc.wantHTTP {
				wantOrigin = "http://localhost:8080"
			}
			if cfg.WebAuthn.RPOrigins[0] != wantOrigin {
				t.Errorf("origin = %q, want %q", cfg.WebAuthn.RPOrigins[0], wantOrigin)
			}
		})
	}
}

func TestLoadTLSRedirect(t *testing.T) {
	for _, value := range []string{"", "false", "true", "1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvTLSRedirect, value)
			// This legacy setting must not select a second listener.
			t.Setenv("MCM_TLS_REDIRECT_ADDR", ":80")
			t.Setenv(EnvAddr, ":9090")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.TLSRedirect != (value == "true" || value == "1") || cfg.Addr != ":9090" {
				t.Fatalf("TLSRedirect/Addr = %v/%q for %q", cfg.TLSRedirect, cfg.Addr, value)
			}
		})
	}
}
