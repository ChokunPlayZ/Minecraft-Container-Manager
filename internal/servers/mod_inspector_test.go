package servers

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func createTestJar(t *testing.T, dir, filename string, files map[string]string) string {
	t.Helper()
	jarPath := filepath.Join(dir, filename)
	f, err := os.Create(jarPath)
	if err != nil {
		t.Fatalf("failed to create jar file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to add file %s to jar: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write content to %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	return jarPath
}

func TestInspectModJarFabric(t *testing.T) {
	tempDir := t.TempDir()
	fabricContent := `{
		"id": "universal-graves",
		"name": "Universal Graves",
		"version": "3.12.0+26.2",
		"description": "Customisable grave mod for Fabric!"
	}`

	jarPath := createTestJar(t, tempDir, "graves-3.12.0+26.2.jar", map[string]string{
		"fabric.mod.json": fabricContent,
	})

	manifest, err := inspectModJar(jarPath)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}

	if manifest.ModID != "universal-graves" {
		t.Errorf("expected ModID 'universal-graves', got %q", manifest.ModID)
	}
	if manifest.Title != "Universal Graves" {
		t.Errorf("expected Title 'Universal Graves', got %q", manifest.Title)
	}
	if manifest.Version != "3.12.0+26.2" {
		t.Errorf("expected Version '3.12.0+26.2', got %q", manifest.Version)
	}
	if manifest.SHA1 == "" {
		t.Errorf("expected non-empty SHA1 hash")
	}
}

func TestInspectModJarPaperPlugin(t *testing.T) {
	tempDir := t.TempDir()
	ymlContent := `
name: EssentialsX
version: 2.20.1
description: The modern essential suite for Paper/Spigot.
main: com.earth2me.essentials.Essentials
`
	jarPath := createTestJar(t, tempDir, "EssentialsX-2.20.1.jar", map[string]string{
		"plugin.yml": ymlContent,
	})

	manifest, err := inspectModJar(jarPath)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}

	if manifest.ModID != "EssentialsX" {
		t.Errorf("expected ModID 'EssentialsX', got %q", manifest.ModID)
	}
	if manifest.Title != "EssentialsX" {
		t.Errorf("expected Title 'EssentialsX', got %q", manifest.Title)
	}
	if manifest.Version != "2.20.1" {
		t.Errorf("expected Version '2.20.1', got %q", manifest.Version)
	}
}

func TestInspectModJarForge(t *testing.T) {
	tempDir := t.TempDir()
	tomlContent := `
modLoader="javafml"
loaderVersion="[47,)"

[[mods]]
modId="ironchests"
version="14.4.4"
displayName="Iron Chests"
description='Upgrades for chests'
`
	jarPath := createTestJar(t, tempDir, "ironchest-1.20.1-14.4.4.jar", map[string]string{
		"META-INF/mods.toml": tomlContent,
	})

	manifest, err := inspectModJar(jarPath)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}

	if manifest.ModID != "ironchests" {
		t.Errorf("expected ModID 'ironchests', got %q", manifest.ModID)
	}
	if manifest.Title != "Iron Chests" {
		t.Errorf("expected Title 'Iron Chests', got %q", manifest.Title)
	}
	if manifest.Version != "14.4.4" {
		t.Errorf("expected Version '14.4.4', got %q", manifest.Version)
	}
}
