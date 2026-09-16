package docker

import (
	"strings"
	"testing"
)

func TestEntryScriptSignalHandlingAndCleanExit(t *testing.T) {
	if !strings.Contains(DefaultEntryScript, "trap term_handler TERM INT") {
		t.Errorf("DefaultEntryScript missing trap term_handler")
	}
	if !strings.Contains(DefaultEntryScript, `printf '%s\n' "stop"`) {
		t.Errorf("DefaultEntryScript missing stop command forwarding")
	}
	if !strings.Contains(DefaultEntryScript, `wait "$SERVER_PID"`) {
		t.Errorf("DefaultEntryScript missing wait for SERVER_PID")
	}
	if !strings.Contains(DefaultEntryScript, `exit $EXIT_CODE`) {
		t.Errorf("DefaultEntryScript missing exit code propagation")
	}
}

func TestEntryScriptForgeInstallerHandling(t *testing.T) {
	// Should check for installer if neither run.sh nor server.jar exists
	if !strings.Contains(DefaultEntryScript, `if [ ! -f "/data/run.sh" ] && [ ! -f "/data/server.jar" ]; then`) {
		t.Errorf("DefaultEntryScript missing check for uninstalled server")
	}
	// Should support installer.jar or *installer*.jar
	if !strings.Contains(DefaultEntryScript, `INSTALLER="/data/installer.jar"`) {
		t.Errorf("DefaultEntryScript missing check for /data/installer.jar")
	}
	if !strings.Contains(DefaultEntryScript, `*installer*.jar`) {
		t.Errorf("DefaultEntryScript missing glob check for installer jars")
	}
	// Should run installer with --installServer
	if !strings.Contains(DefaultEntryScript, `java -jar "$INSTALLER" --installServer`) {
		t.Errorf("DefaultEntryScript missing java -jar $INSTALLER --installServer")
	}
	// Should check installer exit code
	if !strings.Contains(DefaultEntryScript, `INSTALL_EXIT=$?`) {
		t.Errorf("DefaultEntryScript missing check for installer exit code")
	}
	// For older Forge (<= 1.16.5), should find forge-*.jar and link to server.jar
	if !strings.Contains(DefaultEntryScript, `ln -sf "$f" /data/server.jar`) {
		t.Errorf("DefaultEntryScript missing symlink from forge-*.jar to server.jar")
	}
	// For modern Forge, should execute run.sh
	if !strings.Contains(DefaultEntryScript, `sh /data/run.sh nogui < "$FIFO" &`) {
		t.Errorf("DefaultEntryScript missing execution of run.sh")
	}
	// For modern Forge, should configure user_jvm_args.txt if present
	if !strings.Contains(DefaultEntryScript, `/data/user_jvm_args.txt`) {
		t.Errorf("DefaultEntryScript missing user_jvm_args.txt memory config")
	}
}
