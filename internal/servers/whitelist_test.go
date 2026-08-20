package servers

import (
	"path/filepath"
	"testing"
)

func TestWhitelistReadMissingIsEmpty(t *testing.T) {
	s := &Store{dataDir: t.TempDir()}
	entries, err := s.readWhitelist("some-id")
	if err != nil {
		t.Fatalf("readWhitelist(missing) returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty whitelist, got %v", entries)
	}
}

func TestWhitelistWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := &Store{dataDir: dir}
	id := "abc"
	entries := []WhitelistEntry{
		{UUID: placeholderUUID, Name: "Steve"},
		{UUID: "123e4567-e89b-12d3-a456-426614174000", Name: "Alex"},
	}
	if err := s.writeWhitelist(id, entries); err != nil {
		t.Fatalf("writeWhitelist returned error: %v", err)
	}

	// Parent data dir should be created.
	got, err := s.readWhitelist(id)
	if err != nil {
		t.Fatalf("readWhitelist returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("round-trip mismatch: got %v, want 2 entries", got)
	}
	if got[0].Name != "Steve" || got[1].Name != "Alex" {
		t.Fatalf("names not preserved: %v", got)
	}

	if _, err := filepath.Abs(s.whitelistPath(id)); err != nil {
		t.Fatalf("whitelistPath: %v", err)
	}
}

func TestWhitelistCaseInsensitiveLookup(t *testing.T) {
	if !equalFold("steve", "Steve") {
		t.Fatal("equalFold should be case-insensitive")
	}
	if equalFold("steve", "alex") {
		t.Fatal("equalFold should not match different names")
	}
}
