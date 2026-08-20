package servers

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/db"
)

func TestBuildPlayerCommand(t *testing.T) {
	cases := []struct {
		name    string
		action  string
		args    PlayerCommandArgs
		want    string
		wantErr bool
	}{
		{name: "Steve", action: "kick", want: "kick Steve"},
		{name: "Steve", action: "kick", args: PlayerCommandArgs{Reason: "AFK"}, want: "kick Steve AFK"},
		{name: "Notch", action: "ban", args: PlayerCommandArgs{Reason: "cheating"}, want: "ban Notch cheating"},
		{name: "Steve", action: "pardon", args: PlayerCommandArgs{Target: "Notch"}, want: "pardon Notch"},
		{name: "Steve", action: "op", want: "op Steve"},
		{name: "Steve", action: "deop", want: "deop Steve"},
		{name: "Steve", action: "give", args: PlayerCommandArgs{Item: "minecraft:diamond", Amount: 5}, want: "give Steve minecraft:diamond 5"},
		{name: "Steve", action: "give", args: PlayerCommandArgs{Item: "diamond_sword"}, want: "give Steve diamond_sword 1"},
		{name: "Steve", action: "gamemode", args: PlayerCommandArgs{Mode: "creative"}, want: "gamemode creative Steve"},
		{name: "Steve", action: "gamemode", args: PlayerCommandArgs{Mode: "1"}, want: "gamemode 1 Steve"},
		{name: "Steve", action: "tp", args: PlayerCommandArgs{Target: "Alex"}, want: "tp Steve Alex"},
		{name: "Steve", action: "kill", want: "kill Steve"},
		{name: "Steve", action: "custom", args: PlayerCommandArgs{Command: "time set day"}, want: "time set day"},
	}
	for _, tc := range cases {
		t.Run(tc.action+"_"+tc.name, func(t *testing.T) {
			got, err := BuildPlayerCommand(tc.name, tc.action, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("BuildPlayerCommand = %q want %q", got, tc.want)
			}
		})
	}
}

func TestBuildPlayerCommandInvalid(t *testing.T) {
	bad := []struct {
		name   string
		action string
		args   PlayerCommandArgs
	}{
		{name: "../evil", action: "kick"},
		{name: "Steve", action: "pardon"}, // missing target
		{name: "Steve", action: "give"},   // missing item
		{name: "Steve", action: "give", args: PlayerCommandArgs{Item: "../bomb"}},
		{name: "Steve", action: "gamemode", args: PlayerCommandArgs{Mode: "hamster"}},
		{name: "Steve", action: "tp"},     // missing target
		{name: "Steve", action: "fly"},    // unknown action
		{name: "Steve", action: "custom"}, // empty custom
		{name: "Steve", action: "custom", args: PlayerCommandArgs{Command: "rm -rf / && echo hi"}},
	}
	for _, tc := range bad {
		if _, err := BuildPlayerCommand(tc.name, tc.action, tc.args); err == nil {
			t.Fatalf("expected error for %q/%q %+v", tc.name, tc.action, tc.args)
		}
	}
}

// newTestStore opens an in-memory sqlite Store with an inserted server row. It
// bypasses Create so we do not need jars/ports resolvers for this test.
func newTestStore(t *testing.T, id, state string) *Store {
	t.Helper()
	dir := t.TempDir()
	dbHandle, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	s := &Store{db: dbHandle.DB, dataDir: dir}
	now := time.Now().UTC().Format(time.RFC3339)
	if id == "" {
		id = uuid.NewString()
	}
	if _, err := dbHandle.DB.ExecContext(context.Background(),
		`INSERT INTO servers (id, name, server_type, version, build, ram_mb, cpu_limit, memory_limit_mb, host_port, extra_ports, container_id, state, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "test", "vanilla", "1.21.1", "", 1024, 0, 0, 25565, "[]", "", state, now, now); err != nil {
		t.Fatalf("insert server: %v", err)
	}
	return s
}

func TestRunPlayerCommandRCONDisabled(t *testing.T) {
	s := newTestStore(t, "", StateRunning)
	ctx := context.Background()
	id := firstServerID(t, s)
	srv, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("get server: %v", err)
	}

	// Ensure server.properties exists but does not enable RCON.
	if _, err := s.SaveProperties(srv.ID, "server-port=25565\n"); err != nil {
		t.Fatalf("save properties: %v", err)
	}
	s.db.ExecContext(context.Background(), `UPDATE servers SET state=? WHERE id=?`, StateRunning, srv.ID)
	_, err = s.RunPlayerCommand(ctx, srv.ID, "Steve", "kick Steve")
	if !errors.Is(err, ErrRCONDisabled) {
		t.Fatalf("RunPlayerCommand with RCON disabled = %v, want ErrRCONDisabled", err)
	}
}

func TestRunPlayerCommandServerNotRunning(t *testing.T) {
	s := newTestStore(t, "", StateStopped)
	ctx := context.Background()
	id := firstServerID(t, s)
	srv, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	_, err = s.RunPlayerCommand(ctx, srv.ID, "Steve", "kick Steve")
	if !errors.Is(err, ErrServerNotRunning) {
		t.Fatalf("RunPlayerCommand on stopped server = %v, want ErrServerNotRunning", err)
	}
}

func firstServerID(t *testing.T, s *Store) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT id FROM servers LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("query server id: %v", err)
	}
	return id
}
