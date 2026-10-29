package servers

import (
	"strings"
	"testing"
)

func TestBuildToolsEntrypointRebuildWarning(t *testing.T) {
	for _, serverType := range []string{"spigot", "bukkit", "craftbukkit", "paper"} {
		t.Run(serverType, func(t *testing.T) {
			srv := &Server{ServerType: serverType, ContainerID: "existing"}
			current := currentContainerConfig(srv)
			if current.EntrypointVersion != 3 {
				t.Fatalf("current entrypoint version = %d, want 3", current.EntrypointVersion)
			}
			applied := current
			applied.EntrypointVersion = 2
			needsRebuild, reasons := checkRebuildNeeded(srv, encodeContainerConfig(applied))
			wantRebuild := serverType != "paper"
			if needsRebuild != wantRebuild {
				t.Fatalf("needs rebuild = %v, want %v (reasons: %v)", needsRebuild, wantRebuild, reasons)
			}
			if wantRebuild && (len(reasons) != 1 || !strings.Contains(reasons[0], "BuildTools Maven cache")) {
				t.Fatalf("missing BuildTools rebuild reason: %v", reasons)
			}
			if needsRebuild, reasons := checkRebuildNeeded(srv, encodeContainerConfig(current)); needsRebuild {
				t.Fatalf("rebuilt container still needs rebuild: %v", reasons)
			}
		})
	}
}
