package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds the runtime configuration for MCM, populated from environment
// variables with sane defaults.
type Config struct {
	Addr          string
	PortRange     PortRange
	DataDir       string
	DBPath        string
	DockerHost    string
	SessionSecret []byte
	WebAuthn      WebAuthnConfig
	// S3 holds the object-storage configuration used for world backups. An
	// empty Endpoint disables backups.
	S3 S3Config
	// SecureCookies forces Secure on session cookies. It is controlled by
	// MCM_TLS and defaults to false.
	SecureCookies bool
}

// WebAuthnConfig configures the WebAuthn (passkey) relying party.
type WebAuthnConfig struct {
	// RPID is the relying party ID (the effective domain, e.g. "localhost").
	RPID string
	// RPOrigins lists the permitted origins, e.g. "https://example.com".
	RPOrigins []string
	// RPDisplayName is shown during passkey creation.
	RPDisplayName string
}

// S3Config describes a path-style S3-compatible object store endpoint.
type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
}

// PortRange is an inclusive range of host ports available for server containers.
type PortRange struct {
	Start int
	End   int
}

// EnvVar names, kept as constants so tests and docs can refer to them.
const (
	EnvAddr           = "MCM_ADDR"
	EnvPortRange      = "MCM_PORT_RANGE"
	EnvDataDir        = "MCM_DATA_DIR"
	EnvDBPath         = "MCM_DB_PATH"
	EnvDockerHost     = "DOCKER_HOST"
	EnvSessionSecret  = "MCM_SESSION_SECRET"
	EnvTLS            = "MCM_TLS"
	EnvWebAuthnRPID   = "MCM_WEB_AUTHN_RPID"
	EnvWebAuthnOrigin = "MCM_WEB_AUTHN_RP_ORIGIN"
	EnvWebAuthnName   = "MCM_WEB_AUTHN_RP_NAME"
	EnvS3Endpoint     = "MCM_S3_ENDPOINT"
	EnvS3AccessKey    = "MCM_S3_ACCESS_KEY"
	EnvS3SecretKey    = "MCM_S3_SECRET_KEY"
	EnvS3Bucket       = "MCM_S3_BUCKET"
	EnvS3Region       = "MCM_S3_REGION"
)

const (
	defaultAddr          = ":8080"
	defaultPortRange     = "25565-25665"
	defaultDataDir       = "./data"
	defaultDockerHost    = "unix:///var/run/docker.sock"
	defaultSessionSecret = ""
	defaultRPID          = "localhost"
	defaultRPName        = "Minecraft Container Manager"
)

// Load builds a Config from the environment. Missing variables fall back to
// defaults. It returns an error only when the configured port range is invalid.
func Load() (*Config, error) {
	cfg := &Config{
		Addr:       getenv(EnvAddr, defaultAddr),
		DataDir:    getenv(EnvDataDir, defaultDataDir),
		DockerHost: getenv(EnvDockerHost, defaultDockerHost),
		WebAuthn: WebAuthnConfig{
			RPID:          getenv(EnvWebAuthnRPID, defaultRPID),
			RPOrigins:     splitOrigins(getenv(EnvWebAuthnOrigin, "")),
			RPDisplayName: getenv(EnvWebAuthnName, defaultRPName),
		},
		S3: S3Config{
			Endpoint:  getenv(EnvS3Endpoint, ""),
			AccessKey: getenv(EnvS3AccessKey, ""),
			SecretKey: getenv(EnvS3SecretKey, ""),
			Bucket:    getenv(EnvS3Bucket, ""),
			Region:    getenv(EnvS3Region, "us-east-1"),
		},
	}

	pr, err := ParsePortRange(getenv(EnvPortRange, defaultPortRange))
	if err != nil {
		return nil, err
	}
	cfg.PortRange = pr

	dbPath := getenv(EnvDBPath, "")
	if dbPath == "" {
		dbPath = filepath.Join(cfg.DataDir, "mcm.db")
	}
	cfg.DBPath = dbPath

	secret := getenv(EnvSessionSecret, defaultSessionSecret)
	if secret == "" {
		// Ephemeral in-memory secret: sessions will not survive a restart.
		secret = randomSecret()
	}
	cfg.SessionSecret = []byte(secret)

	if tls := getenv(EnvTLS, ""); tls != "" {
		if b, err := strconv.ParseBool(tls); err == nil {
			cfg.SecureCookies = b
		}
	}
	if len(cfg.WebAuthn.RPOrigins) == 0 {
		cfg.WebAuthn.RPOrigins = []string{defaultOrigin(cfg.Addr)}
	}

	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitOrigins(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultOrigin(addr string) string {
	host := "localhost"
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		if p := addr[i+1:]; p != "" {
			host += ":" + p
		}
	}
	return "http://" + host
}

// ParsePortRange parses "start-end". Both ends are inclusive. It rejects
// reversed, empty, or out-of-range values.
func ParsePortRange(s string) (PortRange, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return PortRange{}, fmt.Errorf("invalid port range %q: expected start-end", s)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return PortRange{}, fmt.Errorf("invalid port range %q: bad start: %w", s, err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return PortRange{}, fmt.Errorf("invalid port range %q: bad end: %w", s, err)
	}
	if start < 1 || end > 65535 || start > end {
		return PortRange{}, fmt.Errorf("invalid port range %q", s)
	}
	return PortRange{Start: start, End: end}, nil
}

func randomSecret() string {
	// Ephemeral secret used only so that empty MCM_SESSION_SECRET does not
	// leave an empty signing key. It is replaced on every restart.
	return fmt.Sprintf("ephemeral-%d", os.Getpid())
}
