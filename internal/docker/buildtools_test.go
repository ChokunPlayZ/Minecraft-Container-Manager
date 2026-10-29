package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildToolsChildJVMEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name       string
		serverType string
		options    string
		buildExit  int
	}{
		{name: "spigot", serverType: "spigot"},
		{name: "bukkit preserves options", serverType: "bukkit", options: "-Dcustom=preserved"},
		{name: "craftbukkit", serverType: "craftbukkit"},
		{name: "failed build", serverType: "spigot", buildExit: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binDir := filepath.Join(dir, "bin")
			dataDir := filepath.Join(dir, "data")
			buildDir := filepath.Join(dir, "buildtools")
			for _, d := range []string{binDir, dataDir} {
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, content string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(path, []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(dataDir, "installer.jar"), "buildtools", 0o644)
			for _, name := range []string{"git", "javac"} {
				write(filepath.Join(binDir, name), "#!/bin/sh\nexit 0\n", 0o755)
			}
			// Maven is a separate process launched by BuildTools. Capture the
			// inherited JVM options there, then simulate a built server jar.
			write(filepath.Join(binDir, "mvn"), `#!/bin/sh
printf '%s' "${JAVA_TOOL_OPTIONS:-}" > "$TEST_BUILD_ENV"
exit "$TEST_BUILD_EXIT"
`, 0o755)
			write(filepath.Join(binDir, "java"), `#!/bin/sh
case " $* " in
  *" --rev "*)
    sh -c 'mvn'
    result=$?
    [ "$result" -eq 0 ] || exit "$result"
    printf 'built' > "$TEST_DATA/spigot-test.jar"
    ;;
  *) printf '%s' "${JAVA_TOOL_OPTIONS:-}" > "$TEST_SERVER_ENV" ;;
esac
`, 0o755)
			script := strings.NewReplacer(
				"/data", dataDir,
				"/tmp/buildtools", buildDir,
				"/tmp/console.in", filepath.Join(dir, "console.in"),
			).Replace(DefaultEntryScript)
			cmd := exec.Command("/bin/sh", "-c", script)
			cmd.Env = append(os.Environ(),
				"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SERVER_TYPE="+tc.serverType, "SERVER_VERSION=26.3",
				"JAVA_TOOL_OPTIONS="+tc.options, "TEST_DATA="+dataDir,
				"TEST_BUILD_ENV="+filepath.Join(dir, "build-env"),
				"TEST_SERVER_ENV="+filepath.Join(dir, "server-env"),
				fmt.Sprintf("TEST_BUILD_EXIT=%d", tc.buildExit),
			)
			output, err := cmd.CombinedOutput()
			if tc.buildExit == 0 && err != nil {
				t.Fatalf("entrypoint: %v\n%s", err, output)
			}
			if tc.buildExit != 0 {
				if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != tc.buildExit {
					t.Fatalf("entrypoint exit = %v, want %d\n%s", err, tc.buildExit, output)
				}
			}
			options, err := os.ReadFile(filepath.Join(dir, "build-env"))
			if err != nil {
				t.Fatal(err)
			}
			want := "-Duser.home=" + buildDir + " -Dmaven.repo.local=" + filepath.Join(buildDir, ".m2", "repository")
			if tc.options != "" {
				want = tc.options + " " + want
			}
			if string(options) != want {
				t.Fatalf("Maven JVM options = %q, want %q", options, want)
			}
			for _, path := range []string{buildDir, filepath.Join(dataDir, ".mcm_state")} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("temporary build state still exists: %s (%v)", path, err)
				}
			}
			serverOptions, err := os.ReadFile(filepath.Join(dir, "server-env"))
			if tc.buildExit != 0 {
				if !os.IsNotExist(err) {
					t.Fatalf("server must not launch after a failed build: %v", err)
				}
				return
			}
			if err != nil || string(serverOptions) != tc.options {
				t.Fatalf("server JVM options = %q, want %q (err: %v)", serverOptions, tc.options, err)
			}
			if _, err := os.Stat(filepath.Join(dataDir, "server.jar")); err != nil {
				t.Fatalf("built server jar missing: %v", err)
			}
		})
	}
}
