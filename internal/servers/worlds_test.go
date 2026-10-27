package servers

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func worldStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	_, err = d.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('one','Test','paper','1.21.4',2048,25565,'stopped','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(d, nil, nil, 25565, 25575, dir, "")
}
func worldPut(t *testing.T, s *Store, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(s.dataPath("one"), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func levelNBT(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	name := func(n string) { _ = binary.Write(&b, binary.BigEndian, uint16(len(n))); b.WriteString(n) }
	b.WriteByte(10)
	name("")
	b.WriteByte(10)
	name("Data")
	b.WriteByte(10)
	name("WorldGenSettings")
	b.WriteByte(4)
	name("seed")
	_ = binary.Write(&b, binary.BigEndian, int64(-9223372036854775807))
	b.WriteByte(0)
	b.WriteByte(10)
	name("Version")
	b.WriteByte(8)
	name("Name")
	name("1.21.4")
	b.WriteByte(0)
	b.WriteByte(0)
	b.WriteByte(0)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, _ = gz.Write(b.Bytes())
	_ = gz.Close()
	return compressed.Bytes()
}
func worldZIP(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "world.zip")
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, data := range entries {
		entry, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write(data)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return file
}
func TestWorldLifecycleAndSeed(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	worldPut(t, s, "server.properties", []byte("# keep me\nmotd=Hello\nlevel-name=world\nlevel-seed=old\nlevel-seed=duplicate\n"))
	input := WorldInput{Name: "survival", Seed: "-9223372036854775807", LevelType: "minecraft:amplified", GenerateStructures: true}
	w, err := s.SaveWorld(ctx, "one", input, true)
	if err != nil || w.Generated || w.Active || *w.Seed != input.Seed {
		t.Fatalf("world=%+v err=%v", w, err)
	}
	if _, err = s.SaveWorld(ctx, "one", input, true); !errors.Is(err, ErrWorldConflict) {
		t.Fatal(err)
	}
	if _, err = s.ActivateWorld(ctx, "one", "survival"); err != nil {
		t.Fatal(err)
	}
	props, _ := s.worldProperties("one")
	if props["level-name"] != "survival" || props["level-seed"] != input.Seed || props["motd"] != "Hello" || props["level-type"] != input.LevelType {
		t.Fatal(props)
	}
	text, _ := os.ReadFile(filepath.Join(s.dataPath("one"), "server.properties"))
	if !strings.Contains(string(text), "# keep me") || strings.Contains(string(text), "duplicate") {
		t.Fatal(string(text))
	}
	if _, err = s.DeleteWorld(ctx, "one", "survival"); err == nil {
		t.Fatal("deleted active world")
	}
	worldPut(t, s, "survival/level.dat", levelNBT(t))
	worldPut(t, s, "survival_nether/DIM-1/region/r.0.0.mca", testRegion())
	list, err := s.ListWorlds(ctx, "one")
	if err != nil || len(list.Worlds) != 1 || !list.Worlds[0].Generated || list.Worlds[0].Version != "1.21.4" || *list.Worlds[0].Seed != "-9223372036854775807" || len(list.Worlds[0].Dimensions) != 2 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if _, err = s.SaveWorld(ctx, "one", input, false); err == nil {
		t.Fatal("changed generated seed")
	}
	if _, err = s.SaveWorld(ctx, "one", WorldInput{Name: "fresh", Seed: "glacier", GenerateStructures: true}, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateWorld(ctx, "one", "fresh"); err != nil {
		t.Fatal(err)
	}
	recovery, err := s.DeleteWorld(ctx, "one", "survival")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(s.dataPath("one"), recovery, "survival_nether", "DIM-1", "region", "r.0.0.mca")); err != nil {
		t.Fatal(err)
	}
	if got := escapeWorldSeed(`🌍\seed`); got != `\ud83c\udf0d\\seed` {
		t.Fatal(got)
	}
}
func TestWorldRejectsUnsafeAndNonWorldPaths(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	for _, name := range []string{"../world", "/tmp/world", "world/new", "world\\new", "world\nseed", "world_nether", ".hidden"} {
		if _, err := s.SaveWorld(ctx, "one", WorldInput{Name: name}, true); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	worldPut(t, s, "mods/plugin.jar", []byte("mod"))
	for _, action := range []func() error{
		func() error { _, e := s.DeleteWorld(ctx, "one", "mods"); return e }, func() error { _, e := s.ActivateWorld(ctx, "one", "mods"); return e },
	} {
		if err := action(); !errors.Is(err, ErrPathNotFound) {
			t.Fatalf("non-world error=%v", err)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(s.dataPath("one"), "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateWorld(ctx, "one", "linked"); !errors.Is(err, ErrInvalidPath) {
		t.Fatal(err)
	}
	worldPut(t, s, "safe/level.dat", levelNBT(t))
	if err := os.Symlink(outside, filepath.Join(s.dataPath("one"), "safe", "region")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListWorlds(ctx, "one"); !errors.Is(err, ErrInvalidPath) {
		t.Fatal(err)
	}
}
func TestWorldRequiresOfflineAndSupportedServer(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	_, _ = s.db.Exec(`UPDATE servers SET state='running' WHERE id='one'`)
	if _, err := s.SaveWorld(ctx, "one", WorldInput{Name: "safe"}, true); !errors.Is(err, ErrWorldBusy) {
		t.Fatal(err)
	}
	list, err := s.ListWorlds(ctx, "one")
	if err != nil || list.Editable {
		t.Fatalf("list=%+v %v", list, err)
	}
	_, _ = s.db.Exec(`UPDATE servers SET state='stopped', container_id='unknown' WHERE id='one'`)
	if _, err := s.SaveWorld(ctx, "one", WorldInput{Name: "safe"}, true); !errors.Is(err, ErrWorldBusy) {
		t.Fatal(err)
	}
	_, _ = s.db.Exec(`UPDATE servers SET server_type='velocity' WHERE id='one'`)
	if _, err := s.ListWorlds(ctx, "one"); !errors.Is(err, ErrWorldUnsupported) {
		t.Fatal(err)
	}
}
func TestWorldArchiveRoundTripAndValidation(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	data := levelNBT(t)
	archive := worldZIP(t, map[string][]byte{"./wrapper/old/level.dat": data, "./wrapper/old/region/r.0.0.mca": testRegion(), "./wrapper/old/playerdata/player.dat": []byte("inventory"), "./wrapper/old/session.lock": []byte("lock"), "./wrapper/old_nether/DIM-1/region/r.0.0.mca": testRegion(), "./wrapper/server.properties": []byte("do not import")})
	w, err := s.ImportWorld(ctx, "one", "imported", archive)
	if err != nil || !w.Generated || w.Active {
		t.Fatalf("world=%+v err=%v", w, err)
	}
	if _, err := os.Stat(filepath.Join(s.dataPath("one"), "server.properties")); !os.IsNotExist(err) {
		t.Fatal("imported server config")
	}
	if _, err = s.ImportWorld(ctx, "one", "imported", archive); !errors.Is(err, ErrWorldConflict) {
		t.Fatal(err)
	}
	exported, err := s.ExportWorld(ctx, "one", "imported")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(exported)
	z, err := zip.OpenReader(exported)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	found := map[string]bool{}
	for _, f := range z.File {
		found[f.Name] = true
	}
	if !found["imported/level.dat"] || !found["imported/playerdata/player.dat"] || !found["imported_nether/DIM-1/region/r.0.0.mca"] || found["imported/session.lock"] {
		t.Fatal(found)
	}
	for _, entries := range []map[string][]byte{
		{"../escape": []byte("bad"), "level.dat": data}, {"one/level.dat": data, "two/level.dat": data}, {"level.dat": []byte("bedrock")}, {"one/level.dat": data, "one/../escape": []byte("bad")},
	} {
		file := worldZIP(t, entries)
		if _, err := s.ImportWorld(ctx, "one", "rejected", file); err == nil {
			t.Fatal("accepted unsafe archive")
		}
		if _, err := os.Stat(filepath.Join(s.dataPath("one"), "rejected")); !os.IsNotExist(err) {
			t.Fatal("partial import published")
		}
	}
}
func testRegion() []byte {
	data := make([]byte, 4*4096)
	binary.BigEndian.PutUint32(data[0:4], 2<<8|1)
	binary.BigEndian.PutUint32(data[4:8], 3<<8|1)
	binary.BigEndian.PutUint32(data[4096:4100], 123)
	binary.BigEndian.PutUint32(data[4100:4104], 456)
	copy(data[8192:], []byte("chunk 0 payload"))
	copy(data[12288:], []byte("chunk 1 payload"))
	return data
}
func TestWorldChunkResetPreservesNeighborsAndRecovery(t *testing.T) {
	s := worldStore(t)
	ctx := context.Background()
	original := testRegion()
	worldPut(t, s, "world/level.dat", levelNBT(t))
	for _, kind := range []string{"region", "entities", "poi"} {
		worldPut(t, s, "world/"+kind+"/r.-1.0.mca", original)
	}
	worldPut(t, s, "world/region/c.-32.0.mcc", []byte("external"))
	list, err := s.ListWorldRegions(ctx, "one", "world", "world", 0)
	if err != nil || list.Total != 1 || list.Regions[0].Chunks != 2 {
		t.Fatalf("regions=%+v err=%v", list, err)
	}
	chunks, err := s.ListWorldChunks(ctx, "one", "world", "world", -1, 0)
	if err != nil || len(chunks) != 2 || chunks[0].X != -32 || chunks[1].X != -31 {
		t.Fatalf("chunks=%+v err=%v", chunks, err)
	}
	result, err := s.ResetWorldChunks(ctx, "one", "world", WorldChunkSelection{Dimension: "world", RegionX: -1, Chunks: []int{0}})
	if err != nil || result.Reset != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, kind := range []string{"region", "entities", "poi"} {
		rel := "world/" + kind + "/r.-1.0.mca"
		now, _ := os.ReadFile(filepath.Join(s.dataPath("one"), rel))
		if binary.BigEndian.Uint32(now[:4]) != 0 || binary.BigEndian.Uint32(now[4096:4100]) != 0 || !bytes.Equal(now[8192:], original[8192:]) || !bytes.Equal(now[4:4096], original[4:4096]) {
			t.Fatal("neighbor or payload changed")
		}
		saved, err := os.ReadFile(filepath.Join(s.dataPath("one"), result.RecoveryPath, rel))
		if err != nil || !bytes.Equal(saved, original) {
			t.Fatal("missing recovery copy")
		}
	}
	if _, err = os.Stat(filepath.Join(s.dataPath("one"), result.RecoveryPath, "world/region/c.-32.0.mcc")); err != nil {
		t.Fatal(err)
	}
	for _, selection := range []WorldChunkSelection{{Dimension: "world", RegionX: -1, Chunks: []int{1, 1}}, {Dimension: "world", RegionX: -1, Chunks: []int{0}}, {Dimension: "../mods", Chunks: []int{1}}, {Dimension: "world", RegionX: -1, Chunks: []int{1024}}} {
		if _, err = s.ResetWorldChunks(ctx, "one", "world", selection); err == nil {
			t.Fatal("accepted invalid reset")
		}
	}
	corrupt := testRegion()
	binary.BigEndian.PutUint32(corrupt[4:8], 2<<8|1)
	worldPut(t, s, "world/region/r.0.0.mca", corrupt)
	if _, err = s.ResetWorldChunks(ctx, "one", "world", WorldChunkSelection{Dimension: "world", Chunks: []int{0}}); err == nil {
		t.Fatal("edited overlapping chunks")
	}
}
func TestWorldMetadataRejectsMalformedNBT(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("bad"), {10, 0, 0, 9, 0, 1, 'a', 1, 0xff, 0xff, 0xff, 0xff}} {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		_, _ = gz.Write(data)
		gz.Close()
		p := filepath.Join(t.TempDir(), "level.dat")
		_ = os.WriteFile(p, b.Bytes(), 0644)
		if _, err := readWorldMetadata(p); err == nil {
			t.Fatal("accepted malformed NBT")
		}
	}
}
