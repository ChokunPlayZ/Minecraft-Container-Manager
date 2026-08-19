package api

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// ceremonyTTL bounds how long a WebAuthn begin/finish ceremony may stay open.
const ceremonyTTL = 5 * time.Minute

// ceremonyStore holds in-memory WebAuthn ceremony sessions keyed by an opaque
// ID. It is only valid inside a single server process.
type ceremonyStore struct {
	mu    sync.Mutex
	items map[string]ceremonyEntry
}

type ceremonyEntry struct {
	session   webauthn.SessionData
	expiresAt time.Time
}

func newCeremonyStore() *ceremonyStore {
	return &ceremonyStore{items: make(map[string]ceremonyEntry)}
}

// save stores a ceremony session and returns its opaque ID.
func (c *ceremonyStore) save(session *webauthn.SessionData) string {
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	key := hex.EncodeToString(id)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.prune()
	c.items[key] = ceremonyEntry{session: *session, expiresAt: time.Now().Add(ceremonyTTL)}
	return key
}

// take removes and returns the ceremony session for id, if present and fresh.
func (c *ceremonyStore) take(id string) (webauthn.SessionData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(c.items, id)
	if time.Now().After(entry.expiresAt) {
		return webauthn.SessionData{}, false
	}
	return entry.session, true
}

func (c *ceremonyStore) prune() {
	now := time.Now()
	for id, entry := range c.items {
		if now.After(entry.expiresAt) {
			delete(c.items, id)
		}
	}
}
