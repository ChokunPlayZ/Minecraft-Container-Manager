package jars

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func serveFixture(t *testing.T, mux *http.ServeMux, path, fixtureName string) {
	t.Helper()
	body := fixture(t, fixtureName)
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
}

func newTestResolver(t *testing.T) (*Resolver, *httptest.Server, *httptest.Server, *httptest.Server) {
	t.Helper()

	paperMux := http.NewServeMux()
	serveFixture(t, paperMux, "/projects/paper", "paper_versions.json")
	serveFixture(t, paperMux, "/projects/paper/versions/1.21.1/builds", "paper_builds.json")
	paper := httptest.NewServer(paperMux)

	fabricMux := http.NewServeMux()
	serveFixture(t, fabricMux, "/versions/game", "fabric_game.json")
	serveFixture(t, fabricMux, "/versions/loader/1.21.1", "fabric_loaders.json")
	fabric := httptest.NewServer(fabricMux)

	mojangMux := http.NewServeMux()
	serveFixture(t, mojangMux, "/mc/game/version_manifest_v2.json", "mojang_manifest.json")
	mojang := httptest.NewServer(mojangMux)

	r := NewResolverWithBases(nil, paper.URL, fabric.URL, mojang.URL+"/mc/game/version_manifest_v2.json")
	return r, paper, fabric, mojang
}

func TestPaperVersionsAndBuilds(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	versions, err := r.PaperVersions(context.Background())
	if err != nil {
		t.Fatalf("PaperVersions: %v", err)
	}
	if len(versions) != 3 || versions[0] != "1.20.1" || versions[2] != "1.21.1" {
		t.Fatalf("unexpected versions: %v", versions)
	}

	builds, err := r.PaperBuilds(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("PaperBuilds: %v", err)
	}
	if len(builds) != 3 || builds[2].Number != 120 {
		t.Fatalf("unexpected builds: %v", builds)
	}
}

func TestPaperResolve(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	res, err := r.Resolve(context.Background(), TypePaper, "1.21.1", "120")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Type != TypePaper || res.Version != "1.21.1" || res.Build != "120" {
		t.Fatalf("unexpected resolve: %+v", res)
	}

	// No build specified resolves to the newest build.
	res, err = r.Validate(context.Background(), TypePaper, "1.21.1", "")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Build != "120" {
		t.Fatalf("expected newest build 120, got %s", res.Build)
	}
}

func TestFabricVersionsLoadersAndResolve(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	games, err := r.FabricGameVersions(context.Background())
	if err != nil {
		t.Fatalf("FabricGameVersions: %v", err)
	}
	if len(games) != 3 || games[1] != "1.21.1" {
		t.Fatalf("unexpected games: %v", games)
	}

	loaders, err := r.FabricLoaders(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("FabricLoaders: %v", err)
	}
	if len(loaders) != 2 || loaders[1] != "0.15.5" {
		t.Fatalf("unexpected loaders: %v", loaders)
	}

	res, err := r.Resolve(context.Background(), TypeFabric, "1.21.1", "")
	if err != nil {
		t.Fatalf("Resolve fabric: %v", err)
	}
	if res.Type != TypeFabric || res.Build != "0.15.5" {
		t.Fatalf("unexpected fabric resolve: %+v", res)
	}
}

func TestVanillaValidate(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	res, err := r.Validate(context.Background(), TypeVanilla, "1.20.4", "")
	if err != nil {
		t.Fatalf("Validate vanilla: %v", err)
	}
	if res.Type != TypeVanilla || res.Version != "1.20.4" || res.Build != "" {
		t.Fatalf("unexpected vanilla resolve: %+v", res)
	}

	if _, err := r.Validate(context.Background(), TypeVanilla, "999.0", ""); err == nil {
		t.Fatal("expected error for unknown vanilla version")
	}
}

func TestParseJarType(t *testing.T) {
	for _, ok := range []string{"paper", "fabric", "vanilla", "forge", "neoforge", "spigot"} {
		if _, err := ParseJarType(ok); err != nil {
			t.Fatalf("ParseJarType(%q): %v", ok, err)
		}
	}
	if _, err := ParseJarType("bungee"); err == nil {
		t.Fatal("expected error for unsupported jar type")
	}
}

func TestForgeResolve(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.URL.Path {
		case "/maven-metadata.json":
			_, _ = w.Write([]byte(fixture(t, "forge_metadata.json")))
		case "/releases":
			_, _ = w.Write([]byte(fixture(t, "neoforge_releases.json")))
		case "/":
			_, _ = w.Write([]byte(fixture(t, "spigot_versions.json")))
		default:
			http.NotFound(w, req)
		}
	}))
	defer srv.Close()
	r.ForgeBase = srv.URL
	r.NeoForgeBase = srv.URL
	r.SpigotBase = srv.URL

	versions, err := r.ForgeGameVersions(context.Background())
	if err != nil {
		t.Fatalf("ForgeGameVersions: %v", err)
	}
	if len(versions) != 2 || versions[0] != "1.20.1" || versions[1] != "1.21.1" {
		t.Fatalf("unexpected forge versions: %v", versions)
	}

	builds, err := r.ForgeBuilds(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("ForgeBuilds: %v", err)
	}
	if len(builds) != 1 || builds[0] != "1.21.1-52.0.14" {
		t.Fatalf("unexpected forge builds: %v", builds)
	}

	res, err := r.Resolve(context.Background(), TypeForge, "1.21.1", "")
	if err != nil {
		t.Fatalf("Resolve forge: %v", err)
	}
	if res.Type != TypeForge || res.Build != "1.21.1-52.0.14" {
		t.Fatalf("unexpected forge resolve: %+v", res)
	}
}

func TestNeoForgeResolve(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/releases" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture(t, "neoforge_releases.json")))
	}))
	defer srv.Close()
	r.NeoForgeBase = srv.URL

	versions, err := r.NeoForgeGameVersions(context.Background())
	if err != nil {
		t.Fatalf("NeoForgeGameVersions: %v", err)
	}
	if len(versions) != 2 || versions[0] != "1.20.1" || versions[1] != "1.21.1" {
		t.Fatalf("unexpected neoforge versions: %v", versions)
	}

	builds, err := r.NeoForgeBuilds(context.Background(), "1.20.1")
	if err != nil {
		t.Fatalf("NeoForgeBuilds: %v", err)
	}
	if len(builds) != 2 || builds[0] != "1.20.1-47.1.3" || builds[1] != "1.20.1-47.2.0" {
		t.Fatalf("unexpected neoforge builds: %v", builds)
	}

	res, err := r.Resolve(context.Background(), TypeNeoForge, "1.20.1", "1.20.1-47.2.0")
	if err != nil {
		t.Fatalf("Resolve neoforge: %v", err)
	}
	if res.Type != TypeNeoForge || res.Build != "1.20.1-47.2.0" {
		t.Fatalf("unexpected neoforge resolve: %+v", res)
	}
}

func TestSpigotResolve(t *testing.T) {
	r, paper, fabric, mojang := newTestResolver(t)
	defer paper.Close()
	defer fabric.Close()
	defer mojang.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fixture(t, "spigot_versions.json")))
	}))
	defer srv.Close()
	r.SpigotBase = srv.URL

	versions, err := r.SpigotGameVersions(context.Background())
	if err != nil {
		t.Fatalf("SpigotGameVersions: %v", err)
	}
	if len(versions) != 2 || versions[0] != "1.20.4" || versions[1] != "1.21.1" {
		t.Fatalf("unexpected spigot versions: %v", versions)
	}

	builds, err := r.SpigotBuilds(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("SpigotBuilds: %v", err)
	}
	if len(builds) != 1 || builds[0] != "latest" {
		t.Fatalf("unexpected spigot builds: %v", builds)
	}

	res, err := r.Validate(context.Background(), TypeSpigot, "1.20.4", "")
	if err != nil {
		t.Fatalf("Validate spigot: %v", err)
	}
	if res.Type != TypeSpigot || res.Build != "" {
		t.Fatalf("unexpected spigot resolve: %+v", res)
	}
}
