package servers

import (
	"path/filepath"
	"testing"
)

func TestDockerDataPathFallsBackToDataDir(t *testing.T) {
	s := &Store{dataDir: "/var/lib/mcm"}
	want := filepath.Join("/var/lib/mcm", "servers", "abc")
	if got := s.dockerDataPath("abc"); got != want {
		t.Fatalf("dockerDataPath = %q, want %q", got, want)
	}
}

func TestDockerDataPathUsesHostDir(t *testing.T) {
	s := &Store{dataDir: "/data", dataDirHost: "/srv/mcm"}
	want := filepath.Join("/srv/mcm", "servers", "abc")
	if got := s.dockerDataPath("abc"); got != want {
		t.Fatalf("dockerDataPath = %q, want %q", got, want)
	}
}
