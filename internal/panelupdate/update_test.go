package panelupdate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testCommit = "1234567890abcdef1234567890abcdef12345678"

func testManager(t *testing.T) (*Manager, *[][]string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "install.conf"), []byte("SOURCE_DIR=/checkout with spaces\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := &[][]string{}
	m := &Manager{configDir: root, directory: filepath.Join(root, "panel-update"), executable: "/bin with spaces/mcm", helper: "/bin with spaces/mcmctl"}
	m.run = func(_ context.Context, name string, args ...string) (string, error) {
		*calls = append(*calls, append([]string{name}, args...))
		joined := strings.Join(args, " ")
		if name == "systemctl" {
			return "active", nil
		}
		if name == "systemd-run" {
			return "", nil
		}
		switch {
		case strings.Contains(joined, "status"):
			return "", nil
		case strings.Contains(joined, "symbolic-ref"):
			return "main", nil
		case strings.HasSuffix(joined, ".remote"):
			return "origin", nil
		case strings.HasSuffix(joined, ".merge"):
			return "refs/heads/main", nil
		case strings.Contains(joined, "ls-remote"):
			return testCommit + "\trefs/heads/main", nil
		}
		return "", errors.New("unexpected command")
	}
	return m, calls
}

func TestCheckAndDetachedLaunch(t *testing.T) {
	m, calls := testManager(t)
	s, err := m.Check(context.Background())
	if err != nil || s.LatestCommit != testCommit || !s.Available {
		t.Fatalf("check: %+v %v", s, err)
	}
	s, err = m.Start(context.Background(), testCommit)
	if err != nil || s.Job == nil || s.Job.State != "queued" {
		t.Fatalf("start: %+v %v", s, err)
	}
	var launch []string
	for _, call := range *calls {
		if call[0] == "systemd-run" {
			launch = call
		}
	}
	if len(launch) == 0 || strings.Join(launch[len(launch)-4:], "|") != "/bin with spaces/mcm|--update-worker|"+m.directory+"|/bin with spaces/mcmctl" {
		t.Fatalf("worker arguments: %q", launch)
	}
	if _, err := m.Start(context.Background(), testCommit); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate launch: %v", err)
	}
	// A replacement panel process sees the same running job.
	replacement := &Manager{configDir: m.configDir, directory: m.directory, run: m.run}
	if s, err := replacement.Status(context.Background()); err != nil || s.Job.State != "queued" {
		t.Fatalf("restart lost job: %+v %v", s, err)
	}
}

func TestStartRejectsChangedCommit(t *testing.T) {
	m, calls := testManager(t)
	if _, err := m.Start(context.Background(), strings.Repeat("f", 40)); !errors.Is(err, ErrConflict) {
		t.Fatalf("unexpected result: %v", err)
	}
	for _, call := range *calls {
		if call[0] == "systemd-run" {
			t.Fatal("started an unreviewed revision")
		}
	}
}

func TestCheckFailuresInvalidatePreviousCheck(t *testing.T) {
	for _, test := range []struct {
		name, match, output string
		failure             bool
	}{
		{"dirty source", "status", " M file", false},
		{"detached HEAD", "symbolic-ref", "", true},
		{"network failure", "ls-remote", "", true},
		{"malformed revision", "ls-remote", "oops\trefs/heads/main", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, _ := testManager(t)
			if _, err := m.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			run := m.run
			m.run = func(ctx context.Context, name string, args ...string) (string, error) {
				if strings.Contains(strings.Join(args, " "), test.match) {
					if test.failure {
						return test.output, errors.New("failed")
					}
					return test.output, nil
				}
				return run(ctx, name, args...)
			}
			if _, err := m.Check(context.Background()); err == nil {
				t.Fatal("accepted invalid source")
			}
			if s, _ := m.Status(context.Background()); s.Available || s.CheckedAt != nil {
				t.Fatal("kept stale check")
			}
		})
	}
}

func TestNoUpstreamUsesOriginBranch(t *testing.T) {
	m, calls := testManager(t)
	run := m.run
	m.run = func(ctx context.Context, name string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "config --get") {
			return "", errors.New("not configured")
		}
		return run(ctx, name, args...)
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := (*calls)[len(*calls)-1]
	if strings.Join(last[len(last)-3:], " ") != "-- origin refs/heads/main" {
		t.Fatalf("fallback: %q", last)
	}
}

func TestWorkerPersistsSuccessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			helper := filepath.Join(dir, "mcmctl")
			script := "#!/bin/sh\n[ \"$1\" = update ] || exit 2\necho build-progress\n"
			if fail {
				script += "echo rollback-details >&2\nexit 1\n"
			}
			if err := os.WriteFile(helper, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			err := RunWorker(dir, helper)
			if (err != nil) != fail {
				t.Fatalf("worker: %v", err)
			}
			m := &Manager{directory: dir}
			s, err := m.Status(context.Background())
			if err != nil || s.Job == nil || s.Job.FinishedAt == nil || !strings.Contains(s.Log, "build-progress") {
				t.Fatalf("result lost: %+v %v", s, err)
			}
			if fail && (s.Job.State != "failed" || !strings.Contains(s.Log, "rollback-details")) {
				t.Fatalf("failure hidden: %+v", s)
			}
			if !fail && s.Job.State != "succeeded" {
				t.Fatalf("success hidden: %+v", s)
			}
		})
	}
}

func TestInterruptedJobAndBoundedLog(t *testing.T) {
	m, _ := testManager(t)
	if err := os.MkdirAll(m.directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := saveJob(m.directory, Job{State: "running", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.directory, "update.log"), []byte(strings.Repeat("x", 40000)+"last line"), 0600); err != nil {
		t.Fatal(err)
	}
	m.run = func(context.Context, string, ...string) (string, error) { return "unknown", errors.New("unit removed") }
	s, err := m.Status(context.Background())
	if err != nil || s.Job.State != "failed" || len(s.Log) != 32768 || !strings.HasSuffix(s.Log, "last line") {
		t.Fatalf("interrupted job: %+v %v", s.Job, err)
	}
	var saved Job
	data, _ := os.ReadFile(filepath.Join(m.directory, "status.json"))
	if err := json.Unmarshal(data, &saved); err != nil || saved.State != "failed" {
		t.Fatal("did not persist interrupted result")
	}
}
