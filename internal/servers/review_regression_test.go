package servers

import (
	"archive/zip"
	"context"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reviewStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	d, e := db.Open(filepath.Join(dir, "mcm.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { d.Close() })
	insertServer(t, d, "review-server", 25565, "", StateStopped)
	if _, e = d.DB.Exec(`UPDATE servers SET server_type='custom' WHERE id='review-server'`); e != nil {
		t.Fatal(e)
	}
	return NewStore(d, nil, fixtureJarResolver(t, jars.NewResolver()), 25565, 25575, dir, "")
}
func TestReviewModToggleUsesProductionMethod(t *testing.T) {
	s := reviewStore(t)
	dir := filepath.Join(s.dataPath("review-server"), "mods")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "foo.jar"), []byte("jar"), 0644)
	m, e := s.SetModEnabled(context.Background(), "review-server", "foo.jar", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, "foo.jar.disabled")); e != nil {
		t.Errorf("disable reported Enabled=%v but enabled file still exists; disabled file: %v", m.Enabled, e)
	}
}
func TestReviewInstallReturns(t *testing.T) {
	s := reviewStore(t)
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	go func() { _, e := s.Install(ctx, "review-server", true); done <- e }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(400 * time.Millisecond):
		t.Error("Install(true) stuck after context expired; same server mutex is acquired again by ensureContainer")
	}
}
func TestReviewFileSymlinkContainment(t *testing.T) {
	s := reviewStore(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside server"), 0644)
	os.MkdirAll(s.dataPath("review-server"), 0755)
	os.Symlink(outside, filepath.Join(s.dataPath("review-server"), "link"))
	got, e := s.ReadFileContent("review-server", "link/secret.txt")
	if e == nil {
		t.Errorf("read outside server directory: %q", got)
	}
	_, e = s.WriteFileContent("review-server", "link/secret.txt", "overwritten")
	if e == nil {
		t.Error("write outside server directory accepted")
	}
}
func TestReviewModpackUpgradeRemovesOldFiles(t *testing.T) {
	s := reviewStore(t)
	archive := filepath.Join(t.TempDir(), "pack.zip")
	write := func(name string) {
		f, e := os.Create(archive)
		if e != nil {
			t.Fatal(e)
		}
		z := zip.NewWriter(f)
		w, e := z.Create("mods/" + name)
		if e != nil {
			t.Fatal(e)
		}
		w.Write(fixtureJar(t))
		z.Close()
		f.Close()
	}
	write("example-1.jar")
	if _, e := s.InstallModpack(context.Background(), "review-server", archive, InstallModpackOpts{Source: "upload"}); e != nil {
		t.Fatal(e)
	}
	write("example-2.jar")
	if _, e := s.InstallModpack(context.Background(), "review-server", archive, InstallModpackOpts{Source: "upload"}); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(s.dataPath("review-server"), "mods/example-1.jar")); e == nil {
		t.Error("old mod remains enabled alongside replacement; new manifest no longer tracks old jar")
	}
}
func TestReviewArchiveCannotOverwriteSource(t *testing.T) {
	s := reviewStore(t)
	os.MkdirAll(s.dataPath("review-server"), 0755)
	target := filepath.Join(s.dataPath("review-server"), "save.zip")
	os.WriteFile(target, []byte("original archive"), 0644)
	_, e := s.Archive("review-server", "save.zip", "save.zip")
	if e == nil {
		t.Error("archive accepted its source as output")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "original archive" {
		t.Error("original source archive was overwritten")
	}
}
func TestReviewAddingSecondOfflineOperatorPreservesFirst(t *testing.T) {
	s := reviewStore(t)
	if _, e := s.SaveProperties("review-server", "online-mode=false\n"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.AddOP(context.Background(), "review-server", "Alice", 4); e != nil {
		t.Fatal(e)
	}
	ops, e := s.AddOP(context.Background(), "review-server", "Bob", 4)
	if e != nil {
		t.Fatal(e)
	}
	if len(ops) != 2 {
		t.Errorf("adding Bob replaced Alice: %+v", ops)
	}
}
func TestReviewInvalidUpdateHasNoContainerSideEffects(t *testing.T) {
	s := reviewStore(t)
	f := &fakeRuntime{}
	s.docker = f
	s.db.Exec(`UPDATE servers SET container_id='live-container',state='running' WHERE id='review-server'`)
	port := 25600
	badCPU := -1.0
	_, e := s.Update(context.Background(), "review-server", UpdateInput{HostPort: &port, CPULimit: &badCPU})
	if e == nil {
		t.Fatal("expected validation failure")
	}
	if len(f.removedIDs) > 0 {
		t.Errorf("rejected PATCH already force-removed container: %v", f.removedIDs)
	}
}
