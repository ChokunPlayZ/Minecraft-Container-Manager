package servers

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/jars"
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
	want := map[string]PlayerDataFile{
		"world/playerdata/" + id + ".dat":         {UUID: id, Name: "Steve", World: "world"},
		"worlds/modded/playerdata/" + id + ".dat": {UUID: id, Name: "Steve", World: "worlds/modded"},
		"survival/players/data/" + id + ".dat":    {UUID: id, Name: "Steve", World: "survival"},
		"worlds/modern/survival/season/one/players/data/" + id + ".dat": {
			UUID: id, Name: "Steve", World: "worlds/modern/survival/season/one",
		},
		"classic/players/OldSteve.dat": {Name: "OldSteve", World: "classic"},
	}
	data := levelNBT(t)
	for rel := range want {
		worldPut(t, s, rel, data)
	}
	worldPut(t, s, "classic/level.dat", data)
	worldPut(t, s, "survival/level.dat", data)
	for _, rel := range []string{
		"world/playerdata/not-a-uuid.dat",
		"world/playerdata/" + id + ".dat_old",
		"survival/players/data/" + id + ".dat_old",
		"survival/players/stats/" + id + ".json",
		"survival/data/" + id + ".dat",
		"plugins/playerdata/" + id + ".dat",
		"plugins/world/players/data/" + id + ".dat",
		".mcm-world-trash/deleted/playerdata/" + id + ".dat",
		".mcm-world-trash/deleted/players/data/" + id + ".dat",
		"config/players/settings.dat",
		"classic/players/not-a-player.dat",
	} {
		worldPut(t, s, rel, data)
	}
	worldPut(t, s, "usercache.json", []byte(`[{"uuid":"`+id+`","name":"Steve"}]`))
	if err := os.Symlink("survival", filepath.Join(s.dataPath("one"), "linked-world")); err != nil {
		t.Fatal(err)
	}
	for _, jarType := range []jars.JarType{
		jars.TypeVanilla, jars.TypeFabric, jars.TypeQuilt, jars.TypeForge, jars.TypeNeoForge,
		jars.TypePaper, jars.TypeSpigot, jars.TypePurpur, jars.TypeFolia, jars.TypePufferfish, jars.TypeLeaf,
		jars.TypeSponge, jars.TypeMohist, jars.TypeYouer, jars.TypeKetting, jars.TypeCrucible, jars.TypeCustom,
	} {
		t.Run(string(jarType), func(t *testing.T) {
			if _, err := s.db.Exec(`UPDATE servers SET server_type=? WHERE id='one'`, jarType); err != nil {
				t.Fatal(err)
			}
			players, err := s.ListPlayerData(ctx, "one")
			if err != nil || len(players) != len(want) {
				t.Fatalf("players=%+v err=%v", players, err)
			}
			seen := make(map[string]bool, len(players))
			for _, p := range players {
				entry, ok := want[p.Path]
				if !ok || seen[p.Path] || p.UUID != entry.UUID || p.Name != entry.Name || p.World != entry.World || p.ModifiedAt.IsZero() {
					t.Fatalf("unexpected player: %+v", p)
				}
				seen[p.Path] = true
			}
		})
	}
	if _, err := s.ListPlayerData(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing server", err)
	}
}
