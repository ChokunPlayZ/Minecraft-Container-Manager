package servers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIsClientOnlyFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected bool
	}{
		{"missing-mods-checker-1.20.1-1.0.jar", true},
		{"MissingModsChecker-Forge-1.20.1.jar", true},
		{"missingmodschecker-1.20.1.jar", true},
		{"oculus-mc1.20.1-1.6.4.jar", true},
		{"rubidium-mc1.20.1-0.7.1.jar", true},
		{"embeddium-0.3.31+mc1.20.1.jar", true},
		{"sodium-fabric-0.5.8+mc1.20.4.jar", true},
		{"iris-1.6.17+1.20.1.jar", true},
		{"fancymenu_forge_3.1.2_MC_1.20.1.jar", true},
		{"drippyloadingscreen_forge_3.0.0.jar", true},
		{"controlling-forge-1.20.1-12.0.2.jar", true},
		{"soundphysics-forge-1.20.1.jar", true},
		{"presence-footsteps-1.20.1.jar", true},
		{"entityculling-forge-1.6.2-mc1.20.1.jar", true},
		// Server safe mods MUST NOT be falsely flagged
		{"jei-1.20.1-forge-15.3.0.4.jar", false},
		{"create-1.20.1-0.5.1.f.jar", false},
		{"waystones-forge-1.20-14.1.3.jar", false},
		{"cloth-config-11.1.118-forge.jar", false},
		{"ferritecore-6.0.1-forge.jar", false},
		{"curios-forge-5.8.1+1.20.1.jar", false},
		{"appleskin-forge-mc1.20.1-2.5.1.jar", false},
		{"geckolib-forge-1.20.1-4.4.4.jar", false},
		{"architectury-9.2.14-forge.jar", false},
	}

	for _, tc := range tests {
		got, reason := IsClientOnlyFilename(tc.filename)
		if got != tc.expected {
			t.Errorf("IsClientOnlyFilename(%q) = %v (reason: %q), want %v", tc.filename, got, reason, tc.expected)
		}
	}
}

func TestIsClientOnlyModID(t *testing.T) {
	tests := []struct {
		modID    string
		expected bool
	}{
		{"missingmodschecker", true},
		{"oculus", true},
		{"rubidium", true},
		{"embeddium", true},
		{"sodium", true},
		{"fancymenu", true},
		{"controlling", true},
		{"create", false},
		{"jei", false},
		{"waystones", false},
		{"curios", false},
	}

	for _, tc := range tests {
		got, _ := IsClientOnlyModID(tc.modID)
		if got != tc.expected {
			t.Errorf("IsClientOnlyModID(%q) = %v, want %v", tc.modID, got, tc.expected)
		}
	}
}

func TestInspectModJarTier2Environment(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Fabric client-only mod
	fabricClientJar := createTestJar(t, tempDir, "custom-hud-1.0.jar", map[string]string{
		"fabric.mod.json": `{
			"id": "customhud",
			"name": "Custom HUD",
			"version": "1.0.0",
			"environment": "client"
		}`,
	})
	m1, err := inspectModJar(fabricClientJar)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}
	if !m1.IsClientOnly {
		t.Errorf("expected fabric client mod to be flagged as client-only")
	}

	// 2. Fabric common (safe) mod
	fabricCommonJar := createTestJar(t, tempDir, "custom-lib-1.0.jar", map[string]string{
		"fabric.mod.json": `{
			"id": "customlib",
			"name": "Custom Lib",
			"version": "1.0.0",
			"environment": "*"
		}`,
	})
	m2, err := inspectModJar(fabricCommonJar)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}
	if m2.IsClientOnly {
		t.Errorf("expected fabric common mod to NOT be flagged as client-only")
	}

	// 3. NeoForge client mod
	neoforgeClientJar := createTestJar(t, tempDir, "neoforge-client-1.0.jar", map[string]string{
		"META-INF/neoforge.mods.toml": `
modId="neoclient"
displayName="NeoForge Client Tool"
version="1.0"
dist="CLIENT"
`,
	})
	m3, err := inspectModJar(neoforgeClientJar)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}
	if !m3.IsClientOnly {
		t.Errorf("expected neoforge client mod to be flagged as client-only")
	}

	// 4. Forge client-only flag
	forgeClientJar := createTestJar(t, tempDir, "forge-client-1.0.jar", map[string]string{
		"META-INF/mods.toml": `
modId="forgeclient"
displayName="Forge Client Tool"
version="1.0"
clientSideOnly=true
`,
	})
	m4, err := inspectModJar(forgeClientJar)
	if err != nil {
		t.Fatalf("inspectModJar failed: %v", err)
	}
	if !m4.IsClientOnly {
		t.Errorf("expected forge clientSideOnly mod to be flagged as client-only")
	}
}

func TestSanitizeClientMods(t *testing.T) {
	tempDir := t.TempDir()
	store := &Store{dataDir: tempDir}
	serverID := "srv-sanitize"
	modsDir := filepath.Join(tempDir, "servers", serverID, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	// Create a known client-only mod (Tier 1)
	clientMod := createTestJar(t, modsDir, "missing-mods-checker-1.20.1.jar", map[string]string{"dummy.txt": "abc"})

	// Create a Tier 2 fabric client-only mod
	fabricClientMod := createTestJar(t, modsDir, "client-hud-2.0.jar", map[string]string{
		"fabric.mod.json": `{"id":"clienthud","environment":"client"}`,
	})

	// Create a safe mod
	safeMod := createTestJar(t, modsDir, "waystones-1.20.1.jar", map[string]string{
		"fabric.mod.json": `{"id":"waystones","environment":"*"}`,
	})

	quarantined, err := store.SanitizeClientMods(context.Background(), serverID)
	if err != nil {
		t.Fatalf("SanitizeClientMods failed: %v", err)
	}

	if len(quarantined) != 2 {
		t.Fatalf("expected 2 quarantined mods, got %d: %v", len(quarantined), quarantined)
	}

	// Verify that client mods were renamed to .disabled
	if _, err := os.Stat(clientMod + ".disabled"); os.IsNotExist(err) {
		t.Errorf("expected missing-mods-checker to be renamed to .disabled")
	}
	if _, err := os.Stat(clientMod); !os.IsNotExist(err) {
		t.Errorf("expected original missing-mods-checker jar to no longer exist")
	}

	if _, err := os.Stat(fabricClientMod + ".disabled"); os.IsNotExist(err) {
		t.Errorf("expected fabric client mod to be renamed to .disabled")
	}

	// Verify safe mod remains untouched
	if _, err := os.Stat(safeMod); os.IsNotExist(err) {
		t.Errorf("expected safe waystones mod to remain enabled")
	}
}
