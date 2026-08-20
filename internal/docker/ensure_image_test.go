package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/docker/docker/client"
)

const testImage = "itzg/minecraft-server:test"

// fakeDaemon is an in-process Docker Engine API daemon backed by a scripted
// sequence of image/pull/create responses. It exercises image-presence checks
// and pull-on-create without a real daemon or a bound port.
type fakeDaemon struct {
	haveImage       atomic.Bool
	pullCount       atomic.Int32
	createCount     atomic.Int32
	failFirstCreate atomic.Bool
}

// RoundTrip implements http.RoundTripper over the Docker Engine API.
func (d *fakeDaemon) RoundTrip(req *http.Request) (*http.Response, error) {
	p := req.URL.Path
	switch {
	case strings.HasSuffix(p, "/images/json"):
		if d.haveImage.Load() {
			return d.response(http.StatusOK, `[{"Id":"sha256:abc","RepoTags":["`+testImage+`"]}]`)
		}
		return d.response(http.StatusOK, `[]`)
	case strings.HasSuffix(p, "/images/create"):
		d.pullCount.Add(1)
		d.haveImage.Store(true)
		// Progress stream: newline-delimited JSON status objects.
		return d.response(http.StatusOK, `{"status":"pulling from `+testImage+`"}
{"status":"Pull complete"}
{"status":"Status: Downloaded newer image for `+testImage+`"}
`)
	case strings.HasSuffix(p, "/containers/create"):
		d.createCount.Add(1)
		if d.failFirstCreate.Load() && d.createCount.Load() == 1 {
			return d.response(http.StatusNotFound, `{"message":"No such image: `+testImage+`"}`)
		}
		return d.response(http.StatusCreated, `{"Id":"container-1234"}`)
	default:
		return d.response(http.StatusNotFound, `{"message":"not found"}`)
	}
}

func (d *fakeDaemon) response(status int, body string) (*http.Response, error) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    &http.Request{Method: http.MethodGet},
	}, nil
}

// newFakeManager builds a Manager bound to the fake daemon with a fixed API
// version to avoid version-negotiation traffic.
func newFakeManager(d *fakeDaemon) *Manager {
	cli, err := client.NewClientWithOpts(
		client.WithHost("tcp://fake"),
		client.WithHTTPClient(&http.Client{Transport: d}),
		client.WithVersion("1.44"),
	)
	if err != nil {
		panic(err)
	}
	return &Manager{client: cli, host: "tcp://fake", image: testImage}
}

func TestCreatePullsImageWhenMissing(t *testing.T) {
	d := &fakeDaemon{}
	m := newFakeManager(d)
	ctx := context.Background()

	cid, err := m.Create(ctx, CreateOpts{
		ID:         "zip",
		HostPort:   25601,
		DataDir:    "/tmp/data",
		ServerType: "paper",
		Version:    "1.21.1",
		Build:      "120",
		RAMMB:      2048,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if cid != "container-1234" {
		t.Fatalf("unexpected container id %q", cid)
	}
	if d.pullCount.Load() != 1 {
		t.Fatalf("expected 1 pull, got %d", d.pullCount.Load())
	}
	if d.createCount.Load() != 1 {
		t.Fatalf("expected 1 create, got %d", d.createCount.Load())
	}
}

func TestCreateSkipsPullWhenImagePresent(t *testing.T) {
	d := &fakeDaemon{}
	d.haveImage.Store(true)
	m := newFakeManager(d)
	ctx := context.Background()

	if _, err := m.Create(ctx, CreateOpts{
		ID:         "zip",
		HostPort:   25601,
		DataDir:    "/tmp/data",
		ServerType: "fabric",
		Version:    "1.21.1",
		Build:      "0.15.11",
		RAMMB:      4096,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.pullCount.Load() != 0 {
		t.Fatalf("expected no pull when image present, got %d", d.pullCount.Load())
	}
}

func TestCreateRetriesAfterStaleCreateFailure(t *testing.T) {
	d := &fakeDaemon{}
	d.failFirstCreate.Store(true)
	d.haveImage.Store(true) // image present locally, but the create still races
	m := newFakeManager(d)
	ctx := context.Background()

	// The image is already local but the first create still reports the image
	// missing (a stale race); Create pulls and retries once.
	if _, err := m.Create(ctx, CreateOpts{
		ID:         "zip",
		HostPort:   25601,
		DataDir:    "/tmp/data",
		ServerType: "paper",
		Version:    "1.21.1",
		Build:      "120",
		RAMMB:      2048,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.createCount.Load() != 2 {
		t.Fatalf("expected 2 creates (retry), got %d", d.createCount.Load())
	}
	if d.pullCount.Load() != 1 {
		t.Fatalf("expected 1 pull, got %d", d.pullCount.Load())
	}
}

func TestItzgEnvBuildVars(t *testing.T) {
	cases := []struct {
		name string
		opts CreateOpts
		want []string
	}{
		{
			name: "paper",
			opts: CreateOpts{ServerType: "paper", Version: "1.21.1", Build: "120", RAMMB: 2048},
			want: []string{"TYPE=PAPER", "VERSION=1.21.1", "MEMORY=2048M", "EULA=TRUE", "BUILD_NUMBER=120"},
		},
		{
			name: "fabric",
			opts: CreateOpts{ServerType: "fabric", Version: "1.21.1", Build: "0.15.11", RAMMB: 4096},
			want: []string{"TYPE=FABRIC", "VERSION=1.21.1", "MEMORY=4096M", "EULA=TRUE", "FABRIC_LOADER=0.15.11"},
		},
		{
			name: "vanilla-no-build",
			opts: CreateOpts{ServerType: "vanilla", Version: "1.21", RAMMB: 1024},
			want: []string{"TYPE=VANILLA", "VERSION=1.21", "MEMORY=1024M", "EULA=TRUE"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := itzgEnv(c.opts)
			for _, w := range c.want {
				if !containsStr(env, w) {
					t.Errorf("env %v missing %q", env, w)
				}
			}
		})
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
