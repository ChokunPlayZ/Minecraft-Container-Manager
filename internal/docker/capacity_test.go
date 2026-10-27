package docker

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

type capacityTransport struct {
	fakeDaemon
	body   string
	status int
}

func (d *capacityTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(r.URL.Path, "/info") {
		return d.fakeDaemon.RoundTrip(r)
	}
	return d.response(d.status, d.body)
}

func TestHostCapacity(t *testing.T) {
	for _, tt := range []struct {
		name      string
		runtime   string
		body      string
		status    int
		cpu       int
		memory    uint64
		wantError bool
	}{
		{"docker host", "docker", `{"NCPU":12,"MemTotal":68719476736}`, 200, 12, 64 << 30, false},
		{"podman host", "podman", `{"NCPU":8,"MemTotal":34359738368}`, 200, 8, 32 << 30, false},
		{"missing metrics", "podman", `{}`, 200, 0, 0, false},
		{"invalid metrics", "docker", `{"NCPU":-1,"MemTotal":-1}`, 200, 0, 0, false},
		{"unavailable", "docker", `{"message":"daemon unavailable"}`, 500, 0, 0, true},
		{"invalid response", "docker", `invalid`, 200, 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			daemon := &capacityTransport{body: tt.body, status: tt.status}
			cli, err := client.NewClientWithOpts(client.WithHost("tcp://remote-host"), client.WithVersion("1.40"), client.WithHTTPClient(&http.Client{Transport: daemon}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cli.Close() })
			manager := &Manager{client: cli, runtime: tt.runtime}
			capacity, err := manager.HostCapacity(context.Background())
			if (err != nil) != tt.wantError {
				t.Fatalf("HostCapacity error = %v", err)
			}
			if tt.wantError {
				return
			}
			if capacity.Runtime != tt.runtime || capacity.CPUCores != tt.cpu || capacity.MemoryTotalBytes != tt.memory {
				t.Fatalf("HostCapacity = %+v", capacity)
			}
		})
	}
}
