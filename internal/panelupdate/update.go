// Package panelupdate runs managed-install updates in a separate user service.
package panelupdate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/buildinfo"
)

var ErrConflict = errors.New("an update is already running or the checked revision changed; check again")

type Job struct {
	State      string     `json:"state"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Message    string     `json:"message"`
}

type Status struct {
	Build        buildinfo.Info `json:"build"`
	Supported    bool           `json:"supported"`
	Reason       string         `json:"reason,omitempty"`
	LatestCommit string         `json:"latest_commit,omitempty"`
	CheckedAt    *time.Time     `json:"checked_at,omitempty"`
	Available    bool           `json:"available"`
	Job          *Job           `json:"job,omitempty"`
	Log          string         `json:"log,omitempty"`
}

type Manager struct {
	mu                                       sync.Mutex
	executable, helper, directory, configDir string
	reason, latest                           string
	checked                                  *time.Time
	run                                      func(context.Context, string, ...string) (string, error)
}

func command(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=10")
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		var detail string
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		return strings.TrimSpace(string(output)), fmt.Errorf("%s: %w: %s", name, err, detail)
	}
	return strings.TrimSpace(string(output)), nil
}

func New() *Manager {
	m := &Manager{run: command}
	m.executable, _ = os.Executable()
	m.helper = filepath.Join(filepath.Dir(m.executable), "mcmctl")
	base, _ := os.UserConfigDir()
	m.configDir = filepath.Join(base, "mcm")
	m.directory = filepath.Join(m.configDir, "panel-update")
	switch {
	case runtime.GOOS != "linux":
		m.reason = "Web updates require a Linux installation managed by mcmctl. Rebuild and restart this deployment to update."
	case !regularFile(m.helper) || !regularFile(filepath.Join(m.configDir, "install.conf")):
		m.reason = "Web updates require an installation managed by mcmctl. For container deployments, rebuild the image and recreate the panel container."
	default:
		for _, name := range []string{"git", "systemd-run", "systemctl"} {
			if _, err := exec.LookPath(name); err != nil {
				m.reason = "Web updates require " + name + " on the panel host."
				break
			}
		}
	}
	return m
}

func regularFile(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.Mode().IsRegular()
}

func (m *Manager) status(ctx context.Context) (Status, error) {
	s := Status{Build: buildinfo.Current(), Supported: m.reason == "", Reason: m.reason, LatestCommit: m.latest, CheckedAt: m.checked}
	s.Available = m.latest != "" && (m.latest != s.Build.Commit || s.Build.Dirty)
	if m.reason != "" {
		return s, nil
	}
	data, err := os.ReadFile(filepath.Join(m.directory, "status.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s.Job); err != nil {
		return s, err
	}
	if s.Job != nil && (s.Job.State == "queued" || s.Job.State == "running") {
		active, err := m.run(ctx, "systemctl", "--user", "is-active", "mcm-update.service")
		if err != nil && active != "inactive" && active != "failed" && active != "unknown" {
			return s, err
		}
		if active != "active" && active != "activating" && active != "reloading" {
			// The worker may have saved its result after our first read.
			if latest, readErr := os.ReadFile(filepath.Join(m.directory, "status.json")); readErr == nil {
				if err := json.Unmarshal(latest, &s.Job); err != nil {
					return s, err
				}
			}
			if s.Job != nil && (s.Job.State == "queued" || s.Job.State == "running") {
				now := time.Now().UTC()
				s.Job.State, s.Job.FinishedAt, s.Job.Message = "failed", &now, "The update service stopped before reporting a result. Review the update log and panel service before retrying."
				if err := saveJob(m.directory, *s.Job); err != nil {
					return s, err
				}
			}
		}
	}
	if log, err := logTail(filepath.Join(m.directory, "update.log")); err == nil {
		s.Log = log
	}
	return s, nil
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status(ctx)
}

func (m *Manager) check(ctx context.Context) error {
	if m.reason != "" {
		return errors.New(m.reason)
	}
	m.latest, m.checked = "", nil
	data, err := os.ReadFile(filepath.Join(m.configDir, "install.conf"))
	if err != nil {
		return err
	}
	var source string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "SOURCE_DIR=") {
			source = strings.TrimPrefix(line, "SOURCE_DIR=")
		}
	}
	if !filepath.IsAbs(source) {
		return errors.New("the installed source checkout is missing; repair it with mcmctl update from the correct checkout")
	}
	git := func(args ...string) (string, error) {
		return m.run(ctx, "git", append([]string{"-C", source}, args...)...)
	}
	changes, err := git("status", "--porcelain")
	if err != nil {
		return err
	}
	if changes != "" {
		return errors.New("the source checkout has local changes; commit or stash them before updating")
	}
	branch, err := git("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return errors.New("the source checkout has a detached HEAD; check out a branch before updating")
	}
	remote, _ := git("config", "--get", "branch."+branch+".remote")
	ref, _ := git("config", "--get", "branch."+branch+".merge")
	if remote == "" || ref == "" {
		remote, ref = "origin", "refs/heads/"+branch
	}
	output, err := git("ls-remote", "--exit-code", "--", remote, ref)
	if err != nil {
		return fmt.Errorf("could not check the tracked branch: %w", err)
	}
	fields := strings.Fields(output)
	if len(fields) != 2 || (len(fields[0]) != 40 && len(fields[0]) != 64) || fields[1] != ref {
		return errors.New("the tracked branch did not return a commit")
	}
	for _, c := range fields[0] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return errors.New("invalid remote commit")
		}
	}
	now := time.Now().UTC()
	m.latest, m.checked = fields[0], &now
	return nil
}

func (m *Manager) Check(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.check(ctx); err != nil {
		return Status{}, err
	}
	return m.status(ctx)
}

func (m *Manager) Start(ctx context.Context, expected string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.status(ctx)
	if err != nil {
		return s, err
	}
	if !s.Supported {
		return s, errors.New(s.Reason)
	}
	if s.Job != nil && (s.Job.State == "queued" || s.Job.State == "running") {
		return s, ErrConflict
	}
	if expected == "" {
		return s, ErrConflict
	}
	if err := m.check(ctx); err != nil {
		return s, err
	}
	if m.latest != expected {
		return s, ErrConflict
	}
	if err := os.MkdirAll(m.directory, 0700); err != nil {
		return s, err
	}
	job := Job{State: "queued", StartedAt: time.Now().UTC(), Message: "Preparing the panel update."}
	if err := os.WriteFile(filepath.Join(m.directory, "update.log"), nil, 0600); err != nil {
		return s, err
	}
	if err := saveJob(m.directory, job); err != nil {
		return s, err
	}
	args := []string{"--user", "--collect", "--unit=mcm-update", "--property=Type=exec", "--setenv=MCM_BIN_DIR=" + filepath.Dir(m.executable)}
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		args = append(args, "--setenv=XDG_CONFIG_HOME="+base)
	}
	args = append(args, "--", m.executable, "--update-worker", m.directory, m.helper)
	if _, err := m.run(ctx, "systemd-run", args...); err != nil {
		now := time.Now().UTC()
		job.State, job.FinishedAt, job.Message = "failed", &now, "Could not start the update service: "+err.Error()
		_ = saveJob(m.directory, job)
		return s, err
	}
	return m.status(ctx)
}

func saveJob(directory string, job Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".status-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, "status.json"))
}

func logTail(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > 32768 {
		if _, err := file.Seek(-32768, io.SeekEnd); err != nil {
			return "", err
		}
	}
	data, err := io.ReadAll(io.LimitReader(file, 32768))
	return string(data), err
}

// RunWorker is invoked by systemd independently of the panel service, so stopping
// mcm.service during update does not kill the build or its result reporting.
func RunWorker(directory, helper string) error {
	job := Job{State: "running", StartedAt: time.Now().UTC(), Message: "Building the panel. The panel will restart when the build is ready."}
	if err := saveJob(directory, job); err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(directory, "update.log"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(helper, "update")
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=10")
	err = cmd.Run()
	now := time.Now().UTC()
	job.FinishedAt = &now
	job.State, job.Message = "succeeded", "Panel update complete. Reload the page to use the updated interface."
	if err != nil {
		job.State, job.Message = "failed", "The update failed. Review the log for build, readiness, or rollback details."
	}
	if saveErr := saveJob(directory, job); saveErr != nil {
		return saveErr
	}
	return err
}
