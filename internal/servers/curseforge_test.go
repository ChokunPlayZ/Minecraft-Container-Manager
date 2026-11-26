package servers

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/curseforge"
	"github.com/mcm-panel/mcm/internal/proxy"
)

func TestCurseForgeServerOperationsUseSharedKey(t *testing.T) {
	t.Setenv("CURSEFORGE_API_KEY", "different-environment-key")
	st, serverID := newModTestStore(t, "fabric")
	ctx := context.Background()
	if _, err := curseforge.Save(ctx, st.db, "shared-database-key"); err != nil {
		t.Fatal(err)
	}
	modDir := filepath.Join(st.dataPath(serverID), "mods")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "example-old.jar"), []byte("test jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeModMeta(modDir, "example-old.jar", ModDownloadMeta{Provider: "curseforge", ProjectID: "123"})
	px := proxy.NewService()
	calls := 0
	px.SetClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		calls++
		if req.URL.Host != "api.curseforge.com" || req.Header.Get("x-api-key") != "shared-database-key" {
			t.Errorf("request did not use the shared credential: %s %v", req.URL, req.Header)
		}
		body := `{"data":[{"id":456,"fileName":"example-new.jar","displayName":"Example 2","downloadUrl":"https://edge.forgecdn.net/example-new.jar"}]}`
		if req.URL.Path == "/v1/mods/123/files/456" {
			body = `{"data":{"id":456,"fileName":"example-new.jar","downloadUrl":"https://edge.forgecdn.net/example-new.jar"}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	})})
	st.SetProxy(px)
	// Background scans use a plain context, with no request or browser credential.
	updates, err := st.CheckModUpdates(ctx, serverID, true)
	if err != nil || updates.UpdateCount != 1 || len(updates.Errors) != 0 {
		t.Fatalf("updates: %+v %v", updates, err)
	}
	versions, err := st.GetModAvailableJars(ctx, serverID, "example-old.jar")
	if err != nil || len(versions) != 1 {
		t.Fatalf("available versions: %+v %v", versions, err)
	}
	compatibility, err := st.CheckTargetVersionCompatibility(ctx, serverID, "1.21.4", true)
	if err != nil || compatibility.UpdateCount != 1 {
		t.Fatalf("compatibility: %+v %v", compatibility, err)
	}
	key, err := curseforge.APIKey(ctx, st.db)
	if err != nil {
		t.Fatal(err)
	}
	url, file, _, err := resolveCurseForgeDownload(ctx, px, 123, 456, key)
	if err != nil || url == "" || file != "example-new.jar" {
		t.Fatalf("modpack file: %s %s %v", url, file, err)
	}
	// The version picker reuses the current-version catalog response from the scan.
	if calls != 3 {
		t.Fatalf("expected three server-side catalog requests, got %d", calls)
	}
}
