package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type consoleExecResult struct {
	exitCode int
	stderr   string
}

// consoleDaemon serves Docker HTTP and exec upgrade requests over in-memory
// connections, so console delivery is exercised without a Docker daemon.
type consoleDaemon struct {
	user    string
	results []consoleExecResult
	execs   []container.ExecOptions
}

func (d *consoleDaemon) serve(conn net.Conn) {
	defer conn.Close()
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	defer req.Body.Close()
	var body any
	status := http.StatusOK
	switch {
	case strings.HasSuffix(req.URL.Path, "/containers/server/json"):
		body = map[string]any{"Id": "server", "Config": container.Config{
			Env: []string{"MCM_UID=1001", "MCM_GID=1002", "UID=2001", "GID=2002"},
		}}
	case strings.HasSuffix(req.URL.Path, "/containers/server/exec"):
		var opts container.ExecOptions
		if err := json.NewDecoder(req.Body).Decode(&opts); err != nil {
			return
		}
		d.execs = append(d.execs, opts)
		status = http.StatusCreated
		body = map[string]string{"Id": fmt.Sprintf("exec-%d", len(d.execs)-1)}
	default:
		parts := strings.Split(req.URL.Path, "/")
		index, err := strconv.Atoi(strings.TrimPrefix(parts[len(parts)-2], "exec-"))
		if err != nil || index < 0 || index >= len(d.results) {
			status = http.StatusInternalServerError
			body = map[string]string{"message": "unexpected console exec"}
			break
		}
		result := d.results[index]
		// Model protected_fifos: a write from another user fails even when
		// the FIFO exists. The first exec is always the direct pipe write.
		if index == 0 && d.execs[index].User != d.user {
			result = consoleExecResult{exitCode: 1, stderr: "cannot create /tmp/console.in: Permission denied"}
		}
		if strings.HasSuffix(req.URL.Path, "/start") {
			_, _ = io.Copy(io.Discard, req.Body)
			_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
			_, _ = io.WriteString(stdcopy.NewStdWriter(conn, stdcopy.Stderr), result.stderr)
			return
		}
		body = container.ExecInspect{ExitCode: result.exitCode}
	}
	_, _ = io.Copy(io.Discard, req.Body)
	data, _ := json.Marshal(body)
	resp := http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": {"application/json"},
			"Connection":   {"close"},
		},
		Body:          io.NopCloser(strings.NewReader(string(data))),
		ContentLength: int64(len(data)),
	}
	_ = resp.Write(conn)
}

func TestSendConsole(t *testing.T) {
	cases := []struct {
		name      string
		results   []consoleExecResult
		wantExecs []string
		wantErr   string
		disabled  bool
	}{
		{
			name:      "pipe runs as its owner",
			results:   []consoleExecResult{{}},
			wantExecs: []string{"sh"},
		},
		{
			name:      "missing pipe uses console helper",
			results:   []consoleExecResult{{exitCode: 66}, {}},
			wantExecs: []string{"sh", "mc-send-to-console"},
		},
		{
			name:      "missing console helper uses rcon",
			results:   []consoleExecResult{{exitCode: 66}, {exitCode: 127}, {}},
			wantExecs: []string{"sh", "mc-send-to-console", "rcon-cli"},
		},
		{
			name:      "no pipe or helpers requests recreation",
			results:   []consoleExecResult{{exitCode: 66}, {exitCode: 127}, {exitCode: 127}},
			wantExecs: []string{"sh", "mc-send-to-console", "rcon-cli"},
			disabled:  true,
		},
		{
			name:      "pipe permission errors are preserved",
			results:   []consoleExecResult{{exitCode: 1, stderr: "Permission denied"}},
			wantExecs: []string{"sh"},
			wantErr:   "Permission denied",
		},
		{
			name:      "helper errors are preserved",
			results:   []consoleExecResult{{exitCode: 66}, {exitCode: 1, stderr: "RCON authentication failed"}},
			wantExecs: []string{"sh", "mc-send-to-console"},
			wantErr:   "RCON authentication failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &consoleDaemon{user: "1001:1002", results: tc.results}
			cli, err := client.NewClientWithOpts(
				client.WithHost("tcp://fake"), client.WithVersion("1.44"),
				client.WithDialContext(func(context.Context, string, string) (net.Conn, error) {
					local, remote := net.Pipe()
					_ = local.SetDeadline(time.Now().Add(5 * time.Second))
					_ = remote.SetDeadline(time.Now().Add(5 * time.Second))
					go d.serve(remote)
					return local, nil
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cli.Close() })
			m := &Manager{client: cli}
			command := `say "hello"; $(literal) 'text'`
			err = m.SendConsole(context.Background(), "server", command)
			var gotExecs []string
			for _, opts := range d.execs {
				gotExecs = append(gotExecs, opts.Cmd[0])
				if opts.User != d.user {
					t.Errorf("exec user = %q, want %q", opts.User, d.user)
				}
				if opts.Cmd[len(opts.Cmd)-1] != command {
					t.Errorf("command was not passed as a literal argument: %v", opts.Cmd)
				}
			}
			if !reflect.DeepEqual(gotExecs, tc.wantExecs) {
				t.Fatalf("execs = %v, want %v", gotExecs, tc.wantExecs)
			}
			switch {
			case tc.disabled:
				if !errors.Is(err, ErrConsolePipeDisabled) {
					t.Fatalf("error = %v, want ErrConsolePipeDisabled", err)
				}
			case tc.wantErr != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			case err != nil:
				t.Fatal(err)
			}
		})
	}
}
