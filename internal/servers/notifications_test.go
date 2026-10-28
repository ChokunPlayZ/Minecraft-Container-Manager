package servers

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/notifications"
)

type eventRecorder struct {
	mu     sync.Mutex
	events []notifications.Event
}

func (r *eventRecorder) Notify(e notifications.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func notificationStore(t *testing.T, state string) (*Store, string, *eventRecorder, *fakeRuntime) {
	t.Helper()
	dir := t.TempDir()
	h, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	s := NewStore(h, nil, nil, 25565, 25575, dir, "")
	t.Cleanup(func() { s.WaitProvisioning(context.Background()) })
	id := "notification-server"
	_, err = h.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,container_id,state,created_at,updated_at) VALUES(?, 'Survival', 'custom', 'custom', 1024, 25565, 'container', ?, '', '')`, id, state)
	if err != nil {
		t.Fatal(err)
	}
	r := &eventRecorder{}
	s.SetNotifier(r)
	fake := &fakeRuntime{existing: map[string]bool{"container": true}}
	s.docker = fake
	return s, id, r, fake
}

func TestNotificationCrashReconciliation(t *testing.T) {
	for _, code := range []int{0, 1, 137} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			s, id, recorder, fake := notificationStore(t, StateRunning)
			fake.inspectState = &docker.ContainerState{Status: "exited", ExitCode: code}
			// Background reconciliation, without any API or browser request.
			if err := s.reconcileStatuses(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := s.reconcileStatuses(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(recorder.events) != 1 || recorder.events[0].Kind != notifications.ServerCrashed || recorder.events[0].ServerName != "Survival" {
				t.Fatalf("events: %+v", recorder.events)
			}
			current, _ := s.Get(context.Background(), id)
			if code == 137 && current.State != StateError {
				t.Fatalf("state: %s", current.State)
			}
		})
	}
}

func TestNotificationIntentionalStopAndRestart(t *testing.T) {
	s, id, recorder, fake := notificationStore(t, StateRunning)
	fake.inspectState = &docker.ContainerState{Status: "running"}
	if _, err := s.Stop(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stop(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := s.reconcileStatuses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Kind != notifications.ServerStopped {
		t.Fatalf("stop events: %+v", recorder.events)
	}
	if _, err := s.Start(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 2 || recorder.events[1].Kind != notifications.ServerStarted {
		t.Fatalf("start events: %+v", recorder.events)
	}
	if _, err := s.Restart(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 4 || recorder.events[2].Kind != notifications.ServerStopped || recorder.events[3].Kind != notifications.ServerStarted {
		t.Fatalf("restart events: %+v", recorder.events)
	}
}

func TestNotificationStateRaces(t *testing.T) {
	s, id, recorder, fake := notificationStore(t, StateRunning)
	fake.inspectState = &docker.ContainerState{Status: "exited", ExitCode: 1}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Status(context.Background(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(recorder.events) != 1 {
		t.Fatalf("duplicate events: %+v", recorder.events)
	}
	previous := StateRunning
	changed, err := s.changeState(context.Background(), id, StateStopped, nil, &previous, "", "")
	if err != nil || changed {
		t.Fatalf("stale observation applied: %v %v", changed, err)
	}
	// A status check during a lifecycle operation must not report a crash.
	mu := s.lifecycleLock(id)
	mu.Lock()
	_, err = s.Status(context.Background(), id)
	mu.Unlock()
	if err != nil || len(recorder.events) != 1 {
		t.Fatalf("in-flight events: %+v %v", recorder.events, err)
	}
}

func TestNotificationErrorsAndInstallation(t *testing.T) {
	s, id, recorder, _ := notificationStore(t, StateStopped)
	ctx := context.Background()
	for _, state := range []string{StateInstalling, StateStopped, StateError, StateError} {
		if err := s.setState(ctx, id, state); err != nil {
			t.Fatal(err)
		}
	}
	if len(recorder.events) != 1 || recorder.events[0].Kind != notifications.ServerError {
		t.Fatalf("events: %+v", recorder.events)
	}
}

func TestNotificationConsoleStopRemainsIntentional(t *testing.T) {
	s, _, recorder, fake := notificationStore(t, StateStopping)
	fake.inspectState = &docker.ContainerState{Status: "running"}
	if err := s.reconcileStatuses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("shutdown misreported as a start: %+v", recorder.events)
	}
	fake.inspectState = &docker.ContainerState{Status: "exited"}
	if err := s.reconcileStatuses(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(recorder.events) != 1 || recorder.events[0].Kind != notifications.ServerStopped {
		t.Fatalf("events: %+v", recorder.events)
	}
}

func TestNotificationKillDoesNotWaitForLifecycle(t *testing.T) {
	s, id, recorder, _ := notificationStore(t, StateRunning)
	mu := s.lifecycleLock(id)
	mu.Lock()
	defer mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := s.Kill(context.Background(), id); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("force stop waited for an in-flight lifecycle operation")
	}
	if len(recorder.events) != 1 || recorder.events[0].Kind != notifications.ServerStopped {
		t.Fatalf("events: %+v", recorder.events)
	}
}
