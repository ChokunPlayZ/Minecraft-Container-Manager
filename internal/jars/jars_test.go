package jars

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	allTypes := []string{
		"paper", "fabric", "vanilla", "forge", "neoforge", "spigot",
		"purpur", "folia", "quilt", "mohist", "youer", "ketting", "sponge",
		"limbo", "nanolimbo", "crucible", "pufferfish", "leaf",
		"waterfall", "bungeecord", "geysermc", "custom",
	}
	for _, ok := range allTypes {
		if _, err := ParseJarType(ok); err != nil {
			t.Fatalf("ParseJarType(%q): %v", ok, err)
		}
	}
	if _, err := ParseJarType("unknown_server_type"); err == nil {
		t.Fatal("expected error for unsupported jar type")
	}
}

func TestAvailableJavaVersions(t *testing.T) {
	r := NewResolver()
	releases, err := r.AvailableJavaVersions(context.Background())
	if err != nil {
		t.Fatalf("AvailableJavaVersions error: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("expected at least one java release")
	}
	has21 := false
	for _, rel := range releases {
		if rel.Version == 21 {
			has21 = true
			if !rel.IsLTS {
				t.Errorf("expected Java 21 to be LTS")
			}
		}
	}
	if !has21 {
		t.Errorf("expected Java 21 in releases list: %+v", releases)
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

	// Suffix matching: "47.2.0" matches "1.20.1-47.2.0"
	res, err = r.Resolve(context.Background(), TypeForge, "1.20.1", "47.2.0")
	if err != nil {
		t.Fatalf("Resolve forge with short version: %v", err)
	}
	if res.Build != "1.20.1-47.2.0" {
		t.Fatalf("expected 1.20.1-47.2.0, got %s", res.Build)
	}

	// Loader prefix matching: "forge-47.2.0" matches "1.20.1-47.2.0"
	res, err = r.Resolve(context.Background(), TypeForge, "1.20.1", "forge-47.2.0")
	if err != nil {
		t.Fatalf("Resolve forge with loader prefix: %v", err)
	}
	if res.Build != "1.20.1-47.2.0" {
		t.Fatalf("expected 1.20.1-47.2.0, got %s", res.Build)
	}

	// "latest" resolves newest build
	res, err = r.Resolve(context.Background(), TypeForge, "1.20.1", "latest")
	if err != nil {
		t.Fatalf("Resolve forge latest: %v", err)
	}
	if res.Build != "1.20.1-47.2.0" {
		t.Fatalf("expected 1.20.1-47.2.0, got %s", res.Build)
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

	// NeoForge prefix match: "neoforge-47.2.0"
	res, err = r.Resolve(context.Background(), TypeNeoForge, "1.20.1", "neoforge-47.2.0")
	if err != nil {
		t.Fatalf("Resolve neoforge prefix: %v", err)
	}
	if res.Build != "1.20.1-47.2.0" {
		t.Fatalf("expected 1.20.1-47.2.0, got %s", res.Build)
	}

	// NeoForge latest
	res, err = r.Resolve(context.Background(), TypeNeoForge, "1.20.1", "latest")
	if err != nil {
		t.Fatalf("Resolve neoforge latest: %v", err)
	}
	if res.Build != "1.20.1-47.2.0" {
		t.Fatalf("expected 1.20.1-47.2.0, got %s", res.Build)
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

func TestSpigotDownloadBuildTools(t *testing.T) {
	btSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/java-archive")
		_, _ = w.Write([]byte("fake-buildtools-content"))
	}))
	defer btSrv.Close()

	r := NewResolver()
	r.BuildToolsURL = btSrv.URL

	dir := t.TempDir()
	err := r.DownloadServerJar(context.Background(), TypeSpigot, "26.3", "", dir)
	if err != nil {
		t.Fatalf("DownloadServerJar spigot failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "installer.jar"))
	if err != nil {
		t.Fatalf("installer.jar not found: %v", err)
	}
	if string(content) != "fake-buildtools-content" {
		t.Fatalf("expected fake-buildtools-content, got %q", string(content))
	}

	// Verify eula.txt was created
	eula, err := os.ReadFile(filepath.Join(dir, "eula.txt"))
	if err != nil || string(eula) != "eula=true\n" {
		t.Fatalf("eula.txt missing or invalid: %v", err)
	}
}

func TestMohistBuildsErrorOnUnreleased(t *testing.T) {
	r := NewResolver()
	dir := t.TempDir()

	// 1.21.4 currently has no builds released upstream
	err := r.DownloadServerJar(context.Background(), TypeMohist, "1.21.4", "latest", dir)
	if err == nil {
		t.Fatalf("expected DownloadServerJar to fail for unreleased mohist version 1.21.4")
	}

	// Validating unreleased version should also fail
	_, err = r.Validate(context.Background(), TypeMohist, "1.21.4", "latest")
	if err == nil {
		t.Fatalf("expected Validate to fail for unreleased mohist version 1.21.4")
	}
}

func TestMohistVersionsExcludesUnreleased(t *testing.T) {
	r := NewResolver()
	versions, err := r.MohistVersions(context.Background())
	if err != nil {
		t.Fatalf("MohistVersions failed: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one valid mohist version")
	}
	for _, v := range versions {
		if v == "1.21.4" || v == "1.20.6" {
			t.Errorf("MohistVersions returned unreleased version with 0 builds: %s", v)
		}
	}
	// The highest released version is 1.20.2
	if versions[0] != "1.20.2" {
		t.Errorf("expected highest Mohist version to be 1.20.2, got %s", versions[0])
	}
}

func TestMohistDownloadContextCanceled(t *testing.T) {
	r := NewResolver()
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	err := r.DownloadServerJar(ctx, TypeMohist, "1.20.2", "174", dir)
	if err == nil {
		t.Fatalf("expected error on canceled context")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
	if strings.Contains(err.Error(), "builds-raw") {
		t.Fatalf("should not attempt or report builds-raw fallback when context is canceled, got: %v", err)
	}
}

func TestYouerVersions(t *testing.T) {
	r := NewResolver()
	versions, err := r.YouerVersions(context.Background())
	if err != nil {
		t.Fatalf("YouerVersions failed: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one valid youer version")
	}
	has1211 := false
	for _, v := range versions {
		if v == "1.21.1" {
			has1211 = true
			break
		}
	}
	if !has1211 {
		t.Errorf("expected versions to include 1.21.1, got: %v", versions)
	}
}

func TestYouerBuildsAndValidation(t *testing.T) {
	r := NewResolver()
	builds, err := r.YouerBuilds(context.Background(), "1.21.1")
	if err != nil {
		t.Fatalf("YouerBuilds failed: %v", err)
	}
	if len(builds) == 0 {
		t.Fatal("expected at least one build for 1.21.1")
	}

	res, err := r.Validate(context.Background(), TypeYouer, "1.21.1", "latest")
	if err != nil {
		t.Fatalf("Validate youer failed: %v", err)
	}
	if res.Type != TypeYouer || res.Version != "1.21.1" || res.Build == "" || res.Build == "latest" {
		t.Fatalf("unexpected validation result: %+v", res)
	}
}

func TestYouerDownloadContextCanceled(t *testing.T) {
	r := NewResolver()
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	err := r.DownloadServerJar(ctx, TypeYouer, "1.21.1", "latest", dir)
	if err == nil {
		t.Fatalf("expected error on canceled context")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}


func TestRecommendJavaVersion(t *testing.T) {
	tests := []struct {
		mcVersion string
		want      int
	}{
		{"25w07a", 25},
		{"26w02a", 25},
		{"26.1", 25},
		{"1.22.0", 25},
		{"1.22", 25},
		{"1.23.1", 25},
		{"1.21.4", 21},
		{"1.21.1", 21},
		{"1.20.5", 21},
		{"1.20.4", 17},
		{"1.18.2", 17},
		{"1.17.1", 17},
		{"1.16.5", 8},
		{"1.12.2", 8},
		{"v1.20.1", 17},
		{"1.21.1-fabric", 21},
		{" 1.16.5 ", 8},
		{"2.11.3", 21}, // Geyser standalone
		{"2.4.2", 21},  // Geyser older
		{"3.4.0-SNAPSHOT", 21}, // Velocity
	}

	for _, tt := range tests {
		got := RecommendJavaVersion(tt.mcVersion)
		if got != tt.want {
			t.Errorf("RecommendJavaVersion(%q) = %d, want %d", tt.mcVersion, got, tt.want)
		}
	}

	if got := RecommendJavaVersionForType("geysermc", "2.11.3"); got != 21 {
		t.Errorf("RecommendJavaVersionForType(geysermc) = %d, want 21", got)
	}
	if got := RecommendJavaVersionForType("velocity", "3.4.0"); got != 21 {
		t.Errorf("RecommendJavaVersionForType(velocity) = %d, want 21", got)
	}
}

func TestCompareVersionTokens(t *testing.T) {
	if CompareVersionTokens("563", "470") <= 0 {
		t.Errorf("expected 563 > 470")
	}
	if CompareVersionTokens("12", "120") >= 0 {
		t.Errorf("expected 12 < 120")
	}
	if CompareVersionTokens("latest", "563") <= 0 {
		t.Errorf("expected latest > 563")
	}
	if CompareVersionTokens("1.21.1", "1.20.4") <= 0 {
		t.Errorf("expected 1.21.1 > 1.20.4")
	}
	if CompareVersionTokens("1.20.4", "1.20.4") != 0 {
		t.Errorf("expected 1.20.4 == 1.20.4")
	}
}

func TestPaperDownloadURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/projects/velocity/versions/3.4.0-SNAPSHOT/builds/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": 563,
			"downloads": {
				"server:default": {
					"name": "velocity-3.4.0-SNAPSHOT-563.jar",
					"url": "https://fill-data.papermc.io/v1/objects/test/velocity-3.4.0-SNAPSHOT-563.jar"
				}
			}
		}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	r := NewResolverWithBases(nil, server.URL, "", "")
	dlURL, err := r.paperDownloadURL(context.Background(), "velocity", "3.4.0-SNAPSHOT", "latest")
	if err != nil {
		t.Fatalf("paperDownloadURL: %v", err)
	}
	want := "https://fill-data.papermc.io/v1/objects/test/velocity-3.4.0-SNAPSHOT-563.jar"
	if dlURL != want {
		t.Errorf("paperDownloadURL = %q, want %q", dlURL, want)
	}
}

func TestLiveLimbo(t *testing.T) {
	r := NewResolver()
	urlLatest, err := r.resolveLimboJar(context.Background(), "latest")
	if err != nil {
		t.Fatalf("resolveLimboJar(latest) failed: %v", err)
	}
	t.Logf("resolveLimboJar(latest) returned: %s", urlLatest)

	url121, err := r.resolveLimboJar(context.Background(), "1.21")
	if err != nil {
		t.Fatalf("resolveLimboJar(1.21) failed: %v", err)
	}
	t.Logf("resolveLimboJar(1.21) returned: %s", url121)

	dir := t.TempDir()
	err = r.DownloadServerJar(context.Background(), TypeLimbo, "latest", "latest", dir)
	if err != nil {
		t.Fatalf("DownloadServerJar failed: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "server.jar"))
	if err != nil {
		t.Fatalf("server.jar not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("downloaded server.jar is empty")
	}
	t.Logf("downloaded Limbo server.jar size: %d bytes", info.Size())
}

func TestLivePurpur(t *testing.T) {
	r := NewResolver()
	for _, v := range []string{"latest", "1.21.1", "1.20.4"} {
		dir := t.TempDir()
		err := r.DownloadServerJar(context.Background(), TypePurpur, v, "latest", dir)
		if err != nil {
			t.Fatalf("DownloadServerJar(%s, latest) failed: %v", v, err)
		}
		info, err := os.Stat(filepath.Join(dir, "server.jar"))
		if err != nil || info.Size() == 0 {
			t.Fatalf("server.jar not found or empty for %s: %v", v, err)
		}
		t.Logf("Purpur %s downloaded successfully (%d bytes)", v, info.Size())
	}
}

func TestPurpurBuilds(t *testing.T) {
	r := NewResolver()
	for _, v := range []string{"", "latest", "1.21.0", "1.21.4"} {
		builds, err := r.PurpurBuilds(context.Background(), v)
		if err != nil {
			t.Fatalf("PurpurBuilds(%q) failed: %v", v, err)
		}
		if len(builds) == 0 {
			t.Fatalf("PurpurBuilds(%q) returned no builds", v)
		}
		t.Logf("PurpurBuilds(%q) returned %d builds (latest: %s)", v, len(builds), builds[len(builds)-1])
	}
}


