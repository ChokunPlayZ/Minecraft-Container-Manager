package docker

import (
	"strings"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestExposedPorts(t *testing.T) {
	eps := exposedPorts([]ExtraPort{
		{ContainerPort: 8080, Protocol: "tcp"},
		{ContainerPort: 19132, Protocol: "udp"},
	})

	want := nat.PortSet{
		nat.Port("25565/tcp"): struct{}{},
		nat.Port("8080/tcp"):  struct{}{},
		nat.Port("19132/udp"): struct{}{},
	}
	if len(eps) != len(want) {
		t.Fatalf("expected %d exposed ports, got %d: %v", len(want), len(eps), eps)
	}
	for p := range want {
		if _, ok := eps[p]; !ok {
			t.Errorf("missing exposed port %s in %v", p, eps)
		}
	}
}

func TestExposedPortsNormalizesInvalidProtocol(t *testing.T) {
	eps := exposedPorts([]ExtraPort{
		{ContainerPort: 9000, Protocol: "bogus"},
	})

	if _, ok := eps[nat.Port("9000/tcp")]; !ok {
		t.Fatalf("expected invalid protocol to normalize to tcp, got %v", eps)
	}
}

func TestPortBindings(t *testing.T) {
	pm := portBindings(CreateOpts{
		HostPort: 25601,
		ExtraPorts: []ExtraPort{
			{ID: "webui", HostPort: 8081, ContainerPort: 8080, Protocol: "tcp"},
			{ID: "geyser", HostPort: 19133, ContainerPort: 19132, Protocol: "udp"},
		},
	})

	check := func(key, hostPort string) {
		t.Helper()
		bindings, ok := pm[nat.Port(key)]
		if !ok {
			t.Fatalf("missing binding for %s in %v", key, pm)
		}
		if len(bindings) != 1 {
			t.Fatalf("expected one binding for %s, got %d", key, len(bindings))
		}
		if bindings[0].HostIP != "0.0.0.0" {
			t.Errorf("expected host ip 0.0.0.0 for %s, got %q", key, bindings[0].HostIP)
		}
		if bindings[0].HostPort != hostPort {
			t.Errorf("expected host port %s for %s, got %q", hostPort, key, bindings[0].HostPort)
		}
	}

	check("25565/tcp", "25601")
	check("8080/tcp", "8081")
	check("19132/udp", "19133")
}

func TestItzgEnv(t *testing.T) {
	cases := []struct {
		name    string
		opts    CreateOpts
		wantEnv map[string]string
	}{
		{
			name: "paper",
			opts: CreateOpts{ServerType: "paper", Version: "1.21.1", Build: "120", RAMMB: 2048},
			wantEnv: map[string]string{
				"TYPE":                   "PAPER",
				"VERSION":                "1.21.1",
				"MEMORY":                 "2048M",
				"EULA":                   "TRUE",
				"BUILD_NUMBER":           "120",
				"MCM_DATA_DIR":           "/data",
				"CREATE_CONSOLE_IN_PIPE": "true",
			},
		},
		{
			name: "fabric",
			opts: CreateOpts{ServerType: "fabric", Version: "1.21.1", Build: "0.16.9", RAMMB: 4096},
			wantEnv: map[string]string{
				"TYPE":                   "FABRIC",
				"VERSION":                "1.21.1",
				"MEMORY":                 "4096M",
				"EULA":                   "TRUE",
				"FABRIC_LOADER":          "0.16.9",
				"MCM_DATA_DIR":           "/data",
				"CREATE_CONSOLE_IN_PIPE": "true",
			},
		},
		{
			name: "vanilla has no build var",
			opts: CreateOpts{ServerType: "vanilla", Version: "1.21.1", Build: "", RAMMB: 1024},
			wantEnv: map[string]string{
				"TYPE":                   "VANILLA",
				"VERSION":                "1.21.1",
				"MEMORY":                 "1024M",
				"EULA":                   "TRUE",
				"MCM_DATA_DIR":           "/data",
				"CREATE_CONSOLE_IN_PIPE": "true",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := itzgEnv(tc.opts)
			got := make(map[string]string)
			for _, kv := range env {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					t.Fatalf("malformed env entry %q", kv)
				}
				got[k] = v
			}
			if len(got) != len(tc.wantEnv) {
				t.Fatalf("env count = %d (want %d): %v", len(got), len(tc.wantEnv), got)
			}
			for k, wantV := range tc.wantEnv {
				if got[k] != wantV {
					t.Errorf("env %s = %q, want %q", k, got[k], wantV)
				}
			}
		})
	}
}
