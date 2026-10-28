package servers

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/storage"
	"os"
	"strings"
)

func runtimePaths(root *os.Root) ([]string, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range entries {
		name := e.Name()
		lower := strings.ToLower(name)
		if name == "server.jar" || name == "run.sh" || name == "run.bat" || name == "libraries" || name == "installer.jar" || name == "quilt-server-launch.jar" || name == ".mcm_state" || strings.HasSuffix(lower, ".jar") && (strings.Contains(lower, "forge") || strings.HasPrefix(lower, "spigot-") || strings.HasPrefix(lower, "craftbukkit-")) {
			out = append(out, name)
		}
	}
	return out, nil
}

func (s *Store) stageRuntime(ctx context.Context, srv Server) (rollback func() error, cleanup func() error, err error) {
	root, err := s.openDataRoot(srv.ID)
	if err != nil {
		return nil, nil, err
	}
	stage := ".mcm-runtime-stage-" + uuid.NewString()
	if err = root.Mkdir(stage, 0700); err != nil {
		root.Close()
		return nil, nil, err
	}
	defer root.RemoveAll(stage)
	jt, err := jars.ParseJarType(srv.ServerType)
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	if err = s.jars.DownloadServerJar(ctx, jt, srv.Version, srv.Build, root.Name()+"/"+stage); err != nil {
		root.Close()
		return nil, nil, fmt.Errorf("download server software: %w", err)
	}
	old, err := runtimePaths(root)
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	undo, finish, err := storage.PublishTree(root, stage, old)
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	// Hold the root until either commit cleanup or rollback completes.
	rollback = func() error { defer root.Close(); return undo() }
	cleanup = func() error { defer root.Close(); return finish() }
	return rollback, cleanup, nil
}
