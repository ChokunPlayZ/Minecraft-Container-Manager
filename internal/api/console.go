package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
)

// consoleLine is one parsed log line streamed to the web console as an SSE event.
type consoleLine struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level,omitempty"`
	Message   string `json:"message"`
}

var consoleLevelRe = regexp.MustCompile(`\[[^\]]*/(INFO|WARN|ERROR)\]`)

// parseConsoleLine splits a raw log line into its level and message. Minecraft
// style lines carry a `[Server thread/INFO]`-like prefix; other lines are kept
// verbatim with an empty level. A trailing carriage return is stripped.
func parseConsoleLine(line string) consoleLine {
	line = strings.TrimRight(line, "\r")
	ev := consoleLine{Message: line}
	if loc := consoleLevelRe.FindStringSubmatchIndex(line); loc != nil {
		ev.Level = line[loc[2]:loc[3]]
		rest := strings.TrimSpace(line[loc[1]:])
		rest = strings.TrimPrefix(rest, ":")
		rest = strings.TrimSpace(rest)
		if rest != "" {
			ev.Message = rest
		}
	}
	return ev
}

// consoleFrame renders a single SSE event carrying the parsed line.
func consoleFrame(id int, ev consoleLine) ([]byte, error) {
	data, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("id: %d\ndata: %s\n\n", id, data)), nil
}

// streamConsole demultiplexes the Docker stdcopy stream into plain lines and
// emits them as SSE events until the underlying reader or context closes.
func (s *Server) streamConsole(ctx context.Context, w http.ResponseWriter, rc io.ReadCloser) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Demultiplex the Docker combined stream into plain text on a pipe.
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		_, _ = stdcopy.StdCopy(pw, pw, rc)
	}()
	defer pr.Close()
	go func() {
		<-ctx.Done()
		_ = pr.Close()
	}()

	writeConsoleSSE(ctx, w, flusher.Flush, pr)
}

// writeConsoleSSE reads text lines from r and writes them as SSE `data:` events.
// It flushes after every event and sends a keep-alive comment while idle. Reads
// stop as soon as ctx is canceled (e.g. client disconnect).
func writeConsoleSSE(ctx context.Context, w io.Writer, flush func(), r io.Reader) {
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64*1024), 1<<20)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	evID := 0
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			ev := parseConsoleLine(line)
			if ev.Message == "" {
				continue
			}
			evID++
			frame, err := consoleFrame(evID, ev)
			if err != nil {
				continue
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flush()
		}
	}
}
