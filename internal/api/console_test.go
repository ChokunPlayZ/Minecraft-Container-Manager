package api

import "testing"

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
