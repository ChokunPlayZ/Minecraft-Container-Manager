package jars

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCatalogDiscoveriesAcrossSoftware(t *testing.T) {
	for _, tc := range []struct {
		kind             JarType
		versions, builds string
	}{
		{TypePaper, `{"versions":{"1.21":["1.21.4"]}}`, `[{"id":226},{"id":227}]`},
		{TypeFolia, `{"versions":{"1.21":["1.21.4"]}}`, `[{"id":226},{"id":227}]`},
		{TypeWaterfall, `{"versions":{"1.21":["1.21.4"]}}`, `[{"id":226},{"id":227}]`},
		{TypeVelocity, `{"versions":{"1.21":["1.21.4"]}}`, `[{"id":226},{"id":227}]`},
		{TypeFabric, `[{"version":"1.21.4"}]`, `[{"loader":{"version":"0.16.10"}}]`},
		{TypeQuilt, `[{"version":"1.21.4"}]`, `[{"loader":{"version":"0.28.0"}}]`},
		{TypeVanilla, `{"versions":[{"id":"1.21.4","type":"release"},{"id":"26w01a","type":"snapshot"}]}`, ""},
		{TypeForge, `{"1.21.4":["1.21.4-54.0.1"]}`, `{"1.21.4":["1.21.4-54.0.1"]}`},
		{TypeNeoForge, `{"versions":["1.21.4-21.4.1"]}`, `{"versions":["1.21.4-21.4.1"]}`},
		{TypeSpigot, `{"1.21.4":{}}`, ""},
		{TypePurpur, `{"versions":["1.21.4"]}`, `{"builds":{"all":["226","227"]}}`},
		{TypeMohist, `{"versions":["1.21.4"]}`, `[{"id":227}]`},
		{TypeYouer, `[{"name":"1.21.4"}]`, `[{"id":227}]`},
		{TypeSponge, `{"artifacts":{"build":{"tagValues":{"minecraft":"1.21.4"}}}}`, ""},
		{TypeGeyser, `{"versions":["1.21.4"]}`, `{"builds":[{"build":227}]}`},
		{TypeBungeeCord, "", `{"builds":[{"number":227}]}`},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			buildRequest := false
			resolver := NewResolverWithBases(&http.Client{Transport: jarTestTransport(func(req *http.Request) (*http.Response, error) {
				body := tc.versions
				if buildRequest || strings.HasSuffix(req.URL.Path, "/builds") {
					body = tc.builds
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}, "http://localhost", "http://localhost", "http://localhost/manifest")
			resolver.ForgeBase = "http://localhost"
			resolver.NeoForgeBase = "http://localhost"
			resolver.SpigotBase = "http://localhost"
			seen := map[string][]string{}
			resolver.ObserveCatalog = func(_ context.Context, kind, version string, values []string) {
				if kind != string(tc.kind) {
					t.Errorf("observed %s while querying %s", kind, tc.kind)
				}
				seen[version] = values
			}
			ctx := context.Background()
			if _, err := resolver.CatalogVersions(ctx, string(tc.kind)); err != nil {
				t.Fatal(err)
			}
			if tc.versions != "" && (len(seen[""]) != 1 || seen[""][0] != "1.21.4") {
				t.Fatalf("version discovery: %v", seen)
			}
			buildRequest = true
			version := "1.21.4"
			if tc.kind == TypeBungeeCord {
				version = "latest"
			}
			if _, err := resolver.CatalogBuilds(ctx, string(tc.kind), version); err != nil {
				t.Fatal(err)
			}
			if tc.builds != "" && len(seen[version]) == 0 {
				t.Fatalf("missing build discovery: %v", seen)
			}
		})
	}
}

func TestFallbackCatalogsDoNotBecomeDiscoveries(t *testing.T) {
	resolver := NewResolverWithBases(&http.Client{Transport: jarTestTransport(func(req *http.Request) (*http.Response, error) {
		return nil, ErrUpstream
	})}, "http://localhost", "http://localhost", "http://localhost/manifest")
	resolver.ObserveCatalog = func(_ context.Context, kind, version string, values []string) {
		t.Errorf("fallback catalog treated as discovery: %s %s %v", kind, version, values)
	}
	for _, kind := range []JarType{TypePaper, TypeFabric, TypeVanilla, TypeForge, TypeNeoForge, TypeSpigot, TypePurpur, TypeFolia, TypeWaterfall, TypeVelocity, TypeQuilt, TypeMohist, TypeYouer, TypeSponge, TypeGeyser, TypeBungeeCord} {
		_, _ = resolver.CatalogVersions(context.Background(), string(kind))
		_, _ = resolver.CatalogBuilds(context.Background(), string(kind), "1.21.4")
	}
}

func TestFailedRawCatalogResponsesDoNotBecomeDiscoveries(t *testing.T) {
	for _, kind := range []JarType{TypeNeoForge, TypeSpigot} {
		t.Run(string(kind), func(t *testing.T) {
			resolver := NewResolverWithBases(&http.Client{Transport: jarTestTransport(func(req *http.Request) (*http.Response, error) {
				body := `<metadata><versioning><versions><version>21.4.999</version></versions></versioning></metadata>`
				if kind == TypeSpigot {
					body = `{"1.99.0":{}}`
				}
				return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})}, "https://catalog.test", "", "")
			resolver.NeoForgeBase = "https://catalog.test"
			resolver.SpigotBase = "https://catalog.test"
			resolver.ObserveCatalog = func(_ context.Context, kind, version string, values []string) {
				t.Errorf("failed HTTP response treated as discovery: %s %s %v", kind, version, values)
			}
			_, _ = resolver.CatalogVersions(context.Background(), string(kind))
			_, _ = resolver.CatalogBuilds(context.Background(), string(kind), "1.21.4")
		})
	}
}
