package buildinfo

import "testing"

func TestBuildStamp(t *testing.T) {
	oldCommit, oldTime, oldDirty := Commit, BuiltAt, Dirty
	t.Cleanup(func() { Commit, BuiltAt, Dirty = oldCommit, oldTime, oldDirty })
	Commit, BuiltAt, Dirty = "1234567890abcdef", "2026-10-28T10:00:00Z", "true"
	info := Current()
	if info.Commit != Commit || info.BuiltAt != BuiltAt || !info.Dirty {
		t.Fatalf("stamp lost: %+v", info)
	}
	Commit, BuiltAt, Dirty = "", "", ""
	if Current().Commit == "" {
		t.Fatal("unstamped builds must report an explicit unknown revision")
	}
}
