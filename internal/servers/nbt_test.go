package servers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/nbt"
)

func TestNBTEditingBackupAndConflicts(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	for _, rel := range []string{"world/playerdata/player.dat", "world/level.dat", "structures/castle.nbt", "schematics/castle.schem"} {
		t.Run(rel, func(t *testing.T) {
			original := levelNBT(t)
			worldPut(t, s, rel, original)
			opened, err := s.ReadNBT(ctx, "one", rel)
			if err != nil || !opened.Editable {
				t.Fatalf("open: %+v %v", opened, err)
			}
			opened.Root.Children[0].Children[0].Children[0].Value = "9223372036854775807"
			saved, err := s.WriteNBT(ctx, "one", rel, opened.Revision, opened.Document)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Revision == opened.Revision || saved.BackupPath == "" || saved.Compression != "gzip" {
				t.Fatalf("save: %+v", saved)
			}
			backup, err := os.ReadFile(filepath.Join(s.dataPath("one"), filepath.FromSlash(saved.BackupPath)))
			if err != nil || !bytes.Equal(backup, original) {
				t.Fatal("backup does not match original", err)
			}
			if _, err = s.WriteNBT(ctx, "one", rel, opened.Revision, opened.Document); !errors.Is(err, ErrNBTChanged) {
				t.Fatal("stale save was accepted", err)
			}
			if _, err = s.WriteNBT(ctx, "one", rel, "", saved.Document); !errors.Is(err, ErrNBTChanged) {
				t.Fatal("missing revision accepted", err)
			}
			if err = os.Remove(filepath.Join(s.dataPath("one"), filepath.FromSlash(rel))); err != nil {
				t.Fatal(err)
			}
			if _, err = s.WriteNBT(ctx, "one", rel, saved.Revision, saved.Document); !errors.Is(err, ErrNBTChanged) {
				t.Fatal("deleted file recreated", err)
			}
		})
	}
}

func TestNBTRejectsUnsafePathsAndRunningSaves(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	worldPut(t, s, "world/level.dat", levelNBT(t))
	opened, err := s.ReadNBT(ctx, "one", "world/level.dat")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(s.dataPath("one"), "link")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"", ".", "../secret", "/etc/passwd", "world/../../secret", "link/test.nbt", "world\\level.dat"} {
		if _, err = s.ReadNBT(ctx, "one", rel); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("read unsafe %q: %v", rel, err)
		}
		if _, err = s.WriteNBT(ctx, "one", rel, opened.Revision, opened.Document); !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("write unsafe %q: %v", rel, err)
		}
	}
	for _, state := range []string{StateRunning, StateStarting, StateStopping, StateInstalling, StateBuilding} {
		if err = s.setState(ctx, "one", state); err != nil {
			t.Fatal(err)
		}
		read, err := s.ReadNBT(ctx, "one", "world/level.dat")
		if err != nil || read.Editable {
			t.Fatalf("running read: %+v %v", read, err)
		}
		if _, err = s.WriteNBT(ctx, "one", read.Path, read.Revision, read.Document); !errors.Is(err, ErrNBTBusy) {
			t.Fatalf("saved in %s: %v", state, err)
		}
	}
	if err = s.setState(ctx, "one", StateStopped); err != nil {
		t.Fatal(err)
	}
	invalid := nbt.Document{Root: nbt.Tag{Type: 1, Value: "999"}}
	if _, err = s.WriteNBT(ctx, "one", opened.Path, opened.Revision, invalid); !errors.Is(err, nbt.ErrInvalid) {
		t.Fatal("invalid edit saved", err)
	}
	got, _ := os.ReadFile(filepath.Join(s.dataPath("one"), "world/level.dat"))
	if !bytes.Equal(got, levelNBT(t)) {
		t.Fatal("failed edit changed file")
	}
}

func TestNBTConcurrentSave(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	worldPut(t, s, "world/level.dat", levelNBT(t))
	opened, err := s.ReadNBT(ctx, "one", "world/level.dat")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			doc := opened.Document
			doc.Root.Name = name
			_, err := s.WriteNBT(ctx, "one", opened.Path, opened.Revision, doc)
			results <- err
		}(name)
	}
	wg.Wait()
	close(results)
	saved, conflicts := 0, 0
	for err := range results {
		if err == nil {
			saved++
		} else if errors.Is(err, ErrNBTChanged) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("saves=%d conflicts=%d", saved, conflicts)
	}
}

func TestPlayerDataDiscovery(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	const id = "8667ba71-b85a-4004-af54-457a9734eed7"
	for _, rel := range []string{"world/playerdata/" + id + ".dat", "worlds/modded/playerdata/" + id + ".dat", "world/playerdata/not-a-uuid.dat", "world/playerdata/" + id + ".dat_old", "plugins/playerdata/" + id + ".dat", ".mcm-world-trash/deleted/playerdata/" + id + ".dat"} {
		worldPut(t, s, rel, levelNBT(t))
	}
	worldPut(t, s, "usercache.json", []byte(`[{"uuid":"`+id+`","name":"Steve"}]`))
	if err := os.Symlink(t.TempDir(), filepath.Join(s.dataPath("one"), "linked-world")); err != nil {
		t.Fatal(err)
	}
	players, err := s.ListPlayerData(ctx, "one")
	if err != nil || len(players) != 2 {
		t.Fatalf("players=%+v err=%v", players, err)
	}
	for _, p := range players {
		if p.UUID != id || p.Name != "Steve" || p.ModifiedAt.IsZero() {
			t.Fatalf("bad player: %+v", p)
		}
	}
	if _, err = s.ListPlayerData(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing server", err)
	}
}
