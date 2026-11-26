package servers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/mcm-panel/mcm/internal/proxy"
)

func TestModrinthUpdatesUseTheInstalledChannel(t *testing.T) {
	ctx := context.Background()
	st, id := newModTestStore(t, "fabric")
	old, err := st.UploadMod(ctx, id, "example-beta.jar", bytes.NewReader(fixtureJar(t)), ModDownloadMeta{
		Provider: "modrinth", ProjectID: "example", ReleaseType: "beta",
	})
	if err != nil {
		t.Fatal(err)
	}

	var requests [][]string
	px := proxy.NewService()
	px.SetClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		var payload struct {
			VersionTypes []string `json:"version_types"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Errorf("decode Modrinth update request: %v", err)
		}
		requests = append(requests, payload.VersionTypes)
		response, _ := json.Marshal(map[string]modrinthVersion{old.SHA1: {
			ID: "beta-2", ProjectID: "example", VersionNumber: "2.0-beta", VersionType: "beta",
			Files: []modrinthVersionFile{{Filename: "example-beta-2.jar", URL: "https://cdn.modrinth.com/example-beta-2.jar"}},
		}})
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(response))}
	})})
	st.SetProxy(px)

	current, err := st.CheckModUpdates(ctx, id, true)
	if err != nil || current.Updates[old.Name].LatestReleaseType != "beta" {
		t.Fatalf("current-version update: %+v, %v", current, err)
	}
	target, err := st.CheckTargetVersionCompatibility(ctx, id, "1.21.4", true)
	if err != nil || target.Mods[old.Name].ReleaseType != "beta" {
		t.Fatalf("target-version update: %+v, %v", target, err)
	}
	if len(requests) != 2 {
		t.Fatalf("expected two Modrinth update requests, got %v", requests)
	}
	for _, versionTypes := range requests {
		if len(versionTypes) != 1 || versionTypes[0] != "beta" {
			t.Fatalf("unexpected channel filter: %v", versionTypes)
		}
	}
}

func TestCatalogReplacementRequiresAnExplicitChannelUpgrade(t *testing.T) {
	ctx := context.Background()
	st, id := newModTestStore(t, "fabric")
	jar := fixtureJar(t)
	px := proxy.NewService()
	px.SetClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jar))}
	})})
	st.SetProxy(px)

	old, err := st.UploadMod(ctx, id, "example-beta.jar", bytes.NewReader(jar), ModDownloadMeta{
		Provider: "modrinth", ProjectID: "example", ReleaseType: "beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	old, err = st.SetModEnabled(ctx, id, old.File, false)
	if err != nil {
		t.Fatal(err)
	}
	replace := func(channel string, allowSwitch bool) (Mod, error) {
		return st.ReplaceModFromURL(ctx, id, "replacement.jar", "https://cdn.modrinth.com/replacement.jar", old.File, allowSwitch,
			ModDownloadMeta{Provider: "modrinth", ProjectID: "example", ReleaseType: channel, CatalogVersionID: "replacement"})
	}
	if _, err := replace("release", false); !errors.Is(err, ErrModChannel) {
		t.Fatalf("unconfirmed switch: got %v", err)
	}
	if _, err := replace("alpha", true); !errors.Is(err, ErrModChannel) {
		t.Fatalf("switch to a less stable channel: got %v", err)
	}
	got, err := replace("release", true)
	if err != nil || got.ReleaseType != "release" || got.Enabled || got.File != "replacement.jar.disabled" {
		t.Fatalf("explicit replacement: %+v, %v", got, err)
	}
	list, err := st.ListMods(ctx, id)
	if err != nil || len(list.Items) != 1 || list.Items[0].ReleaseType != "release" || list.Items[0].CatalogVersionID != "replacement" {
		t.Fatalf("persisted replacement: %+v, %v", list, err)
	}
}

func TestSpigetDownloadFollowsTheVersionRedirect(t *testing.T) {
	ctx := context.Background()
	st, id := newModTestStore(t, "paper")
	jar := fixtureJar(t)
	px := proxy.NewService()
	const path = "/v2/resources/12/versions/34/download"
	const redirect = "https://www.spigotmc.org/resources/12/download?version=34"

	var downloads []string
	px.SetClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: roundTripFunc(func(req *http.Request) *http.Response {
			downloads = append(downloads, req.URL.String())
			switch req.URL.Host {
			case "api.spiget.org":
				return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {redirect}}, Body: http.NoBody}
			case "www.spigotmc.org":
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(jar))}
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}
			}
		}),
	})
	st.SetProxy(px)

	_, err := st.DownloadMod(ctx, id, "example.jar", "https://api.spiget.org"+path, ModDownloadMeta{
		Provider: "spiget", ProjectID: "12", CatalogVersionID: "34", ReleaseType: "unclassified",
	})
	if err != nil || len(downloads) != 2 || downloads[1] != redirect {
		t.Fatalf("version download: %v, %v", downloads, err)
	}
}
