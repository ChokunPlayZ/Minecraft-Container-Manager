package docker

import (
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
