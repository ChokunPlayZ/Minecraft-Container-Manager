package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/jars"
)

func readJarFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "jars", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func serveJarFixture(t *testing.T, mux *http.ServeMux, path, name string) {
	t.Helper()
	body := readJarFixture(t, name)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
}

// newJarsTestServer returns an api.Server whose jars resolver points at
// httptest fixtures for all supported providers.
func newJarsTestServer(t *testing.T) *Server {
	t.Helper()

	paperMux := http.NewServeMux()
	serveJarFixture(t, paperMux, "/projects/paper", "paper_versions.json")
	serveJarFixture(t, paperMux, "/projects/paper/versions/1.21.1/builds", "paper_builds.json")
	paper := httptest.NewServer(paperMux)

	fabricMux := http.NewServeMux()
	serveJarFixture(t, fabricMux, "/versions/game", "fabric_game.json")
	serveJarFixture(t, fabricMux, "/versions/loader/1.21.1", "fabric_loaders.json")
	fabric := httptest.NewServer(fabricMux)

	mojangMux := http.NewServeMux()
	serveJarFixture(t, mojangMux, "/mc/game/version_manifest_v2.json", "mojang_manifest.json")
	mojang := httptest.NewServer(mojangMux)

	otherMux := http.NewServeMux()
	serveJarFixture(t, otherMux, "/maven-metadata.json", "forge_metadata.json")
	serveJarFixture(t, otherMux, "/releases", "neoforge_releases.json")
	serveJarFixture(t, otherMux, "/", "spigot_versions.json")
	other := httptest.NewServer(otherMux)

	r := jars.NewResolverWithBases(nil, paper.URL, fabric.URL, mojang.URL+"/mc/game/version_manifest_v2.json")
	r.ForgeBase = other.URL
	r.NeoForgeBase = other.URL
	r.SpigotBase = other.URL

	t.Cleanup(func() {
		paper.Close()
		fabric.Close()
		mojang.Close()
		other.Close()
	})
	return &Server{jars: r}
}

func getJar(t *testing.T, s *Server, versions bool, pathVals map[string]string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/jars", nil)
	req = req.WithContext(context.Background())
	for k, v := range pathVals {
		req.SetPathValue(k, v)
	}
	rr := httptest.NewRecorder()
	if versions {
		s.handleJarVersions(rr, req)
	} else {
		s.handleJarBuilds(rr, req)
	}
	return rr.Code, rr.Body.Bytes()
}

func TestJarVersionsVanilla(t *testing.T) {
	s := newJarsTestServer(t)
	status, body := getJar(t, s, true, map[string]string{"kind": "vanilla"})
	if status != http.StatusOK {
		t.Fatalf("status = %d want 200; body=%s", status, body)
	}
	var versions []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &versions); err != nil {
		t.Fatalf("decode versions: %v; body=%s", err, body)
	}
	if len(versions) != 3 {
		t.Fatalf("got %d versions, want 3 (release+snapshot): %+v", len(versions), versions)
	}
	// The manifest has 1.20.4, 1.21.1 (release) and 25w01a (snapshot).
	want := map[string]bool{"1.20.4": true, "1.21.1": true, "25w01a": true}
	for _, v := range versions {
		if !want[v.Name] {
			t.Fatalf("unexpected version %q", v.Name)
		}
	}
}

func TestJarBuildsVanilla(t *testing.T) {
	s := newJarsTestServer(t)
	status, body := getJar(t, s, false, map[string]string{"kind": "vanilla", "v": "1.21.1"})
	if status != http.StatusOK {
		t.Fatalf("status = %d want 200; body=%s", status, body)
	}
	var builds []struct {
		Version string `json:"version"`
		Build   string `json:"build"`
	}
	if err := json.Unmarshal(body, &builds); err != nil {
		t.Fatalf("decode builds: %v; body=%s", err, body)
	}
	if len(builds) != 1 || builds[0].Build != "latest" || builds[0].Version != "1.21.1" {
		t.Fatalf("unexpected vanilla builds: %+v", builds)
	}
}

func TestJarUnsupportedTypeFriendly(t *testing.T) {
	s := newJarsTestServer(t)
	status, body := getJar(t, s, true, map[string]string{"kind": "bungee"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d want 400", status)
	}
	if !isFriendlyErrorBody(t, body) {
		t.Fatalf("expected friendly error, got raw body: %s", body)
	}
	if bytes.Contains(body, []byte("unsupported jar type")) {
		t.Fatalf("error body leaked raw data: %s", body)
	}
}

// TestJarVersionsLogsUpstreamFailure verifies that an upstream-provider fetch
// failure is written to the backend log (with the underlying error) before the
// generic 502 is returned, so the failing provider is diagnosable.
func TestJarVersionsLogsUpstreamFailure(t *testing.T) {
	failMux := http.NewServeMux()
	failMux.HandleFunc("/projects/paper", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "backend exploded", http.StatusBadGateway)
	})
	fail := httptest.NewServer(failMux)
	t.Cleanup(fail.Close)

	r := jars.NewResolverWithBases(nil, fail.URL, "http://invalid.invalid/v2", "http://invalid.invalid/mc/game/version_manifest_v2.json")

	var buf bytes.Buffer
	s := &Server{jars: r, logger: log.New(&buf, "", 0)}

	req := httptest.NewRequest(http.MethodGet, "/api/jars/paper/versions", nil)
	req.SetPathValue("kind", "paper")
	rr := httptest.NewRecorder()
	s.handleJarVersions(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d want 502", rr.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "upstream provider failure") {
		t.Fatalf("expected upstream failure log, got: %q", out)
	}
	if !strings.Contains(out, fail.URL) {
		t.Fatalf("expected provider URL in log, got: %q", out)
	}
	if !strings.Contains(out, "502") {
		t.Fatalf("expected HTTP status in log, got: %q", out)
	}
}

func isFriendlyErrorBody(t *testing.T, body []byte) bool {
	t.Helper()
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("decode error body: %v; body=%s", err, body)
	}
	if e.Error.Code == "" || e.Error.Message == "" {
		t.Fatalf("missing error code/message: %s", body)
	}
	return true
}
