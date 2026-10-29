package servers

import (
	"context"
	"testing"
)

func TestSoftwareCatalogDiscoveriesPersistAndTrackCurrentFleet(t *testing.T) {
	s, id := newModTestStore(t, "paper")
	t.Cleanup(func() { s.db.Close() })
	ctx := context.Background()
	if _, err := s.db.Exec(`UPDATE servers SET build='226';
		INSERT INTO servers (id,name,server_type,version,build,ram_mb,host_port,state,created_at,updated_at)
		VALUES ('other-game','other','paper','1.20.1','100',1024,25566,'stopped','',''),
		('other-type','other','folia','1.21.4','100',1024,25567,'stopped','',''),
		('current','current','paper','1.21.4','227',1024,25568,'stopped','','')`); err != nil {
		t.Fatal(err)
	}
	remember := func(kind, version string, values ...string) {
		t.Helper()
		if err := s.rememberSoftwareCatalog(ctx, kind, version, values); err != nil {
			t.Fatal(err)
		}
	}
	remember("paper", "1.21.4", "226", "227")
	// An old, empty, or prerelease-only response must not discard build 227.
	remember("paper", "1.21.4", "226")
	remember("paper", "1.21.4")
	remember("paper", "1.21.4", "228-beta", "latest")
	restarted := &Store{db: s.db}
	status, err := restarted.UpdateCheckerStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.LastChecked != "" || status.NextCheckAt != "" {
		t.Fatalf("discovery enabled or claimed a fleet scan: %+v", status)
	}
	if software := status.Servers[id].Software; software == nil || !software.Available || software.LatestBuild != "227" {
		t.Fatalf("missing build update after restart: %+v", software)
	}
	for _, other := range []string{"other-game", "other-type", "current"} {
		if software := status.Servers[other].Software; software != nil && software.Available {
			t.Errorf("unrelated/current server marked outdated: %s %+v", other, software)
		}
	}
	if _, err := s.db.Exec(`UPDATE servers SET build='227' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	status, err = s.UpdateCheckerStatus(ctx)
	if err != nil || status.Servers[id].Software.Available {
		t.Fatalf("updating the build left a badge: %+v %v", status, err)
	}
	remember("paper", "", "1.21.4", "1.21.5", "1.22-rc1")
	// Learning about builds for the older game version cannot hide a newer game.
	remember("paper", "1.21.4", "229")
	status, err = s.UpdateCheckerStatus(ctx)
	if err != nil || !status.Servers[id].Software.Available || status.Servers[id].Software.LatestVersion != "1.21.5" {
		t.Fatalf("new game version was lost: %+v %v", status, err)
	}
	if _, err := s.db.Exec(`UPDATE servers SET version='1.21.5',build='1' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	status, err = s.UpdateCheckerStatus(ctx)
	if err != nil || status.Servers[id].Software.Available {
		t.Fatalf("builds crossed game versions: %+v %v", status, err)
	}
}

func TestSoftwareCatalogPreservesSavedModsAndUnpinnedBuilds(t *testing.T) {
	s, id := newModTestStore(t, "fabric")
	t.Cleanup(func() { s.db.Close() })
	ctx := context.Background()
	srv, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	mods := &ServerModUpdatesResponse{Updates: map[string]ModUpdateInfo{"shared": {LatestVersion: "2"}}, UpdateCount: 1}
	if err := s.saveUpdateCheck(ctx, srv, ServerUpdateCheck{Mods: mods, CheckedAt: "2026-10-28T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := s.rememberSoftwareCatalog(ctx, "fabric", "1.21.4", []string{"0.16.10"}); err != nil {
		t.Fatal(err)
	}
	status, err := s.UpdateCheckerStatus(ctx)
	if err != nil || status.Servers[id].Software != nil || status.Servers[id].Mods.UpdateCount != 1 {
		t.Fatalf("unpinned build or saved mods: %+v %v", status, err)
	}
	if _, err := s.db.Exec(`UPDATE servers SET build='0.16.9' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	status, err = s.UpdateCheckerStatus(ctx)
	if err != nil || !status.Servers[id].Software.Available || status.Servers[id].Mods != nil {
		t.Fatalf("reconfigured server missed a known build or kept stale mods: %+v %v", status, err)
	}
}
