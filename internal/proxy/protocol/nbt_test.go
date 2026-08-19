package protocol

import (
	"bytes"
	"testing"
)

func TestNBTWriterReadRoundTrip(t *testing.T) {
	w := NewNBTWriter()
	w.Root("")
	w.String("name", "the_end")
	w.Byte("piglin_safe", 1)
	w.Int("height", 256)
	w.Long("seed", 123456789)
	w.End()

	got, err := NewNBTReader(bytes.NewReader(w.Bytes())).Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Type != TagCompound {
		t.Fatalf("root type = %d, want compound", got.Type)
	}
	byName := map[string]*Tag{}
	for _, c := range got.Children {
		byName[c.Name] = c
	}
	if byName["name"].Str != "the_end" {
		t.Errorf("name = %q", byName["name"].Str)
	}
	if byName["height"].Int != 256 {
		t.Errorf("height = %d", byName["height"].Int)
	}
	if byName["seed"].Long != 123456789 {
		t.Errorf("seed = %d", byName["seed"].Long)
	}
}
