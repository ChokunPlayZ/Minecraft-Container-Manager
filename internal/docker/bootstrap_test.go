package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestServerBootstrap(t *testing.T) {
	cases := []struct {
		name       string
		currentUID string
		targetUID  string
		serverType string
		switcher   string
		manager    string
		chownFails bool
		want       string
		wantErr    bool
	}{
		{
			name: "drop privileges and repair old files", currentUID: "0", targetUID: "1001", switcher: "su-exec",
			want: "chown -R 1001:1002 /data\nsu-exec 1001:1002\nserver uid=1001 home=/data\n",
		},
		{
			name: "install Alpine switcher and BuildTools git", currentUID: "0", targetUID: "1001", manager: "apk", serverType: "spigot",
			want: "apk add --no-cache su-exec git\nchown -R 1001:1002 /data\nsu-exec 1001:1002\nserver uid=1001 home=/data\n",
		},
		{
			name: "install Debian switcher", currentUID: "0", targetUID: "1001", manager: "apt-get",
			want: "apt-get update\napt-get install -y --no-install-recommends gosu\nchown -R 1001:1002 /data\ngosu 1001:1002\nserver uid=1001 home=/data\n",
		},
		{
			name: "already non root", currentUID: "1001", targetUID: "1001",
			want: "server uid=1001 home=/original-home\n",
		},
		{
			name: "explicit root panel", currentUID: "0", targetUID: "0",
			want: "server uid=0 home=/original-home\n",
		},
		{
			name: "ownership repair must succeed", currentUID: "0", targetUID: "1001", switcher: "su-exec", chownFails: true,
			want: "chown -R 1001:1002 /data\n", wantErr: true,
		},
		{
			name: "cannot silently run as root", currentUID: "0", targetUID: "1001",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			logPath := filepath.Join(dir, "calls")
			writeTool := func(name, script string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nset -eu\n"+script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("/bin/sh", filepath.Join(dir, "sh")); err != nil {
				t.Fatal(err)
			}
			writeTool("id", `printf '%s\n' "$TEST_UID"`)
			chown := `printf 'chown %s\n' "$*" >> "$TEST_LOG"`
			if tc.chownFails {
				chown += "\nexit 1"
			}
			writeTool("chown", chown)
			switchScript := `printf '%s %s\n' "${0##*/}" "$1" >> "$TEST_LOG"
export TEST_UID="${1%%:*}"
shift
exec "$@"`
			if tc.switcher != "" {
				writeTool(tc.switcher, switchScript)
			}
			// Package installation stubs make the requested privilege switcher
			// available, without touching the host or accessing the network.
			writeTool("switcher-template", switchScript)
			if tc.manager != "" {
				writeTool(tc.manager, `printf '%s %s\n' "${0##*/}" "$*" >> "$TEST_LOG"
case "$*" in
  *install*|*add*)
    /bin/cp "$TEST_BIN/switcher-template" "$TEST_BIN/su-exec"
    /bin/cp "$TEST_BIN/switcher-template" "$TEST_BIN/gosu"
    ;;
esac`)
			}
			mainScript := `printf 'server uid=%s home=%s\n' "$(id -u)" "$HOME" >> "$TEST_LOG"`
			cmd := exec.Command("/bin/sh", "-c", serverBootstrapScript, "mcm-entrypoint", mainScript)
			cmd.Env = []string{
				"PATH=" + dir, "HOME=/original-home", "TEST_BIN=" + dir, "TEST_LOG=" + logPath,
				"TEST_UID=" + tc.currentUID, "MCM_UID=" + tc.targetUID, "MCM_GID=1002", "SERVER_TYPE=" + tc.serverType,
			}
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v, output: %s", err, tc.wantErr, out)
			}
			calls, err := os.ReadFile(logPath)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if string(calls) != tc.want {
				t.Fatalf("calls:\n%s\nwant:\n%s\noutput: %s", calls, tc.want, out)
			}
		})
	}
}
