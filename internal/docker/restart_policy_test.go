package docker

import (
	"strings"
	"testing"
)

func TestEntryScriptSignalHandlingAndCleanExit(t *testing.T) {
	entryScript := `FIFO="/tmp/console.in"
rm -f "$FIFO"
mkfifo -m 666 "$FIFO"
exec 3<> "$FIFO"

term_handler() {
  test -p "$FIFO" && printf '%s\n' "stop" "end" "shutdown" > "$FIFO" || true
  wait "$SERVER_PID" 2>/dev/null || true
  exit 0
}
trap term_handler TERM INT

if [ -f "/data/run.sh" ]; then
  sh /data/run.sh nogui < "$FIFO" &
elif [ -f "/data/server.jar" ]; then
  java -Xms512M -Xmx${RAM_MB:-2048}M ${JVM_OPTS} -jar /data/server.jar nogui < "$FIFO" &
else
  echo "No server.jar or run.sh found in /data"
  exit 1
fi
SERVER_PID=$!
wait "$SERVER_PID"
EXIT_CODE=$?
exit $EXIT_CODE
`

	if !strings.Contains(entryScript, "trap term_handler TERM INT") {
		t.Errorf("entryScript missing trap term_handler")
	}
	if !strings.Contains(entryScript, `printf '%s\n' "stop"`) {
		t.Errorf("entryScript missing stop command forwarding")
	}
	if !strings.Contains(entryScript, `wait "$SERVER_PID"`) {
		t.Errorf("entryScript missing wait for SERVER_PID")
	}
	if !strings.Contains(entryScript, `exit $EXIT_CODE`) {
		t.Errorf("entryScript missing exit code propagation")
	}
}
