package servers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

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
// the command payload sane.
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
	Nbt     string `json:"nbt,omitempty"`     // give NBT / item components
}

// validateAndBuildGiveItem combines and validates item identifier and NBT/components.
func validateAndBuildGiveItem(item, nbt string) (string, error) {
	full := strings.TrimSpace(item)
	nbtTrim := strings.TrimSpace(nbt)
	if full == "" {
		return "", errors.New("missing item identifier")
	}
	if nbtTrim != "" && !strings.ContainsAny(full, "[{") {
		full += nbtTrim
	}

	if len(full) > 4096 {
		return "", errors.New("item specification exceeds maximum length of 4096 characters")
	}

	// Reject control characters (including newlines and carriage returns)
	for i := 0; i < len(full); i++ {
		b := full[i]
		if b < 0x20 || b == 0x7f {
			return "", errors.New("item specification contains invalid control characters")
		}
	}

	bracketIdx := strings.IndexAny(full, "[{")
	if bracketIdx == -1 {
		if !itemRe.MatchString(full) {
			return "", fmt.Errorf("invalid item %q", full)
		}
		return full, nil
	}

	baseItem := full[:bracketIdx]
	if !itemRe.MatchString(baseItem) {
		return "", fmt.Errorf("invalid base item identifier %q", baseItem)
	}

	tag := full[bracketIdx:]
	isBracket := strings.HasPrefix(tag, "[") && strings.HasSuffix(tag, "]")
	isBrace := strings.HasPrefix(tag, "{") && strings.HasSuffix(tag, "}")
	if !isBracket && !isBrace {
		return "", errors.New("NBT/components must be enclosed in [] or {}")
	}

	// Validate balanced brackets and quotes
	var square, curly int
	var inQuote rune
	var escape bool

	for _, r := range tag {
		if escape {
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		if inQuote != 0 {
			if r == inQuote {
				inQuote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = r
			continue
		}
		switch r {
		case '[':
			square++
		case ']':
			square--
		case '{':
			curly++
		case '}':
			curly--
		}
		if square < 0 || curly < 0 {
			return "", errors.New("mismatched brackets or braces in NBT/components")
		}
	}

	if square != 0 || curly != 0 || inQuote != 0 {
		return "", errors.New("unclosed bracket, brace, or quote in NBT/components")
	}

	return full, nil
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
		fullItem, err := validateAndBuildGiveItem(args.Item, args.Nbt)
		if err != nil {
			return "", err
		}
		if args.Amount < 1 {
			args.Amount = 1
		}
		return fmt.Sprintf("give %s %s %d", name, fullItem, args.Amount), nil
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

// RunPlayerCommand executes a command against a running server on behalf of a
// player. It prefers RCON when the server exposes it (returning the response
// text) and otherwise falls back to the console stdin pipe, so the quick player
// menu works without RCON just like the main console. The caller is
// responsible for building a safe command line (see BuildPlayerCommand).
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

	// Prefer RCON: it is the only path that returns the server's response.
	// If RCON is disabled or unreachable, fall back to the console pipe so the
	// command still runs without RCON.
	if client, derr := s.rconDial(ctx, id); derr == nil && client != nil {
		defer client.Close()
		if out, cerr := client.Command(commandLine); cerr == nil {
			return strings.TrimSpace(out), nil
		}
	}

	if serr := s.SendConsoleCommand(ctx, id, commandLine); serr != nil {
		return "", serr
	}
	return "", nil
}

// consoleCommandRe keeps interactive console input to characters Minecraft
// accepts as a server command line, rejecting control/non-printable text that
// could corrupt the server stdin or the Docker exec payload.
var consoleCommandRe = regexp.MustCompile(`^[^\r\n\x00]+$`)

// SendConsoleCommand sends a single command to a running server's console via
// the container's stdin (no RCON required). The command must be a single line
// of printable text.
func (s *Store) SendConsoleCommand(ctx context.Context, id, command string) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if srv.State != StateRunning {
		return ErrServerNotRunning
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("empty command")
	}
	if !consoleCommandRe.MatchString(command) {
		return errors.New("command contains unsupported characters")
	}
	if srv.ContainerID == "" {
		return errors.New("server has no container")
	}
	if err := s.docker.SendConsole(ctx, srv.ContainerID, command); err != nil {
		return err
	}
	return nil
}
