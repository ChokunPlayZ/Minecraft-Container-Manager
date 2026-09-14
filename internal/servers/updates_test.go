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
