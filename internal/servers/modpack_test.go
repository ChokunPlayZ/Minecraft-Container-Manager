package servers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func createTestZip(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip file %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write zip file %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func TestInspectModrinthModpack(t *testing.T) {
	idx := modrinthIndex{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     "1.2.3",
		Name:          "Test Fabric Pack",
		Summary:       "A test pack for Fabric",
		Dependencies: map[string]string{
			"minecraft":     "1.20.1",
			"fabric-loader": "0.15.11",
		},
		Files: []modrinthIndexFile{
			{
				Path:      "mods/server-and-client.jar",
				Downloads: []string{"https://example.com/mod1.jar"},
				Env: struct {
					Client string `json:"client"`
					Server string `json:"server"`
				}{Client: "required", Server: "required"},
			},
			{
				Path:      "mods/client-only.jar",
				Downloads: []string{"https://example.com/mod2.jar"},
				Env: struct {
					Client string `json:"client"`
					Server string `json:"server"`
				}{Client: "required", Server: "unsupported"},
			},
		},
	}
	idxBytes, _ := json.Marshal(idx)

	zr := createTestZip(t, map[string]string{
		"modrinth.index.json": string(idxBytes),
	})

	manifest, err := InspectModpackArchive(zr, int64(zr.Len()), "pack.mrpack")
	if err != nil {
		t.Fatalf("unexpected error inspecting modpack: %v", err)
	}

	if manifest.Format != ModpackFormatModrinth {
		t.Errorf("expected format %s, got %s", ModpackFormatModrinth, manifest.Format)
	}
	if manifest.Name != "Test Fabric Pack" {
		t.Errorf("expected name 'Test Fabric Pack', got '%s'", manifest.Name)
	}
	if manifest.MinecraftVersion != "1.20.1" {
		t.Errorf("expected mc version 1.20.1, got '%s'", manifest.MinecraftVersion)
	}
	if manifest.Loader != "fabric" {
		t.Errorf("expected loader fabric, got '%s'", manifest.Loader)
	}
	if manifest.LoaderVersion != "0.15.11" {
		t.Errorf("expected loader version 0.15.11, got '%s'", manifest.LoaderVersion)
	}
	if manifest.TotalFiles != 2 {
		t.Errorf("expected 2 total files, got %d", manifest.TotalFiles)
	}
	if manifest.ServerFiles != 1 {
		t.Errorf("expected 1 server file, got %d", manifest.ServerFiles)
	}
	if manifest.ClientOnlyFiles != 1 {
		t.Errorf("expected 1 client-only file, got %d", manifest.ClientOnlyFiles)
	}
}

func TestInspectCurseForgeModpack(t *testing.T) {
	man := curseForgeManifest{
		ManifestType:    "minecraftModpack",
		ManifestVersion: 1,
		Name:            "Test Forge Pack",
		Version:         "2.0",
		Author:          "Modder",
	}
	man.Minecraft.Version = "1.19.2"
	man.Minecraft.ModLoaders = []struct {
		ID      string `json:"id"`
		Primary bool   `json:"primary"`
	}{
		{ID: "forge-43.2.0", Primary: true},
	}
	man.Files = []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	}{
		{ProjectID: 100, FileID: 200, Required: true},
	}
	manBytes, _ := json.Marshal(man)

	zr := createTestZip(t, map[string]string{
		"manifest.json": string(manBytes),
	})

	manifest, err := InspectModpackArchive(zr, int64(zr.Len()), "cursepack.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.Format != ModpackFormatCurseForge {
		t.Errorf("expected format %s, got %s", ModpackFormatCurseForge, manifest.Format)
	}
	if manifest.Name != "Test Forge Pack" {
		t.Errorf("expected name 'Test Forge Pack', got '%s'", manifest.Name)
	}
	if manifest.MinecraftVersion != "1.19.2" {
		t.Errorf("expected mc version 1.19.2, got '%s'", manifest.MinecraftVersion)
	}
	if manifest.Loader != "forge" {
		t.Errorf("expected loader forge, got '%s'", manifest.Loader)
	}
	if manifest.LoaderVersion != "43.2.0" {
		t.Errorf("expected loader version 43.2.0, got '%s'", manifest.LoaderVersion)
	}
}

func TestInspectGenericModpack(t *testing.T) {
	zr := createTestZip(t, map[string]string{
		"mods/mod1.jar": "jarcontent1",
		"mods/mod2.jar": "jarcontent2",
		"config/cfg.txt": "cfgcontent",
	})

	manifest, err := InspectModpackArchive(zr, int64(zr.Len()), "My_Cool_Pack.zip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if manifest.Format != ModpackFormatGeneric {
		t.Errorf("expected format %s, got %s", ModpackFormatGeneric, manifest.Format)
	}
	if manifest.Name != "My Cool Pack" {
		t.Errorf("expected 'My Cool Pack', got '%s'", manifest.Name)
	}
	if manifest.TotalFiles != 2 {
		t.Errorf("expected 2 jar files, got %d", manifest.TotalFiles)
	}
}

func TestInstallModrinthModpackEndToEnd(t *testing.T) {
	// Setup test HTTP server to serve mod download
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download/test-mod.jar" {
			w.Header().Set("Content-Type", "application/java-archive")
			w.Write([]byte("mock jar binary content"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	// Setup temporary store
	store := newTestStore(t, "srv-modpack", StateStopped)
	ctx := context.Background()
	serverID := "srv-modpack"

	// Build .mrpack file
	idx := modrinthIndex{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     "1.0.0",
		Name:          "Fabulously Test Pack",
		Summary:       "E2E Test modpack",
		Dependencies: map[string]string{
			"minecraft":     "1.21.1",
			"fabric-loader": "0.16.5",
		},
		Files: []modrinthIndexFile{
			{
				Path:      "mods/test-mod.jar",
				Downloads: []string{ts.URL + "/download/test-mod.jar"},
				Env: struct {
					Client string `json:"client"`
					Server string `json:"server"`
				}{Client: "required", Server: "required"},
			},
			{
				Path:      "mods/client-mod.jar",
				Downloads: []string{ts.URL + "/download/client-mod.jar"},
				Env: struct {
					Client string `json:"client"`
					Server string `json:"server"`
				}{Client: "required", Server: "unsupported"},
			},
		},
	}
	idxBytes, _ := json.Marshal(idx)

	packPath := filepath.Join(t.TempDir(), "pack.mrpack")
	packZip := createTestZip(t, map[string]string{
		"modrinth.index.json":        string(idxBytes),
		"overrides/config/test.json": "{\"enabled\": true}",
	})
	data, _ := io.ReadAll(packZip)
	if err := os.WriteFile(packPath, data, 0o644); err != nil {
		t.Fatalf("failed to write test pack: %v", err)
	}

	// Install modpack with AutoConfigureServer = true
	installed, err := store.InstallModpack(ctx, serverID, packPath, InstallModpackOpts{
		Source:              "upload",
		AutoConfigureServer: true,
	})
	if err != nil {
		t.Fatalf("failed to install modpack: %v", err)
	}

	if installed.Name != "Fabulously Test Pack" {
		t.Errorf("expected modpack name 'Fabulously Test Pack', got '%s'", installed.Name)
	}

	// Verify server was auto-configured to fabric
	updatedSrv, err := store.Get(ctx, serverID)
	if err != nil {
		t.Fatalf("failed to get updated server: %v", err)
	}
	if updatedSrv.ServerType != "fabric" {
		t.Errorf("expected server type to be auto-configured to fabric, got %s", updatedSrv.ServerType)
	}

	// Verify server mod was downloaded, client mod was skipped
	serverModPath := filepath.Join(store.dataPath(serverID), "mods", "test-mod.jar")
	if _, err := os.Stat(serverModPath); err != nil {
		t.Errorf("expected server mod to exist at %s: %v", serverModPath, err)
	}

	clientModPath := filepath.Join(store.dataPath(serverID), "mods", "client-mod.jar")
	if _, err := os.Stat(clientModPath); err == nil {
		t.Errorf("expected client-only mod to NOT exist at %s", clientModPath)
	}

	// Verify override was extracted
	cfgPath := filepath.Join(store.dataPath(serverID), "config", "test.json")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("expected override file at %s: %v", cfgPath, err)
	}

	// Verify GetInstalledModpack
	gotInstalled, err := store.GetInstalledModpack(ctx, serverID)
	if err != nil {
		t.Fatalf("failed to get installed modpack: %v", err)
	}
	if gotInstalled.Name != installed.Name {
		t.Errorf("expected name %s, got %s", installed.Name, gotInstalled.Name)
	}

	// Test Uninstall
	if err := store.UninstallModpack(ctx, serverID); err != nil {
		t.Fatalf("failed to uninstall modpack: %v", err)
	}
	if _, err := store.GetInstalledModpack(ctx, serverID); !errors.Is(err, ErrModpackNotFound) {
		t.Errorf("expected ErrModpackNotFound after uninstall, got %v", err)
	}
	if _, err := os.Stat(serverModPath); err == nil {
		t.Errorf("expected server mod to be deleted on uninstall")
	}
}

func TestInstallGenericModpack(t *testing.T) {
	store := newTestStore(t, "srv-generic", StateStopped)
	ctx := context.Background()
	serverID := "srv-generic"

	packPath := filepath.Join(t.TempDir(), "Generic_Adventure_Pack.zip")
	packZip := createTestZip(t, map[string]string{
		"mods/adventure-mod.jar":    "dummy jar content",
		"config/adventure.cfg":      "config line",
	})
	data, _ := io.ReadAll(packZip)
	if err := os.WriteFile(packPath, data, 0o644); err != nil {
		t.Fatalf("failed to write test pack: %v", err)
	}

	installed, err := store.InstallModpack(ctx, serverID, packPath, InstallModpackOpts{
		Source: "upload",
	})
	if err != nil {
		t.Fatalf("failed to install generic modpack: %v", err)
	}

	if installed.Format != ModpackFormatGeneric {
		t.Errorf("expected generic format, got %s", installed.Format)
	}
	if installed.Name != "Generic Adventure Pack" {
		t.Errorf("expected name 'Generic Adventure Pack', got '%s'", installed.Name)
	}

	modFile := filepath.Join(store.dataPath(serverID), "mods", "adventure-mod.jar")
	if _, err := os.Stat(modFile); err != nil {
		t.Errorf("expected extracted mod at %s: %v", modFile, err)
	}

	cfgFile := filepath.Join(store.dataPath(serverID), "config", "adventure.cfg")
	if _, err := os.Stat(cfgFile); err != nil {
		t.Errorf("expected extracted config at %s: %v", cfgFile, err)
	}
}
