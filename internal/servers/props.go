package servers

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// readProps parses a Java-style .properties file into a map. Lines that start
// with # are comments and are ignored.
func readProps(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		out[key] = val
	}
	return out, nil
}

// rconConfig holds the RCON settings read from a server's server.properties.
type rconConfig struct {
	Enabled  bool
	Password string
	Port     int
}

// readRCONConfig reads RCON settings from the server's data directory. If the
// file is missing or RCON is disabled, Enabled is false.
func (s *Store) readRCONConfig(id string) (rconConfig, error) {
	cfg := rconConfig{Enabled: false}
	props, err := readProps(filepath.Join(s.dataPath(id), "server.properties"))
	if err != nil {
		return cfg, err
	}
	if v, ok := props["enable-rcon"]; ok && strings.EqualFold(strings.TrimSpace(v), "true") {
		cfg.Enabled = true
	}
	cfg.Password = props["rcon.password"]
	if v, ok := props["rcon.port"]; ok {
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
			cfg.Port = p
		}
	}
	return cfg, nil
}
