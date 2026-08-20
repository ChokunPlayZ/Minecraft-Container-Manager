package servers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrRCONDisabled is returned when a player command cannot run because the
// server does not have RCON enabled.
var ErrRCONDisabled = errors.New("rcon is not enabled")

// ErrServerNotRunning is returned when a player command is attempted while the
// server is not running.
var ErrServerNotRunning = errors.New("server is not running")

// playerNameRe limits player names to the characters Minecraft accepts (max 16).
var playerNameRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)

// itemRe limits item identifiers to alphanumerics, underscore, colon, dash, and
// dot (e.g. minecraft:diamond or diamond_sword).
var itemRe = regexp.MustCompile(`^[A-Za-z0-9_:.-]{1,64}$`)

// customCommandRe is deliberately loose: a custom command is free-form server
// text, but we still reject control characters and shell metacharacters to keep
// the RCON payload sane.
var customCommandRe = regexp.MustCompile(`^[A-Za-z0-9_ .:/@%+~=,-]{1,256}$`)

// validGamemodes are the named gamemode selectors Minecraft accepts.
var validGamemodes = map[string]bool{
	"survival":  true,
	"creative":  true,
	"adventure": true,
	"spectator": true,
	"0":         true,
	"1":         true,
	"2":         true,
	"3":         true,
}

// PlayerCommandArgs carries optional parameters for a player command action.
type PlayerCommandArgs struct {
	Reason  string `json:"reason,omitempty"`
	Target  string `json:"target,omitempty"`  // another player name (tp/pardon)
	Item    string `json:"item,omitempty"`    // give
	Amount  int    `json:"amount,omitempty"`  // give
	Mode    string `json:"mode,omitempty"`    // gamemode
	Command string `json:"command,omitempty"` // custom free-form
}

// BuildPlayerCommand validates an action and turns it (plus the target player
// name and params) into the exact RCON SERVERDATA command string. It returns an
// error for unknown actions, unsafe player names, or invalid parameters.
func BuildPlayerCommand(name, action string, args PlayerCommandArgs) (string, error) {
	if !playerNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid player name %q", name)
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "kick":
		return joinCommand("kick", name, args.Reason), nil
	case "ban":
		return joinCommand("ban", name, args.Reason), nil
	case "pardon":
		if !playerNameRe.MatchString(args.Target) {
			return "", fmt.Errorf("invalid target player name %q", args.Target)
		}
		return "pardon " + args.Target, nil
	case "op":
		return "op " + name, nil
	case "deop":
		return "deop " + name, nil
	case "give":
		if !itemRe.MatchString(args.Item) {
			return "", fmt.Errorf("invalid item %q", args.Item)
		}
		if args.Amount < 1 {
			args.Amount = 1
		}
		return fmt.Sprintf("give %s %s %d", name, args.Item, args.Amount), nil
	case "gamemode":
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if !validGamemodes[mode] {
			return "", fmt.Errorf("invalid gamemode %q", args.Mode)
		}
		return "gamemode " + mode + " " + name, nil
	case "tp":
		if !playerNameRe.MatchString(args.Target) {
			return "", fmt.Errorf("invalid target player name %q", args.Target)
		}
		return "tp " + name + " " + args.Target, nil
	case "kill":
		return "kill " + name, nil
	case "custom":
		s := strings.TrimSpace(args.Command)
		if s == "" {
			return "", errors.New("custom command is empty")
		}
		if !customCommandRe.MatchString(s) {
			return "", errors.New("custom command contains unsupported characters")
		}
		return s, nil
	default:
		return "", fmt.Errorf("unsupported action %q", action)
	}
}

// joinCommand builds "verb name [optional]" skipping empty trailing segments.
func joinCommand(verb, name, optional string) string {
	if strings.TrimSpace(optional) == "" {
		return verb + " " + name
	}
	return verb + " " + name + " " + optional
}

// RunPlayerCommand executes an RCON SERVERDATA command against a running server.
// It returns the server's response text. The caller is responsible for building
// a safe command line (see BuildPlayerCommand).
func (s *Store) RunPlayerCommand(ctx context.Context, id, name, commandLine string) (string, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if srv.State != StateRunning {
		return "", ErrServerNotRunning
	}
	if !playerNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid player name %q", name)
	}
	if strings.TrimSpace(commandLine) == "" {
		return "", errors.New("empty command")
	}
	client, err := s.rconDial(ctx, id)
	if err != nil {
		return "", err
	}
	if client == nil {
		return "", ErrRCONDisabled
	}
	defer client.Close()
	out, cerr := client.Command(commandLine)
	if cerr != nil {
		return "", cerr
	}
	return strings.TrimSpace(out), nil
}
