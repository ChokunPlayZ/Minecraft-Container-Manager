package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// OP is a single entry in a Minecraft server's ops.json file.
type OP struct {
	UUID                string `json:"uuid"`
	Name                string `json:"name"`
	Level               int    `json:"level"`
	BypassesPlayerLimit bool   `json:"bypassesPlayerLimit"`
}

// placeholderUUID is used when an operator has not yet logged in and the server
// has not assigned a real UUID.
const placeholderUUID = "00000000-0000-0000-0000-000000000000"

// opsPath returns the path to a server's ops.json file.
func (s *Store) opsPath(id string) string {
	return filepath.Join(s.dataPath(id), "ops.json")
}

// readOps loads the operators list from ops.json, treating a missing file as an
// empty list.
func (s *Store) readOps(id string) ([]OP, error) {
	data, err := os.ReadFile(s.opsPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []OP{}, nil
		}
		return nil, err
	}
	var ops []OP
	if len(data) == 0 {
		return []OP{}, nil
	}
	if err := json.Unmarshal(data, &ops); err != nil {
		return nil, fmt.Errorf("parse ops.json: %w", err)
	}
	return ops, nil
}

func (s *Store) writeOps(id string, ops []OP) error {
	if ops == nil {
		ops = []OP{}
	}
	if err := os.MkdirAll(s.dataPath(id), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(ops, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.opsPath(id), data, 0o644)
}

// ListOps returns the operators configured for a server.
func (s *Store) ListOps(ctx context.Context, id string) ([]OP, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.readOps(id)
}

// AddOP adds a player to the operator list, persisting it to ops.json and, when
// the server is running, promoting the player live via RCON.
func (s *Store) AddOP(ctx context.Context, id, name string, level int) ([]OP, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	ops, err := s.readOps(id)
	if err != nil {
		return nil, err
	}
	if level <= 0 {
		level = 4
	}
	found := false
	for i := range ops {
		if equalFold(ops[i].Name, name) || ops[i].UUID == placeholderUUID {
			ops[i].Name = name
			ops[i].Level = level
			found = true
		}
	}
	if !found {
		ops = append(ops, OP{UUID: placeholderUUID, Name: name, Level: level})
	}
	if err := s.writeOps(id, ops); err != nil {
		return nil, err
	}
	if srv.State == StateRunning {
		if client, cerr := s.rconDial(ctx, id); cerr == nil && client != nil {
			defer client.Close()
			_, _ = client.Command("op " + name)
		}
	}
	return s.readOps(id)
}

// RemoveOP removes a player from the operator list by name, persisting the
// change and, when the server is running, revoking the role live via RCON.
func (s *Store) RemoveOP(ctx context.Context, id, name string) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	ops, err := s.readOps(id)
	if err != nil {
		return err
	}
	kept := ops[:0]
	for _, o := range ops {
		if !equalFold(o.Name, name) {
			kept = append(kept, o)
		}
	}
	if err := s.writeOps(id, kept); err != nil {
		return err
	}
	if srv.State == StateRunning {
		if client, cerr := s.rconDial(ctx, id); cerr == nil && client != nil {
			defer client.Close()
			_, _ = client.Command("deop " + name)
		}
	}
	return nil
}

func equalFold(a, b string) bool {
	return len(a) == len(b) && lowerASCII(a) == lowerASCII(b)
}

func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}
