package api

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseConsoleLine(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		level   string
		message string
	}{
		{
			name:    "minecraft info",
			in:      "[Server thread/INFO]: Done (1.234s)! For help, type \"help\"",
			level:   "INFO",
			message: "Done (1.234s)! For help, type \"help\"",
		},
		{
			name:    "warn",
			in:      "[Server thread/WARN]: Can't keep up!",
			level:   "WARN",
			message: "Can't keep up!",
		},
		{
			name:    "error",
			in:      "[Server thread/ERROR]: Something broke",
			level:   "ERROR",
			message: "Something broke",
		},
		{
			name:    "carriage return stripped",
			in:      "[Server thread/INFO]: hello\r",
			level:   "INFO",
			message: "hello",
		},
		{
			name:    "plain line",
			in:      "Starting minecraft server version 1.20.1",
			level:   "",
			message: "Starting minecraft server version 1.20.1",
		},
		{
			name:    "empty message",
			in:      "[Server thread/INFO]:",
			level:   "INFO",
			message: "[Server thread/INFO]:",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := parseConsoleLine(tc.in)
			if ev.Level != tc.level {
				t.Errorf("level = %q, want %q", ev.Level, tc.level)
			}
			if ev.Message != tc.message {
				t.Errorf("message = %q, want %q", ev.Message, tc.message)
			}
		})
	}
}

func TestConsoleFrame(t *testing.T) {
	got, err := consoleFrame(3, consoleLine{Level: "INFO", Message: "hello"})
	if err != nil {
		t.Fatalf("consoleFrame: %v", err)
	}
	want := "id: 3\ndata: {\"timestamp\":\"\",\"level\":\"INFO\",\"message\":\"hello\"}\n\n"
	if string(got) != want {
		t.Errorf("frame = %q, want %q", got, want)
	}
}

func TestWriteConsoleSSE_LastEventID(t *testing.T) {
	input := "line 1\nline 2\nline 3\n"
	r := strings.NewReader(input)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	flushed := 0
	flush := func() { flushed++ }

	// Cancel context after short delay so test finishes
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Skip first 2 lines
	writeConsoleSSE(ctx, &buf, flush, r, 2)

	out := buf.String()
	if strings.Contains(out, "line 1") || strings.Contains(out, "line 2") {
		t.Errorf("expected line 1 and 2 to be skipped, got: %s", out)
	}
	if !strings.Contains(out, "id: 3\ndata: {\"timestamp\":\"\",\"message\":\"line 3\"}") {
		t.Errorf("expected line 3 to be delivered, got: %s", out)
	}
}

func TestWriteConsoleSSE_EOFKeepAlive(t *testing.T) {
	// After EOF, writeConsoleSSE should NOT exit immediately; it should wait for ctx.Done()
	input := "line 1\n"
	r := strings.NewReader(input)
	ctx, cancel := context.WithCancel(context.Background())

	var buf bytes.Buffer
	done := make(chan struct{})

	go func() {
		writeConsoleSSE(ctx, &buf, func() {}, r, 0)
		close(done)
	}()

	// Give it enough time to process EOF
	time.Sleep(50 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("writeConsoleSSE exited prematurely on EOF; should stay alive until ctx canceled")
	default:
		// Working as expected, still alive
	}

	cancel()
	select {
	case <-done:
		// Exited cleanly after cancel
	case <-time.After(200 * time.Millisecond):
		t.Fatal("writeConsoleSSE did not exit after ctx was canceled")
	}
}
