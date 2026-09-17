package servers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/proxy"
)

func TestCheckModUpdatesConcurrentCoalescing(t *testing.T) {
	st, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	srvDir := filepath.Join(st.dataPath(serverID), "plugins")
	_ = os.MkdirAll(srvDir, 0o755)
	pluginJar := filepath.Join(srvDir, "test-plugin.jar")
	_ = os.WriteFile(pluginJar, []byte("fake content"), 0o644)

	var wg sync.WaitGroup
	errCh := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.CheckModUpdates(ctx, serverID, true)
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent CheckModUpdates error: %v", err)
	}
}

func TestCheckModUpdatesAndCache(t *testing.T) {
	st, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	// 1. Empty server returns empty updates
	res, err := st.CheckModUpdates(ctx, serverID, false)
	if err != nil {
		t.Fatalf("CheckModUpdates empty: %v", err)
	}
	if len(res.Updates) != 0 || res.TotalMods != 0 {
		t.Fatalf("expected 0 updates and 0 mods, got updates=%d total=%d", len(res.Updates), res.TotalMods)
	}

	// 2. Mock upstream Modrinth update endpoint
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Simulate Modrinth version_files/update response
		respData := map[string]modrinthVersion{
			"oldhash123": {
				ID:            "ver_new_99",
				ProjectID:     "proj_luckperms",
				Name:          "LuckPerms v5.4.103",
				VersionNumber: "5.4.103",
				Files: []modrinthVersionFile{
					{
						Filename: "LuckPerms-Bukkit-5.4.103.jar",
						URL:      "https://cdn.modrinth.com/data/proj_luckperms/versions/ver_new_99/LuckPerms-Bukkit-5.4.103.jar",
						Primary:  true,
						Hashes:   map[string]string{"sha1": "newhash999"},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(respData)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	px := proxy.NewService()
	px.AddAllowedHost(u.Hostname())
	px.SetClient(ts.Client())
	st.SetProxy(px)

	// Write a mock installed plugin into the server's plugins directory
	srvDir := filepath.Join(st.dataPath(serverID), "plugins")
	_ = os.MkdirAll(srvDir, 0o755)
	pluginJar := filepath.Join(srvDir, "LuckPerms-Bukkit-5.4.102.jar")
	_ = os.WriteFile(pluginJar, []byte("fake jar content"), 0o644)

	// Set metadata with sha1 and project info
	writeModMeta(srvDir, "LuckPerms-Bukkit-5.4.102.jar", ModDownloadMeta{
		ProjectID:   "proj_luckperms",
		ProjectSlug: "luckperms",
		Provider:    "modrinth",
	})

	// ListMods will read the jar, let's verify ListMods works
	mods, err := st.ListMods(ctx, serverID)
	if err != nil {
		t.Fatalf("ListMods: %v", err)
	}
	if len(mods.Items) != 1 {
		t.Fatalf("expected 1 installed mod, got %d", len(mods.Items))
	}

	// Invalidate cache before check
	st.InvalidateModUpdatesCache(serverID)

	// Check updates
	res2, err := st.CheckModUpdates(ctx, serverID, false)
	if err != nil {
		t.Fatalf("CheckModUpdates: %v", err)
	}

	// Cache verification: calling again without force should return cached response immediately
	res3, err := st.CheckModUpdates(ctx, serverID, false)
	if err != nil {
		t.Fatalf("CheckModUpdates cached: %v", err)
	}
	if res3.LastChecked != res2.LastChecked {
		t.Errorf("expected identical LastChecked from cache")
	}

	// Invalidate cache
	st.InvalidateModUpdatesCache(serverID)
	st.updatesMu.RLock()
	_, inCache := st.updatesCache[serverID]
	st.updatesMu.RUnlock()
	if inCache {
		t.Errorf("expected updatesCache to be empty after invalidation")
	}
}

func TestGetModAvailableJars(t *testing.T) {
	st, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	srvDir := filepath.Join(st.dataPath(serverID), "plugins")
	_ = os.MkdirAll(srvDir, 0o755)
	pluginJar := filepath.Join(srvDir, "LuckPerms-Bukkit-5.4.102.jar")
	_ = os.WriteFile(pluginJar, []byte("fake jar content"), 0o644)

	writeModMeta(srvDir, "LuckPerms-Bukkit-5.4.102.jar", ModDownloadMeta{
		ProjectID:   "proj_luckperms",
		ProjectSlug: "luckperms",
		Provider:    "modrinth",
	})

	vers := []modrinthVersion{
		{
			ID:            "ver_1",
			ProjectID:     "proj_luckperms",
			Name:          "5.4.103",
			VersionNumber: "5.4.103",
			VersionType:   "release",
			Files: []modrinthVersionFile{
				{
					Filename: "LuckPerms-Bukkit-5.4.103.jar",
					URL:      "https://cdn.modrinth.com/data/LuckPerms-Bukkit-5.4.103.jar",
					Primary:  true,
					Size:     1024,
				},
			},
		},
	}
	versBytes, _ := json.Marshal(vers)

	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader(versBytes)),
			}
		}),
	}

	px := proxy.NewService()
	px.SetClient(mockClient)
	st.SetProxy(px)

	jars, err := st.GetModAvailableJars(ctx, serverID, "LuckPerms-Bukkit-5.4.102")
	if err != nil {
		t.Fatalf("GetModAvailableJars: %v", err)
	}
	if len(jars) != 1 {
		t.Fatalf("expected 1 jar, got %d", len(jars))
	}
	if jars[0].Filename != "LuckPerms-Bukkit-5.4.103.jar" {
		t.Errorf("got filename %s, want LuckPerms-Bukkit-5.4.103.jar", jars[0].Filename)
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestModMetadataTrackingAndProviderUpdates(t *testing.T) {
	st, serverID := newModTestStore(t, "paper")
	ctx := context.Background()

	// 1. Upload a mod with metadata
	tmpDir := t.TempDir()
	p1 := createTestJar(t, tmpDir, "Chunky-1.4.10.jar", map[string]string{
		"plugin.yml": "name: Chunky\nversion: 1.4.10\n",
	})
	jarData, err := os.ReadFile(p1)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	uploaded, err := st.UploadMod(ctx, serverID, "Chunky-1.4.10.jar", bytes.NewReader(jarData), ModDownloadMeta{
		Provider:    "hangar",
		ProjectID:   "1234",
		ProjectSlug: "chunky",
	})
	if err != nil {
		t.Fatalf("UploadMod: %v", err)
	}
	if uploaded.Provider != "hangar" || uploaded.ProjectID != "1234" || uploaded.ProjectSlug != "chunky" {
		t.Errorf("uploaded meta mismatch: %+v", uploaded)
	}

	// Verify ListMods retrieves it
	list, err := st.ListMods(ctx, serverID)
	if err != nil {
		t.Fatalf("ListMods: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Provider != "hangar" || list.Items[0].ProjectID != "1234" {
		t.Errorf("ListMods did not return saved meta: %+v", list.Items)
	}

	// Verify GetMod retrieves it
	gotMod, err := st.GetMod(ctx, serverID, "Chunky-1.4.10")
	if err != nil {
		t.Fatalf("GetMod: %v", err)
	}
	if gotMod.Provider != "hangar" || gotMod.ProjectSlug != "chunky" {
		t.Errorf("GetMod did not return saved meta: %+v", gotMod)
	}

	// 2. Upload a replacement jar without passing metadata; should inherit from old metadata
	p2 := createTestJar(t, tmpDir, "Chunky-1.4.11.jar", map[string]string{
		"plugin.yml": "name: Chunky\nversion: 1.4.11\n",
	})
	newJarData, err := os.ReadFile(p2)
	if err != nil {
		t.Fatalf("ReadFile p2: %v", err)
	}
	uploaded2, err := st.UploadMod(ctx, serverID, "Chunky-1.4.11.jar", bytes.NewReader(newJarData))
	if err != nil {
		t.Fatalf("UploadMod replacement: %v", err)
	}
	if uploaded2.Provider != "hangar" || uploaded2.ProjectSlug != "chunky" {
		t.Errorf("UploadMod replacement failed to inherit meta: %+v", uploaded2)
	}

	// 3. Mock Hangar API for CheckModUpdates
	hangarResponse := struct {
		Result []hangarVersion `json:"result"`
	}{
		Result: []hangarVersion{
			{
				Name:      "1.4.12",
				CreatedAt: "2026-09-14T00:00:00Z",
				Downloads: map[string]hangarVersionDownload{
					"PAPER": {
						FileInfo: &struct {
							Name      string `json:"name"`
							SizeBytes int64  `json:"sizeBytes"`
						}{
							Name:      "Chunky-1.4.12.jar",
							SizeBytes: 1024,
						},
						DownloadURL: "https://hangarcdn.papermc.io/plugins/chunky/versions/1.4.12/PAPER/Chunky-1.4.12.jar",
					},
				},
			},
		},
	}
	hBytes, _ := json.Marshal(hangarResponse)

	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			if strings.Contains(r.URL.Path, "/projects/chunky/versions") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader(hBytes)),
				}
			}
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}
		}),
	}
	px := proxy.NewService()
	px.SetClient(mockClient)
	st.SetProxy(px)

	updatesRes, err := st.CheckModUpdates(ctx, serverID, true)
	if err != nil {
		t.Fatalf("CheckModUpdates: %v", err)
	}
	if len(updatesRes.Updates) == 0 {
		t.Fatal("expected update for Chunky, got none")
	}
	u, ok := updatesRes.Updates["Chunky-1.4.11"]
	if !ok {
		t.Fatalf("expected update key Chunky-1.4.11, got %+v", updatesRes.Updates)
	}
	if u.Provider != "hangar" || u.LatestVersion != "1.4.12" || u.LatestJar != "Chunky-1.4.12.jar" {
		t.Errorf("unexpected update info: %+v", u)
	}
}

func TestCheckTargetVersionCompatibility(t *testing.T) {
	st, serverID := newModTestStore(t, "fabric")
	ctx := context.Background()

	srvDir := filepath.Join(st.dataPath(serverID), "mods")
	_ = os.MkdirAll(srvDir, 0o755)

	// Mod 1: Lithium (will have update_available)
	mod1 := filepath.Join(srvDir, "lithium-fabric-0.12.0.jar")
	_ = os.WriteFile(mod1, []byte("lithium old content"), 0o644)
	manifest1 := ModManifest{ModID: "lithium", Title: "Lithium", Version: "0.12.0", SHA1: "lithium_sha1"}
	manifestCacheMu.Lock()
	fi1, _ := os.Stat(mod1)
	manifestCache[mod1] = manifestCacheEntry{modTime: fi1.ModTime(), size: fi1.Size(), manifest: manifest1}
	manifestCacheMu.Unlock()
	writeModMeta(srvDir, "lithium-fabric-0.12.0.jar", ModDownloadMeta{
		ProjectID: "lithium_proj",
		Provider:  "modrinth",
	}, "lithium")

	// Mod 2: Fabric API (already compatible)
	mod2 := filepath.Join(srvDir, "fabric-api-0.90.0.jar")
	_ = os.WriteFile(mod2, []byte("fapi content"), 0o644)
	manifest2 := ModManifest{ModID: "fabric-api", Title: "Fabric API", Version: "0.90.0", SHA1: "fapi_sha1"}
	manifestCacheMu.Lock()
	fi2, _ := os.Stat(mod2)
	manifestCache[mod2] = manifestCacheEntry{modTime: fi2.ModTime(), size: fi2.Size(), manifest: manifest2}
	manifestCacheMu.Unlock()

	// Mod 3: Incompatible Mod (Modrinth project exists, but 0 versions for 1.21.1)
	mod3 := filepath.Join(srvDir, "old-mod-1.0.jar")
	_ = os.WriteFile(mod3, []byte("old mod content"), 0o644)
	manifest3 := ModManifest{ModID: "oldmod", Title: "Old Mod", Version: "1.0", SHA1: "old_sha1"}
	manifestCacheMu.Lock()
	fi3, _ := os.Stat(mod3)
	manifestCache[mod3] = manifestCacheEntry{modTime: fi3.ModTime(), size: fi3.Size(), manifest: manifest3}
	manifestCacheMu.Unlock()
	writeModMeta(srvDir, "old-mod-1.0.jar", ModDownloadMeta{
		ProjectSlug: "oldmod",
		Provider:    "modrinth",
	}, "oldmod")

	// Mod 4: Custom / Private untracked mod
	mod4 := filepath.Join(srvDir, "custom-private-mod.jar")
	_ = os.WriteFile(mod4, []byte("private mod"), 0o644)
	manifest4 := ModManifest{Title: "Private Mod", Version: "1.0"}
	manifestCacheMu.Lock()
	fi4, _ := os.Stat(mod4)
	manifestCache[mod4] = manifestCacheEntry{modTime: fi4.ModTime(), size: fi4.Size(), manifest: manifest4}
	manifestCacheMu.Unlock()

	// Setup mock Modrinth API responses
	modrinthHashResp := map[string]modrinthVersion{
		"lithium_sha1": {
			ID:            "v_lithium_new",
			ProjectID:     "lithium_proj",
			Name:          "Lithium 0.14.0",
			VersionNumber: "0.14.0",
			VersionType:   "release",
			DatePublished: "2024-07-01T12:00:00Z",
			Files: []modrinthVersionFile{
				{
					Filename: "lithium-fabric-0.14.0.jar",
					URL:      "https://cdn.modrinth.com/data/lithium/lithium-fabric-0.14.0.jar",
					Primary:  true,
					Hashes:   map[string]string{"sha1": "lithium_new_sha1"},
				},
			},
		},
		"fapi_sha1": {
			ID:            "v_fapi",
			ProjectID:     "fapi_proj",
			Name:          "Fabric API 0.90.0",
			VersionNumber: "0.90.0",
			VersionType:   "release",
			DatePublished: "2024-06-01T12:00:00Z",
			Files: []modrinthVersionFile{
				{
					Filename: "fabric-api-0.90.0.jar",
					URL:      "https://cdn.modrinth.com/data/fapi/fabric-api-0.90.0.jar",
					Primary:  true,
					Hashes:   map[string]string{"sha1": "fapi_sha1"},
				},
			},
		},
	}
	hashBytes, _ := json.Marshal(modrinthHashResp)

	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			if strings.Contains(r.URL.Path, "/version_files/update") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader(hashBytes)),
				}
			}
			if strings.Contains(r.URL.Path, "/project/oldmod/version") {
				// 0 versions found for 1.21.1
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewReader([]byte("[]"))),
				}
			}
			if strings.Contains(r.URL.Path, "lithium-fabric-0.14.0.jar") {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewReader([]byte("new lithium content"))),
				}
			}
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}
		}),
	}
	px := proxy.NewService()
	px.SetClient(mockClient)
	st.SetProxy(px)

	report, err := st.CheckTargetVersionCompatibility(ctx, serverID, "1.21.1", true)
	if err != nil {
		t.Fatalf("CheckTargetVersionCompatibility failed: %v", err)
	}

	if report.TotalMods != 4 {
		t.Fatalf("expected 4 total mods, got %d", report.TotalMods)
	}
	if report.CompatibleCount != 2 {
		t.Fatalf("expected 2 compatible mods, got %d", report.CompatibleCount)
	}
	if report.UpdateCount != 1 {
		t.Fatalf("expected 1 update available, got %d", report.UpdateCount)
	}
	if report.NotAvailableCount != 1 {
		t.Fatalf("expected 1 not available mod, got %d", report.NotAvailableCount)
	}
	if report.UntrackedCount != 1 {
		t.Fatalf("expected 1 untracked mod, got %d", report.UntrackedCount)
	}
	if report.ReadyToUpdate {
		t.Fatalf("expected ReadyToUpdate to be false due to old-mod being unavailable")
	}

	// Verify Lithium
	m1 := report.Mods["lithium-fabric-0.12.0"]
	if m1.Status != ModCompatUpdateAvailable || m1.CompatibleVersion != "0.14.0" || m1.CompatibleJar != "lithium-fabric-0.14.0.jar" {
		t.Errorf("unexpected Lithium report: %+v", m1)
	}

	// Verify Fabric API
	m2 := report.Mods["fabric-api-0.90.0"]
	if m2.Status != ModCompatAlreadyCompatible {
		t.Errorf("unexpected Fabric API report: %+v", m2)
	}

	// Verify Old Mod
	m3 := report.Mods["old-mod-1.0"]
	if m3.Status != ModCompatNotAvailable {
		t.Errorf("unexpected Old Mod report: %+v", m3)
	}

	// Verify Private Mod
	m4 := report.Mods["custom-private-mod"]
	if m4.Status != ModCompatUntracked {
		t.Errorf("unexpected Private Mod report: %+v", m4)
	}

	// Test BatchUpdateTargetMods
	batchRes, err := st.BatchUpdateTargetMods(ctx, serverID, "1.21.1", []string{"lithium-fabric-0.12.0"})
	if err != nil {
		t.Fatalf("BatchUpdateTargetMods failed: %v", err)
	}
	if batchRes.Updated != 1 || batchRes.Failed != 0 {
		t.Fatalf("expected 1 updated, 0 failed, got: %+v", batchRes)
	}

	// Check that old jar was deleted and new jar exists
	if _, err := os.Stat(mod1); !os.IsNotExist(err) {
		t.Errorf("expected old lithium jar to be removed")
	}
	newLithiumPath := filepath.Join(srvDir, "lithium-fabric-0.14.0.jar")
	if _, err := os.Stat(newLithiumPath); err != nil {
		t.Errorf("expected new lithium jar to exist, err: %v", err)
	}
}

