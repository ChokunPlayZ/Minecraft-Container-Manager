package servers

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/rcon"
)

// Player is a currently-connected player on a running server.
type Player struct {
	Name string `json:"name"`
}

// playerListSource reports where a player list was obtained.
type playerListSource string

const (
	playerSourceRCON    playerListSource = "rcon"
	playerSourceConsole playerListSource = "console"
)

// PlayerListResult is the outcome of querying a server for connected players.
type PlayerListResult struct {
	Players []Player `json:"players"`
	Source  string   `json:"source"`
}

// listRe matches the vanilla "list" command output:
// "There are 2 of a max 20 players online: Steve, Alex"
var listRe = regexp.MustCompile(`There are \d+ of a max \d+ players online:\s*(.*)`)

// joinRe matches a player join console line.
var joinRe = regexp.MustCompile(`([A-Za-z0-9_]{1,16}) joined the game`)

// leaveRe matches a player leave console line.
var leaveRe = regexp.MustCompile(`([A-Za-z0-9_]{1,16}) left the game`)

// dockerHost returns the address of the Docker daemon host.
func (s *Store) dockerHost() string {
	if s.docker == nil {
		return "127.0.0.1"
	}
	return s.docker.HostAddress()
}

// rconDial attempts to open an authenticated RCON connection to a server. It
// returns (nil, nil) when RCON is not enabled or no password is configured so
// callers can fall back to other sources.
func (s *Store) rconDial(ctx context.Context, id string) (*rcon.Client, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	cfg, err := s.readRCONConfig(id)
	if err != nil || !cfg.Enabled || cfg.Password == "" {
		return nil, nil
	}
	// The container only publishes its primary game port (srv.HostPort); RCON
	// must be reached through that same published port.
	addr := net.JoinHostPort(s.dockerHost(), strconv.Itoa(srv.HostPort))
	client, err := rcon.Dial(addr, cfg.Password, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect rcon: %w", err)
	}
	return client, nil
}

// PlayerList returns the currently connected players for a running server. It
// prefers querying via RCON and falls back to scanning the server's recent
// console log for join/leave lines when RCON is unavailable.
func (s *Store) PlayerList(ctx context.Context, id string) (PlayerListResult, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return PlayerListResult{}, err
	}

	// RCON path: query the live server.
	client, err := s.rconDial(ctx, id)
	if err == nil && client != nil {
		defer client.Close()
		out, cerr := client.Command("list")
		if cerr == nil {
			if names := parseListOutput(out); names != nil {
				return playerResult(names, playerSourceRCON), nil
			}
		}
	}

	// Fallback: scan the console log for currently-online players.
	if srv.ContainerID != "" && srv.State == StateRunning {
		names, cerr := s.playersFromConsole(ctx, srv)
		if cerr == nil && names != nil {
			return playerResult(names, playerSourceConsole), nil
		}
	}

	return PlayerListResult{Players: []Player{}, Source: string(playerSourceConsole)}, nil
}

func playerResult(names []string, source playerListSource) PlayerListResult {
	sort.Strings(names)
	players := make([]Player, 0, len(names))
	for _, n := range names {
		players = append(players, Player{Name: n})
	}
	return PlayerListResult{Players: players, Source: string(source)}
}

// parseListOutput extracts the comma-separated player names from a "list"
// command response. It trims server prefixes such as "[Server thread/INFO]".
func parseListOutput(out string) []string {
	// Some servers prefix the response with a log line; take the last match.
	var names []string
	for _, line := range strings.Split(out, "\n") {
		m := listRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		names = splitPlayers(m[1])
	}
	return names
}

func splitPlayers(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// playersFromConsole reads the full console log and tracks join/leave events to
// compute the set of players currently online.
func (s *Store) playersFromConsole(ctx context.Context, srv Server) ([]string, error) {
	rc, err := s.docker.Logs(ctx, srv.ContainerID, false)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	joined := map[string]bool{}
	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := leaveRe.FindStringSubmatch(line); m != nil {
			delete(joined, m[1])
			continue
		}
		if m := joinRe.FindStringSubmatch(line); m != nil {
			joined[m[1]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(joined))
	for name := range joined {
		out = append(out, name)
	}
	return out, nil
}
