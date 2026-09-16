package servers

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskProgressStore(t *testing.T) {
	store := newTestStore(t, "srv-task-test", StateStopped)
	serverID := "srv-task-test"

	// Initially nil
	if p := store.GetTaskProgress(serverID); p != nil {
		t.Fatalf("expected nil task progress, got %+v", p)
	}

	ch, cancel := store.SubscribeTaskProgress(serverID)
	defer cancel()

	// Update progress
	store.SetTaskProgress(serverID, TaskProgress{
		Operation:  "archive",
		Stage:      "compressing",
		StageTitle: "Compressing Files",
		StageIndex: 2,
		StageTotal: 2,
		Percent:    50,
		Message:    "file.txt",
		Current:    5,
		Total:      10,
	})

	select {
	case p := <-ch:
		if p.Operation != "archive" || p.Percent != 50 || p.Stage != "compressing" {
			t.Fatalf("unexpected progress received: %+v", p)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for task progress event")
	}

	p := store.GetTaskProgress(serverID)
	if p == nil || p.Percent != 50 {
		t.Fatalf("unexpected GetTaskProgress: %+v", p)
	}

	store.ClearTaskProgress(serverID)
	if p := store.GetTaskProgress(serverID); p != nil {
		t.Fatalf("expected nil after clear, got %+v", p)
	}
}

func TestArchiveMultipleEmitsTaskProgress(t *testing.T) {
	store := newTestStore(t, "srv-arch-test", StateStopped)
	serverID := "srv-arch-test"
	dataDir := store.dataPath(serverID)

	testDir := filepath.Join(dataDir, "folder_to_zip")
	_ = os.MkdirAll(testDir, 0o755)
	_ = os.WriteFile(filepath.Join(testDir, "a.txt"), []byte("hello world"), 0o644)
	_ = os.WriteFile(filepath.Join(testDir, "b.txt"), []byte("second file content"), 0o644)

	ch, cancel := store.SubscribeTaskProgress(serverID)
	defer cancel()

	var events []TaskProgress
	done := make(chan struct{})
	go func() {
		defer close(done)
		for p := range ch {
			events = append(events, p)
			if p.Stage == "completed" || p.Stage == "failed" {
				return
			}
		}
	}()

	res, err := store.ArchiveMultiple(serverID, []string{"folder_to_zip"}, "", "output.zip")
	if err != nil {
		t.Fatalf("ArchiveMultiple failed: %v", err)
	}
	if res.Name != "output.zip" {
		t.Errorf("expected archive name output.zip, got %s", res.Name)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for archive progress events to finish")
	}

	if len(events) == 0 {
		t.Fatal("no progress events recorded during ArchiveMultiple")
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Stage != "completed" || lastEvent.Percent != 100 {
		t.Errorf("expected final event to be completed at 100%%, got %+v", lastEvent)
	}
}
