package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestForgeHeapUpdatesOnSecondLaunch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "user_jvm_args.txt"), []byte("-Dcustom=preserved\n-Xmx1024M\n-Xms256M\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Isolate only the actual launch portion; the shell, args rewrite and run.sh
	// invocation are production code, without Docker or a Java installation.
	start := strings.Index(DefaultEntryScript, "if [ -f \"/data/run.sh\" ]; then\n  if [ -f \"/data/user_jvm_args.txt\"")
	if start < 0 {
		t.Fatal("launch block missing")
	}
	script := strings.ReplaceAll(DefaultEntryScript[start:], "/data", dir)
	script = strings.ReplaceAll(script, `< "$FIFO"`, `< /dev/null`)
	for _, ram := range []int{2048, 4096} {
		command := exec.Command("sh", "-c", script)
		command.Env = append(os.Environ(), fmt.Sprintf("RAM_MB=%d", ram), "SERVER_TYPE=forge")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("launch: %v %s", err, output)
		}
		data, err := os.ReadFile(filepath.Join(dir, "user_jvm_args.txt"))
		if err != nil {
			t.Fatal(err)
		}
		args := string(data)
		if strings.Count(args, "-Xmx") != 1 || !strings.Contains(args, fmt.Sprintf("-Xmx%dM", ram)) || !strings.Contains(args, "-Dcustom=preserved") {
			t.Fatalf("stale or lost JVM options: %s", args)
		}
	}
}
