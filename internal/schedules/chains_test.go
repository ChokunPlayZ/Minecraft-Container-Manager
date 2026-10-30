package schedules

import (
	"context"
	"errors"
	"io"
	"log"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func backupChain() Input {
	return Input{Name: "Announced backup", Action: "chain", Cron: "midnight", Timezone: "UTC", Enabled: true, Steps: []Step{
		{Action: "command", Command: `broadcast "Backup Starting"`},
		{Action: "command", Command: "save-all"},
		{Action: "backup", BackupTarget: "s3"},
		{Action: "command", Command: `broadcast "Backup Complete"`},
	}}
}

func TestChainPersistenceEditingAndValidation(t *testing.T) {
	store, database := testStore(t)
	ctx := context.Background()
	input := backupChain()
	input.Steps[1].Command = "  save-all  "
	task, err := store.Create(ctx, "server-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if input.Steps[1].Command != "  save-all  " {
		t.Fatal("normalizing a chain mutated the input slice")
	}
	want := backupChain().Steps
	persisted, err := NewStore(database.DB).Get(ctx, "server-1", task.ID)
	if err != nil || !reflect.DeepEqual(persisted.Steps, want) {
		t.Fatalf("steps not preserved: %+v %v", persisted, err)
	}
	input.Steps = []Step{{Action: "delay", DelaySeconds: 30}, {Action: "start"}, {Action: "stop"}, {Action: "restart"}, {Action: "backup"}}
	updated, err := store.Update(ctx, "server-1", task.ID, input)
	if err != nil || !reflect.DeepEqual(updated.Steps, input.Steps) {
		t.Fatalf("edited steps not preserved: %+v %v", updated, err)
	}
	paused, err := store.SetEnabled(ctx, "server-1", task.ID, false)
	if err != nil || paused.Enabled || paused.NextRunAt != "" || !reflect.DeepEqual(paused.Steps, input.Steps) {
		t.Fatalf("pausing lost steps: %+v %v", paused, err)
	}
	resumed, err := store.SetEnabled(ctx, "server-1", task.ID, true)
	if err != nil || !resumed.Enabled || resumed.NextRunAt == "" || !reflect.DeepEqual(resumed.Steps, input.Steps) {
		t.Fatalf("resuming lost steps: %+v %v", resumed, err)
	}
	if _, err := store.Get(ctx, "server-2", task.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-server chain read: %v", err)
	}
	for _, steps := range [][]Step{
		nil, {}, make([]Step, MaxSteps+1),
		{{Action: "chain"}}, {{Action: "shell", Command: "whoami"}},
		{{Action: "command"}}, {{Action: "command", Command: "say hello\nstop"}},
		{{Action: "command", Command: strings.Repeat("a", 4097)}},
		{{Action: "backup", BackupTarget: "invalid"}},
		{{Action: "delay", DelaySeconds: 0}}, {{Action: "delay", DelaySeconds: -1}}, {{Action: "delay", DelaySeconds: 3601}},
	} {
		invalid := backupChain()
		invalid.Steps = steps
		if _, err := store.Update(ctx, "server-1", task.ID, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid chain accepted: %+v %v", steps, err)
		}
	}
	// A rejected edit must leave the last saved chain intact.
	persisted, err = store.Get(ctx, "server-1", task.ID)
	if err != nil || !reflect.DeepEqual(persisted.Steps, input.Steps) {
		t.Fatalf("invalid edit changed the saved chain: %+v %v", persisted, err)
	}
	input.Action = "restart"
	updated, err = store.Update(ctx, "server-1", task.ID, input)
	if err != nil || len(updated.Steps) != 0 {
		t.Fatalf("switching to single action kept steps: %+v %v", updated, err)
	}
}

func TestChainRunsInOrderWaitsForBackupAndDoesNotOverlap(t *testing.T) {
	store, _ := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Minute)
	input := backupChain()
	input.Cron = "* * * * *"
	task, err := store.Create(ctx, "server-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE scheduled_tasks SET next_run_at=? WHERE id=?`, stamp(now), task.ID); err != nil {
		t.Fatal(err)
	}
	backupStarted, finishBackup := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var executed []Step
	scheduler := NewScheduler(store, func(ctx context.Context, got Task) error {
		if got.ID != task.ID || got.ServerID != "server-1" || len(got.Steps) != 0 {
			t.Errorf("step lost task identity or retained nested steps: %+v", got)
		}
		calls.Add(1)
		executed = append(executed, Step{Action: got.Action, Command: got.Command, BackupTarget: got.BackupTarget})
		if got.Action == "backup" {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) < 29*time.Minute {
				t.Error("backup step lost its 30-minute timeout")
			}
			close(backupStarted)
			<-finishBackup
		}
		return nil
	}, nil)
	scheduler.tick(ctx, now, false)
	<-backupStarted
	scheduler.tick(ctx, now.Add(time.Minute), false)
	if calls.Load() != 3 {
		t.Fatalf("chain advanced before backup completed or overlapped: %d calls", calls.Load())
	}
	close(finishBackup)
	scheduler.workers.Wait()
	if !reflect.DeepEqual(executed, input.Steps) {
		t.Fatalf("wrong action order: %+v", executed)
	}
	got, err := store.Get(ctx, "server-1", task.ID)
	if err != nil || got.LastStatus != "success" || got.LastError != "" {
		t.Fatalf("chain result: %+v %v", got, err)
	}
}

func TestChainStopsAtFailedOrPanickedStep(t *testing.T) {
	for _, panicStep := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicStep], func(t *testing.T) {
			store, _ := testStore(t)
			now := time.Now().UTC().Truncate(time.Minute)
			task, err := store.Create(context.Background(), "server-1", backupChain())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(`UPDATE scheduled_tasks SET next_run_at=? WHERE id=?`, stamp(now), task.ID); err != nil {
				t.Fatal(err)
			}
			calls := 0
			scheduler := NewScheduler(store, func(_ context.Context, task Task) error {
				calls++
				if task.Action == "backup" {
					if panicStep {
						panic("storage unavailable")
					}
					return errors.New("storage unavailable")
				}
				return nil
			}, log.New(io.Discard, "", 0))
			scheduler.tick(context.Background(), now, false)
			scheduler.workers.Wait()
			got, err := store.Get(context.Background(), "server-1", task.ID)
			if err != nil || calls != 3 || got.LastStatus != "failed" || !strings.Contains(got.LastError, "step 3 of 4 (backup)") || !strings.Contains(got.LastError, "storage unavailable") {
				t.Fatalf("chain did not stop and record failed step: calls=%d task=%+v err=%v", calls, got, err)
			}
		})
	}
}

func TestChainWaitAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	scheduler := NewScheduler(nil, func(context.Context, Task) error { calls++; cancel(); return nil }, nil)
	err := scheduler.executeTask(ctx, Task{Action: "chain", Steps: []Step{{Action: "command", Command: "save-all"}, {Action: "backup"}}})
	if calls != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("chain continued after cancellation: calls=%d err=%v", calls, err)
	}
	// Cancellation while waiting must prevent all subsequent steps.
	err = scheduler.executeTask(ctx, Task{Action: "chain", Steps: []Step{{Action: "delay", DelaySeconds: 3600}, {Action: "backup"}}})
	if calls != 1 || !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "step 1 of 2 (delay)") {
		t.Fatalf("wait ignored cancellation: calls=%d err=%v", calls, err)
	}
	started := time.Now()
	if err := wait(context.Background(), 10*time.Millisecond); err != nil || time.Since(started) < 10*time.Millisecond {
		t.Fatalf("wait returned too early: %v", err)
	}
}
