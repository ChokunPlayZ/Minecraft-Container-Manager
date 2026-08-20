package config

import (
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
	t.Setenv(EnvDBPath, "")
	t.Setenv(EnvDockerHost, "")
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
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.DBPath != filepath.Join("./data", "mcm.db") {
		t.Errorf("DBPath = %q", cfg.DBPath)
	}
	if len(cfg.SessionSecret) == 0 {
		t.Error("expected a non-empty ephemeral session secret")
	}
	if len(cfg.WebAuthn.RPOrigins) != 1 || cfg.WebAuthn.RPOrigins[0] != "http://localhost:8080" {
		t.Errorf("WebAuthn RPOrigins = %v, want [http://localhost:8080]", cfg.WebAuthn.RPOrigins)
	}
	if cfg.WebAuthn.RPID != "localhost" {
		t.Errorf("WebAuthn RPID = %q, want localhost", cfg.WebAuthn.RPID)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv(EnvAddr, ":9090")
	t.Setenv(EnvPortRange, "31000-31100")
	t.Setenv(EnvDataDir, "/tmp/mcm")
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
	t.Setenv(EnvDBPath, "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(cfg.DBPath, "/srv/mcm") {
		t.Errorf("DBPath = %q, expected under /srv/mcm", cfg.DBPath)
	}
}

func TestLoadHardeningDefaults(t *testing.T) {
	t.Setenv(EnvTLSCert, "")
	t.Setenv(EnvTLSKey, "")
	t.Setenv(EnvTLSRedirect, "")
	t.Setenv(EnvTLSRedirectAddr, "")
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
	if !cfg.TLSRedirect {
		t.Error("TLSRedirect should default to true")
	}
	if cfg.TLSRedirectAddr != ":80" {
		t.Errorf("TLSRedirectAddr = %q, want :80", cfg.TLSRedirectAddr)
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
	if cfg.ServerImage != "itzg/minecraft-server" {
		t.Errorf("ServerImage = %q, want itzg/minecraft-server", cfg.ServerImage)
	}
}

func TestLoadHardeningOverrides(t *testing.T) {
	t.Setenv(EnvAddr, ":9090")
	t.Setenv(EnvTLSCert, "/certs/tls.crt")
	t.Setenv(EnvTLSKey, "/certs/tls.key")
	t.Setenv(EnvTLSRedirect, "false")
	t.Setenv(EnvTLSRedirectAddr, ":7666")
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
	if cfg.TLSRedirect {
		t.Error("TLSRedirect should be false")
	}
	if cfg.TLSRedirectAddr != ":7666" {
		t.Errorf("TLSRedirectAddr = %q, want :7666", cfg.TLSRedirectAddr)
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

func TestRedirectAddrFor(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{":8080", ":80"},
		{"0.0.0.0:8443", "0.0.0.0:80"},
		{"mc.example.com:443", "mc.example.com:80"},
	}
	for _, tc := range cases {
		if got := redirectAddrFor(tc.in); got != tc.want {
			t.Errorf("redirectAddrFor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
