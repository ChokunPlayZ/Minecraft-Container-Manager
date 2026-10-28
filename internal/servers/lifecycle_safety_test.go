package servers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/errdefs"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type failingRemoveRuntime struct {
	fakeRuntime
	removeError error
}

func (r *failingRemoveRuntime) Remove(context.Context, string) error { return r.removeError }
func TestLifecycleRemovalFailureKeepsTracking(t *testing.T) {
	for _, operation := range []string{"delete", "recreate", "update"} {
		t.Run(operation, func(t *testing.T) {
			s := reviewStore(t)
			runtime := &failingRemoveRuntime{removeError: errors.New("daemon unavailable")}
			s.docker = runtime
			s.db.Exec(`UPDATE servers SET container_id='tracked' WHERE id='review-server'`)
			var err error
			switch operation {
			case "delete":
				err = s.Delete(context.Background(), "review-server")
			case "recreate":
				_, err = s.Recreate(context.Background(), "review-server")
			case "update":
				port := 25600
				_, err = s.Update(context.Background(), "review-server", UpdateInput{HostPort: &port})
			}
			if err == nil {
				t.Fatal("runtime removal error ignored")
			}
			srv, err := s.Get(context.Background(), "review-server")
			if err != nil || srv.ContainerID != "tracked" {
				t.Fatalf("lost tracking: %+v %v", srv, err)
			}
		})
	}
	t.Run("confirmed missing", func(t *testing.T) {
		s := reviewStore(t)
		s.docker = &failingRemoveRuntime{removeError: errdefs.NotFound(errors.New("missing"))}
		s.db.Exec(`UPDATE servers SET container_id='missing' WHERE id='review-server'`)
		if _, err := s.Recreate(context.Background(), "review-server"); err != nil {
			t.Fatal(err)
		}
	})
}
func TestDataOperationExcludesStartsAndWorldChanges(t *testing.T) {
	s := reviewStore(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- s.WithStoppedData(context.Background(), "review-server", true, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	attempted := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		close(attempted)
		unlock := s.lockWorld("review-server", true)
		defer unlock()
		close(acquired)
	}()
	<-attempted
	select {
	case <-acquired:
		t.Fatal("world mutation overlapped restore")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("data lock not released")
	}
}
func TestConcurrentPortClaimsAreAtomic(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var wait sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			s := NewStore(database, nil, jars.NewResolver(), 25565, 25575, dir, "")
			<-start
			_, err := s.Create(context.Background(), CreateInput{Name: fmt.Sprint(i), ServerType: jars.TypeCustom, RAMMB: 1024, NoHostPort: true, ExtraPorts: []ExtraPort{{HostPort: 30000, ContainerPort: 30000, Protocol: "tcp"}}})
			results <- err
		}(i)
	}
	close(start)
	wait.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrPortInUse) {
			t.Fatal(err)
		}
	}
	if success != 1 {
		t.Fatalf("%d simultaneous port claims succeeded", success)
	}
	var count int
	database.DB.QueryRow(`SELECT COUNT(*) FROM server_port_reservations WHERE host_port=30000`).Scan(&count)
	if count != 1 {
		t.Fatal("reservation missing")
	}
}
func TestFailedModUploadPreservesSameNameArtifact(t *testing.T) {
	s := reviewStore(t)
	dir := filepath.Join(s.dataPath("review-server"), "mods")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "old.jar"), []byte("original"), 0644)
	if _, err := s.UploadMod(context.Background(), "review-server", "old.jar", strings.NewReader("provider error page")); err == nil {
		t.Fatal("non-JAR upload accepted")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "old.jar"))
	if string(data) != "original" {
		t.Fatal("bad replacement destroyed previous jar")
	}
}
func TestConcurrentMetadataPublicationKeepsAllEntries(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- writeModMeta(dir, fmt.Sprintf("mod-%d.jar", i), ModDownloadMeta{Provider: "modrinth", ProjectID: fmt.Sprint(i)})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	meta, err := readModsMetaChecked(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		if meta[fmt.Sprintf("mod-%d.jar", i)].ProjectID != fmt.Sprint(i) {
			t.Fatalf("lost metadata entry %d", i)
		}
	}
}

func TestArtifactChecksumFailurePreservesPreviousBytes(t *testing.T) {
	s := reviewStore(t)
	dir := filepath.Join(s.dataPath("review-server"), "mods")
	os.MkdirAll(dir, 0755)
	old := filepath.Join(dir, "same.jar")
	os.WriteFile(old, []byte("previous"), 0644)
	payload := fixtureJar(t)
	if _, err := s.UploadMod(context.Background(), "review-server", "same.jar", bytes.NewReader(payload), ModDownloadMeta{Hashes: map[string]string{"sha1": "wrong"}}); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if data, _ := os.ReadFile(old); string(data) != "previous" {
		t.Fatal("checksum failure replaced previous file")
	}
	if _, err := s.UploadMod(context.Background(), "review-server", "same.jar", bytes.NewReader(payload), ModDownloadMeta{Hashes: map[string]string{"sha1": fixtureJarSHA1(t)}}); err != nil {
		t.Fatal(err)
	}
}
