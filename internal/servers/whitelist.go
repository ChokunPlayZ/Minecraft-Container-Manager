package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WhitelistEntry is a single entry in a Minecraft server's whitelist.json file.
type WhitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// whitelistPath returns the path to a server's whitelist.json file.
func (s *Store) whitelistPath(id string) string {
	return filepath.Join(s.dataPath(id), "whitelist.json")
}

// readWhitelist loads the whitelist from whitelist.json, treating a missing
// file as an empty list.
func (s *Store) readWhitelist(id string) ([]WhitelistEntry, error) {
	data, err := os.ReadFile(s.whitelistPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []WhitelistEntry{}, nil
		}
		return nil, err
	}
	var entries []WhitelistEntry
	if len(data) == 0 {
		return []WhitelistEntry{}, nil
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse whitelist.json: %w", err)
	}
	return entries, nil
}

func (s *Store) writeWhitelist(id string, entries []WhitelistEntry) error {
	if entries == nil {
		entries = []WhitelistEntry{}
	}
	if err := os.MkdirAll(s.dataPath(id), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.whitelistPath(id), data, 0o644)
}

// ListWhitelist returns the whitelisted players configured for a server.
func (s *Store) ListWhitelist(ctx context.Context, id string) ([]WhitelistEntry, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.readWhitelist(id)
}

// AddWhitelist adds a player to the whitelist, persisting it to whitelist.json
// and, when the server is running, whitelisting the player live via RCON.
func (s *Store) AddWhitelist(ctx context.Context, id, name string) ([]WhitelistEntry, error) {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	entries, err := s.readWhitelist(id)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range entries {
		if equalFold(entries[i].Name, name) {
			entries[i].Name = name
			found = true
		}
	}
	if !found {
		entries = append(entries, WhitelistEntry{UUID: placeholderUUID, Name: name})
	}
	if err := s.writeWhitelist(id, entries); err != nil {
		return nil, err
	}
	if srv.State == StateRunning {
		if client, cerr := s.rconDial(ctx, id); cerr == nil && client != nil {
			defer client.Close()
			_, _ = client.Command("whitelist add " + name)
		}
	}
	return s.readWhitelist(id)
}

// RemoveWhitelist removes a player from the whitelist by name, persisting the
// change and, when the server is running, removing the entry live via RCON.
func (s *Store) RemoveWhitelist(ctx context.Context, id, name string) error {
	srv, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	entries, err := s.readWhitelist(id)
	if err != nil {
		return err
	}
	kept := entries[:0]
	for _, e := range entries {
		if !equalFold(e.Name, name) {
			kept = append(kept, e)
		}
	}
	if err := s.writeWhitelist(id, kept); err != nil {
		return err
	}
	if srv.State == StateRunning {
		if client, cerr := s.rconDial(ctx, id); cerr == nil && client != nil {
			defer client.Close()
			_, _ = client.Command("whitelist remove " + name)
		}
	}
	return nil
}
