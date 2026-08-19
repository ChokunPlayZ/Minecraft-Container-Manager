package auth

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultSessionURL is the Mojang session server used to validate online-mode
// logins.
const DefaultSessionURL = "https://sessionserver.mojang.com/session/minecraft/hasJoined"

// SessionClient verifies a player against a session server. It is an interface
// so tests can substitute a fake endpoint.
type SessionClient interface {
	// HasJoined validates a player for online mode. serverID is the SHA-1
	// digest computed over (serverId + shared secret + public key), formatted
	// as a signed lowercase hex string.
	HasJoined(ctx context.Context, username, serverID string) (Profile, error)
}

// MojangClient is the production SessionClient backed by Mojang's session
// server. BaseURL may be overridden for tests.
type MojangClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewMojangClient returns a MojangClient with a reasonable HTTP timeout.
func NewMojangClient() *MojangClient {
	return &MojangClient{
		BaseURL: DefaultSessionURL,
		HTTP:    &http.Client{Timeout: 5 * time.Second},
	}
}

type hasJoinedResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// HasJoined validates a player and returns their profile.
func (c *MojangClient) HasJoined(ctx context.Context, username, serverID string) (Profile, error) {
	if c.BaseURL == "" {
		c.BaseURL = DefaultSessionURL
	}
	u := c.BaseURL + "?username=" + escape(username) + "&serverId=" + escape(serverID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Profile{}, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Profile{}, fmt.Errorf("session server request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return Profile{}, fmt.Errorf("player %q is not authenticated on this server", username)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Profile{}, fmt.Errorf("session server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var hr hasJoinedResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		return Profile{}, fmt.Errorf("decode session response: %w", err)
	}
	id, err := parseUUIDID(hr.ID)
	if err != nil {
		return Profile{}, err
	}
	name := hr.Name
	if name == "" {
		name = username
	}
	return Profile{Name: name, UUID: id, OnlineMode: true}, nil
}

func escape(s string) string {
	return strings.NewReplacer("+", "%2B", "&", "%26", "=", "%3D", " ", "%20").Replace(s)
}

func parseUUIDID(s string) (uuid.UUID, error) {
	// The session server returns a UUID without dashes.
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return uuid.Nil, fmt.Errorf("invalid session uuid %q", s)
	}
	formatted := s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
	return uuid.Parse(formatted)
}

// ServerHash computes the signed hex digest used as the hasJoined serverId.
// It follows the canonical Java algorithm: SHA-1 over the concatenation of the
// serverId string, the shared secret, and the RSA public key bytes, formatted
// as a signed (two's complement) lowercase hex number.
func ServerHash(serverID string, sharedSecret, publicKey []byte) string {
	h := sha1.New()
	h.Write([]byte(serverID))
	h.Write(sharedSecret)
	h.Write(publicKey)
	digest := h.Sum(nil)
	n := new(big.Int).SetBytes(digest)
	if digest[0]&0x80 != 0 {
		// Two's complement negative.
		bits := uint(8 * len(digest))
		mod := new(big.Int).Lsh(big.NewInt(1), bits)
		n.Sub(n, mod)
	}
	return n.Text(16)
}
