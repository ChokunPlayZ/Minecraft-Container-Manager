package servers

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/mcm-panel/mcm/internal/storage"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var playerNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`)

func (s *Store) playerUUID(ctx context.Context, id, name string) (string, error) {
	if !playerNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid Minecraft player name")
	}
	root, err := s.openDataRoot(id)
	if err != nil {
		return "", err
	}
	defer root.Close()
	content := ""
	if f, readErr := storage.OpenRegular(root, "server.properties"); readErr == nil {
		data, e := io.ReadAll(io.LimitReader(f, maxEditBytes+1))
		f.Close()
		if e != nil {
			return "", e
		}
		if int64(len(data)) > maxEditBytes {
			return "", ErrReadTooLarge
		}
		content = string(data)
	} else if !os.IsNotExist(readErr) {
		return "", readErr
	}
	online := true
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "online-mode" {
			online = !strings.EqualFold(strings.TrimSpace(value), "false")
		}
	}
	if !online {
		// Java UUID.nameUUIDFromBytes uses raw MD5, without a UUID namespace prefix.
		sum := md5.Sum([]byte("OfflinePlayer:" + name))
		sum[6] = (sum[6] & 0x0f) | 0x30
		sum[8] = (sum[8] & 0x3f) | 0x80
		return uuid.UUID(sum).String(), nil
	}
	// A cache entry created by the server preserves the player's canonical identity.
	var cached []struct {
		Name string `json:"name"`
		UUID string `json:"uuid"`
	}
	if f, readErr := storage.OpenRegular(root, "usercache.json"); readErr == nil {
		data, e := io.ReadAll(io.LimitReader(f, maxEditBytes+1))
		f.Close()
		if e != nil {
			return "", e
		}
		if int64(len(data)) > maxEditBytes {
			return "", ErrReadTooLarge
		}
		if err = json.Unmarshal(data, &cached); err != nil {
			return "", err
		}
		for _, player := range cached {
			if strings.EqualFold(player.Name, name) {
				if parsed, e := uuid.Parse(player.UUID); e == nil && parsed != uuid.Nil {
					return parsed.String(), nil
				}
			}
		}
	} else if !os.IsNotExist(readErr) {
		return "", readErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.mojang.com/users/profiles/minecraft/"+name, nil)
	if err != nil {
		return "", err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("resolve online player identity: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("online player %q could not be resolved", name)
	}
	var profile struct {
		ID string `json:"id"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&profile); err != nil {
		return "", err
	}
	parsed, err := uuid.Parse(profile.ID)
	if err != nil || parsed == uuid.Nil {
		return "", fmt.Errorf("invalid player identity response")
	}
	return parsed.String(), nil
}
