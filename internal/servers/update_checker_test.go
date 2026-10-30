package servers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/checkcache"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/proxy"
)

func testJSONResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSoftwareUpdatesIgnoreNonReleaseVersions(t *testing.T) {
	for _, tc := range []struct {
		name, installed, candidate, build, wantVersion, wantBuild string
		available                                                 bool
	}{
		{name: "current release", installed: "1.21.4", wantVersion: "1.21.4"},
		{name: "new release", installed: "1.21.4", candidate: "1.21.5", wantVersion: "1.21.5", available: true},
		{name: "year numbered release", installed: "1.21.4", candidate: "26.1.1", wantVersion: "26.1.1", available: true},
		{name: "snapshot candidate", installed: "1.21.4", candidate: "26.1-snapshot-1", wantVersion: "1.21.4"},
		{name: "loader update", installed: "1.21.4", build: "0.16.9", wantVersion: "1.21.4", wantBuild: "0.16.10", available: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
				if req.URL.Path == "/versions/game" {
					versions := []string{"1.21.4", "3D Shareware v1.34", "1.RV-Pre1", "20w14infinite", "23w13a_or_b", "24w14potato", "25w14craftmine", "26w14a", "b1.8.1", "rd-132211", "1.22-pre1", "1.22-rc1", "latest", "custom"}
					if tc.candidate != "" {
						versions = append(versions, tc.candidate)
					}
					games := make([]map[string]string, 0, len(versions))
					for _, v := range versions {
						games = append(games, map[string]string{"version": v})
					}
					body, err := json.Marshal(games)
					if err != nil {
						t.Fatal(err)
					}
					return testJSONResponse(string(body))
				}
				if req.URL.Path == "/versions/loader/1.21.4" {
					return testJSONResponse(`[{"loader":{"version":"0.16.9"}},{"loader":{"version":"0.16.10"}},{"loader":{"version":"0.17.0-beta.1"}}]`)
				}
				t.Errorf("unexpected upstream request: %s", req.URL)
				return testJSONResponse(`[]`)
			})}
			s := &Store{jars: jars.NewResolverWithBases(client, "", "https://catalog.test", "")}
			software, err := s.checkSoftwareUpdate(context.Background(), Server{ServerType: "fabric", Version: tc.installed, Build: tc.build})
			if err != nil {
				t.Fatal(err)
			}
			if !software.Supported || software.Available != tc.available || software.LatestVersion != tc.wantVersion || software.LatestBuild != tc.wantBuild {
				t.Fatalf("unexpected software update: %+v", software)
			}
		})
	}
}

func TestNightlyUpdateChecksOptInPersistenceAndSharedResponses(t *testing.T) {
	s, first := newModTestStore(t, "paper")
	t.Cleanup(func() { s.db.Close() })
	ctx := context.Background()
	second := "second-server"
	_, err := s.db.Exec(`UPDATE servers SET build='100'; INSERT INTO servers (id,name,server_type,version,build,ram_mb,host_port,state,created_at,updated_at) VALUES (?, 'second', 'paper', '1.21.4', '100', 1024, 25566, 'stopped', '', '')`, second)
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	fail := false
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		calls[req.URL.Path]++
		if fail {
			res := testJSONResponse(`{"error":"unavailable"}`)
			res.StatusCode = 503
			return res
		}
		switch req.URL.Path {
		case "/v3/projects/paper":
			return testJSONResponse(`{"versions":{"1.21":["1.21.4"],"1.22":["1.22.0-rc1"]}}`)
		case "/v3/projects/paper/versions/1.21.4/builds":
			return testJSONResponse(`[{"id":100,"channel":"STABLE"},{"id":120,"channel":"STABLE"}]`)
		case "/api/v1/projects/shared/versions":
			return testJSONResponse(`{"result":[{"name":"2.0","downloads":{"PAPER":{"fileInfo":{"name":"shared-2.jar"},"downloadUrl":"https://example.test/shared-2.jar"}}}]}`)
		default:
			t.Errorf("unexpected upstream request: %s", req.URL)
			return testJSONResponse(`{}`)
		}
	})}
	s.jars = jars.NewResolverWithBases(client, "https://fill.papermc.io/v3", "", "")
	px := proxy.NewService()
	px.SetClient(client)
	s.SetProxy(px)
	for _, id := range []string{first, second} {
		dir := filepath.Join(s.dataPath(id), "plugins")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		createTestJar(t, dir, "shared.jar", map[string]string{"plugin.yml": "name: Shared\nversion: 1.0\n"})
		writeModMeta(dir, "shared.jar", ModDownloadMeta{Provider: "hangar", ProjectSlug: "shared"})
	}
	status, err := s.UpdateCheckerStatus(ctx)
	if err != nil || status.Enabled || len(status.Servers) != 0 {
		t.Fatalf("default status: %+v %v", status, err)
	}
	if err := s.CheckUpdates(ctx); !errors.Is(err, ErrUpdateCheckerDisabled) {
		t.Fatalf("disabled: %v", err)
	}
	if len(calls) != 0 {
		t.Fatal("opted-out scan contacted upstreams")
	}
	if err := s.SetUpdateChecksEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	status, err = s.UpdateCheckerStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first, second} {
		result := status.Servers[id]
		if result.Software == nil || !result.Software.Available || result.Software.LatestBuild != "120" || result.Mods == nil || result.Mods.UpdateCount != 1 || result.Error != "" {
			t.Fatalf("saved result: %+v", result)
		}
	}
	for path, count := range calls {
		if count != 1 {
			t.Errorf("%s called %d times in one pass", path, count)
		}
	}
	if status.Running || status.LastChecked == "" || status.NextCheckAt == "" {
		t.Fatalf("status: %+v", status)
	}
	// A new Store simulates process restart without an in-memory result cache.
	restarted := &Store{db: s.db}
	status, err = restarted.UpdateCheckerStatus(ctx)
	if err != nil || len(status.Servers) != 2 {
		t.Fatalf("restart: %+v %v", status, err)
	}
	cached, err := restarted.CachedModUpdates(ctx, first)
	if err != nil || cached.UpdateCount != 1 {
		t.Fatalf("cached mods: %+v %v", cached, err)
	}
	// Each new pass refreshes upstreams, and failures retain known badges.
	fail = true
	if err := s.CheckUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	status, _ = s.UpdateCheckerStatus(ctx)
	if status.Servers[first].Error == "" || !status.Servers[first].Software.Available || status.Servers[first].Mods.UpdateCount != 1 {
		t.Fatal("failed scan discarded known results")
	}
	if calls["/api/v1/projects/shared/versions"] != 2 {
		t.Fatal("new pass reused the previous pass response")
	}
	s.InvalidateModUpdatesCache(first)
	status, _ = s.UpdateCheckerStatus(ctx)
	if status.Servers[first].Mods != nil {
		t.Fatal("artifact change left stale mod badges")
	}
	_, _ = s.db.Exec(`UPDATE servers SET version='1.22' WHERE id=?`, second)
	status, _ = s.UpdateCheckerStatus(ctx)
	if _, ok := status.Servers[second]; ok {
		t.Fatal("configuration change left stale badges")
	}
	if err := s.SetUpdateChecksEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	status, _ = s.UpdateCheckerStatus(ctx)
	if !status.Servers[first].Software.Available || status.NextCheckAt != "" {
		t.Fatal("opt-out discarded known updates or left a scheduled scan")
	}
}

func TestUpdatePassReusesOverlappingHashesAndSeparatesCompatibility(t *testing.T) {
	px := proxy.NewService()
	var requests [][]string
	px.SetClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		var payload struct {
			Hashes []string `json:"hashes"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, payload.Hashes)
		return testJSONResponse(`{"shared":{"id":"new","version_number":"2"}}`)
	})})
	ctx := checkcache.NewContext(context.Background())
	check := func(hashes []string, version string, loaders []string) {
		t.Helper()
		_, err := fetchModrinthHashUpdates(ctx, px, map[string]any{"hashes": hashes, "algorithm": "sha1", "game_versions": []string{version}, "loaders": loaders}, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	check([]string{"shared", "first-only"}, "1.21.4", []string{"fabric"})
	check([]string{"shared", "second-only"}, "1.21.4", []string{"fabric"})
	check([]string{"shared", "first-only"}, "1.21.4", []string{"fabric"})
	if len(requests) != 2 || len(requests[1]) != 1 || requests[1][0] != "second-only" {
		t.Fatalf("overlap was refetched: %v", requests)
	}
	check([]string{"shared"}, "1.20.1", []string{"fabric"})
	check([]string{"shared"}, "1.21.4", []string{"forge"})
	if len(requests) != 4 {
		t.Fatal("different compatibility shared a cached response")
	}
}

func TestNextUpdateCheckUsesPanelMidnightIncludingDST(t *testing.T) {
	for _, tc := range []struct{ zone, now, want string }{
		{"Asia/Bangkok", "2026-10-28T16:59:59Z", "2026-10-28T17:00:00Z"},
		{"Asia/Bangkok", "2026-10-28T17:00:00Z", "2026-10-29T17:00:00Z"},
		{"America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"UTC", "2026-10-28T23:59:59Z", "2026-10-29T00:00:00Z"},
	} {
		t.Setenv("TZ", tc.zone)
		now, _ := time.Parse(time.RFC3339, tc.now)
		if got := nextUpdateCheck(now).UTC().Format(time.RFC3339); got != tc.want {
			t.Errorf("%s got %s want %s", tc.zone, got, tc.want)
		}
	}
}
