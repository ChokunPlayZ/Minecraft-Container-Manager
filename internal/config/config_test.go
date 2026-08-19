package config

import (
	"path/filepath"
	"strings"
	"testing"
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
