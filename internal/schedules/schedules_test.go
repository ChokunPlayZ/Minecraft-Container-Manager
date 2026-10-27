package schedules

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/db"
)

func TestCronAndTimezones(t *testing.T) {
	for _, tc := range []struct{ expression, zone, after, want string }{
		{"midnight", "Asia/Bangkok", "2026-10-27T16:59:00Z", "2026-10-27T17:00:00Z"},
		{"0 0 * * *", "UTC", "2026-10-27T00:00:00Z", "2026-10-28T00:00:00Z"},
		{"*/15 9-17 * * MON-FRI", "UTC", "2026-10-27T09:01:00Z", "2026-10-27T09:15:00Z"},
		{"@weekly", "UTC", "2026-10-27T09:00:00Z", "2026-11-01T00:00:00Z"},
		{"0 0 * * *", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T04:00:00Z"},
		{"0 0 * * *", "America/New_York", "2026-11-01T04:00:00Z", "2026-11-02T05:00:00Z"},
	} {
		t.Run(tc.expression+tc.zone+tc.after, func(t *testing.T) {
			schedule, err := Parse(tc.expression, tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := time.Parse(time.RFC3339, tc.after)
			if got := stamp(schedule.Next(after)); got != tc.want {
				t.Fatalf("next = %s; want %s", got, tc.want)
			}
		})
	}
	for _, expression := range []string{"", "0 0 * *", "0 0 0 * * *", "60 * * * *", "*/0 * * * *", "@every 1s", "CRON_TZ=UTC 0 0 * * *"} {
		if _, err := Parse(expression, "UTC"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q accepted: %v", expression, err)
		}
	}
	for _, zone := range []string{"", "Local", "Mars/Olympus", "UTC\n"} {
		if _, err := Parse("midnight", zone); !errors.Is(err, ErrInvalid) {
			t.Errorf("zone %q accepted", zone)
		}
	}
}

func TestEnvironmentTimezoneDefaultAndOverride(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"Asia/Bangkok", "Asia/Bangkok"}, {":Europe/London", "Europe/London"}, {"", "UTC"}, {"invalid/zone", "UTC"},
	} {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("TZ", tc.env)
			store, _ := testStore(t)
			in := taskInput()
			in.Timezone = ""
			task, err := store.Create(context.Background(), "server-1", in)
			if err != nil || task.Timezone != tc.want {
				t.Fatalf("default: %+v %v, want %s", task, err, tc.want)
			}
			in.Timezone = "America/New_York"
			task, err = store.Create(context.Background(), "server-1", in)
			if err != nil || task.Timezone != in.Timezone {
				t.Fatalf("override: %+v %v", task, err)
			}
		})
	}
}

func testStore(t *testing.T) (*Store, *db.Store) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	for _, id := range []string{"server-1", "server-2"} {
		_, err := database.DB.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,created_at,updated_at) VALUES (?,?,'paper','1.21.4',2048,'now','now')`, id, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	return NewStore(database.DB), database
}

func taskInput() Input {
	return Input{Name: "Nightly restart", Action: "restart", Cron: "midnight", Timezone: "Asia/Bangkok", Enabled: true}
}

func TestPersistenceValidationAndServerScope(t *testing.T) {
	store, database := testStore(t)
	ctx := context.Background()
	task, err := store.Create(ctx, "server-1", taskInput())
	if err != nil {
		t.Fatal(err)
	}
	if task.NextRunAt == "" || !task.Enabled {
		t.Fatalf("no next run: %+v", task)
	}
	// A new store uses persisted scheduling state, including the next run.
	persisted, err := NewStore(database.DB).Get(ctx, "server-1", task.ID)
	if err != nil || persisted.NextRunAt != task.NextRunAt {
		t.Fatalf("persistence: %+v %v", persisted, err)
	}
	if _, err := store.Get(ctx, "server-2", task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-server read succeeded")
	}
	if _, err := store.Update(ctx, "server-2", task.ID, taskInput()); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-server update succeeded")
	}
	if err := store.Delete(ctx, "server-2", task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-server delete succeeded")
	}
	paused, err := store.SetEnabled(ctx, "server-1", task.ID, false)
	if err != nil || paused.Enabled || paused.NextRunAt != "" {
		t.Fatalf("pause: %+v %v", paused, err)
	}
	resumed, err := store.SetEnabled(ctx, "server-1", task.ID, true)
	if err != nil || !resumed.Enabled || resumed.NextRunAt == "" {
		t.Fatalf("resume: %+v %v", resumed, err)
	}
	for _, change := range []func(*Input){
		func(in *Input) { in.Name = " " },
		func(in *Input) { in.Action = "shell" },
		func(in *Input) { in.Cron = "0 0 31 2 *" },
		func(in *Input) { in.Action = "command"; in.Command = "say hi\nstop" },
		func(in *Input) { in.Action = "command"; in.Command = "" },
	} {
		in := taskInput()
		change(&in)
		if _, err := store.Create(ctx, "server-1", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted invalid input %+v: %v", in, err)
		}
	}
	if _, err := store.Create(ctx, "missing", taskInput()); !errors.Is(err, ErrServerNotFound) {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`DELETE FROM servers WHERE id='server-1'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT count(*) FROM scheduled_tasks`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned schedules: %d %v", count, err)
	}
}

func dueTask(t *testing.T, store *Store, now time.Time) Task {
	t.Helper()
	in := taskInput()
	in.Cron = "* * * * *"
	task, err := store.Create(context.Background(), "server-1", in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`UPDATE scheduled_tasks SET next_run_at=? WHERE id=?`, stamp(now), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestDispatchClaimsAndRecordsFailures(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	task := dueTask(t, store, now)
	var calls atomic.Int32
	execute := func(_ context.Context, got Task) error {
		if got.ID != task.ID || got.Action != "restart" {
			t.Errorf("wrong task %+v", got)
		}
		calls.Add(1)
		return errors.New("runtime unavailable")
	}
	first := NewScheduler(store, execute, log.New(io.Discard, "", 0))
	second := NewScheduler(store, execute, nil)
	first.tick(ctx, now, false)
	second.tick(ctx, now, false)
	first.workers.Wait()
	second.workers.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate dispatch: %d", calls.Load())
	}
	got, err := store.Get(ctx, "server-1", task.ID)
	if err != nil || got.LastStatus != "failed" || got.LastError != "runtime unavailable" || got.LastRunAt != stamp(now) || got.NextRunAt != stamp(now.Add(time.Minute)) {
		t.Fatalf("result: %+v %v", got, err)
	}
}

func TestNoOverlapAndPause(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	task := dueTask(t, store, now)
	var calls atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	scheduler := NewScheduler(store, func(context.Context, Task) error { calls.Add(1); close(started); <-finish; return nil }, nil)
	scheduler.tick(ctx, now, false)
	<-started
	scheduler.tick(ctx, now.Add(time.Minute), false)
	close(finish)
	scheduler.workers.Wait()
	if calls.Load() != 1 {
		t.Fatal("overlapping execution")
	}
	got, _ := store.Get(ctx, "server-1", task.ID)
	if got.LastStatus != "success" {
		t.Fatalf("result: %+v", got)
	}
	if _, err := store.SetEnabled(ctx, "server-1", task.ID, false); err != nil {
		t.Fatal(err)
	}
	scheduler.tick(ctx, now.Add(time.Hour), false)
	scheduler.workers.Wait()
	if calls.Load() != 1 {
		t.Fatal("paused task executed")
	}
}

func TestStartupSkipsMissedRunsAndPanicIsRecorded(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	task := dueTask(t, store, now.Add(-time.Hour))
	scheduler := NewScheduler(store, func(context.Context, Task) error { panic("bad executor") }, log.New(io.Discard, "", 0))
	scheduler.tick(ctx, now, true)
	got, _ := store.Get(ctx, "server-1", task.ID)
	if got.LastStatus != "skipped" || got.LastRunAt != stamp(now.Add(-time.Hour)) || got.NextRunAt != stamp(now.Add(time.Minute)) {
		t.Fatalf("startup: %+v", got)
	}
	scheduler.tick(ctx, now.Add(time.Minute), false)
	scheduler.workers.Wait()
	got, _ = store.Get(ctx, "server-1", task.ID)
	if got.LastStatus != "failed" || got.LastError != "task panicked: bad executor" {
		t.Fatalf("panic: %+v", got)
	}
}
