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
	// Should run installer with --installServer in headless mode
	if !strings.Contains(DefaultEntryScript, `java -Djava.awt.headless=true -jar "$INSTALLER" --installServer`) {
		t.Errorf("DefaultEntryScript missing headless java -jar $INSTALLER --installServer")
	}
	// Should check installer exit code
	if !strings.Contains(DefaultEntryScript, `INSTALL_EXIT=$?`) {
		t.Errorf("DefaultEntryScript missing check for installer exit code")
	}
	// For older Forge (<= 1.16.5), should find forge-*.jar and link to server.jar
	if !strings.Contains(DefaultEntryScript, `ln -sf "$f" /data/server.jar`) {
		t.Errorf("DefaultEntryScript missing symlink from forge-*.jar to server.jar")
	}
	// For modern Forge, should execute run.sh with --nogui
	if !strings.Contains(DefaultEntryScript, `sh /data/run.sh --nogui nogui < "$FIFO" &`) {
		t.Errorf("DefaultEntryScript missing execution of run.sh with --nogui")
	}
	// For modern Forge, should configure user_jvm_args.txt if present
	if !strings.Contains(DefaultEntryScript, `/data/user_jvm_args.txt`) || !strings.Contains(DefaultEntryScript, `-Djava.awt.headless=true`) {
		t.Errorf("DefaultEntryScript missing user_jvm_args.txt headless/memory config")
	}
}

func TestEntryScriptProxyAndVelocityHandling(t *testing.T) {
	// Velocity must not receive --nogui and should specify -p with server port
	if !strings.Contains(DefaultEntryScript, `velocity)`) {
		t.Errorf("DefaultEntryScript missing case for velocity")
	}
	if !strings.Contains(DefaultEntryScript, `SERVER_ARGS="-p ${SERVER_PORT:-25577}"`) {
		t.Errorf("DefaultEntryScript missing velocity port argument")
	}
	// Other proxies and lightweight servers should have empty args
	if !strings.Contains(DefaultEntryScript, `waterfall|bungeecord|limbo|nanolimbo|geysermc)`) {
		t.Errorf("DefaultEntryScript missing proxy and lightweight servers case")
	}
}

func TestEntryScriptQuiltInstallerHandling(t *testing.T) {
	if !strings.Contains(DefaultEntryScript, `*quilt*)`) {
		t.Errorf("DefaultEntryScript missing quilt installer case")
	}
	if !strings.Contains(DefaultEntryScript, `install server`) {
		t.Errorf("DefaultEntryScript missing quilt install server argument")
	}
	if !strings.Contains(DefaultEntryScript, `quilt-server-launch.jar`) {
		t.Errorf("DefaultEntryScript missing quilt-server-launch.jar check")
	}
}

