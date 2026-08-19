// Package auth implements Minecraft player authentication for the gateway
// proxy: online mode via the Mojang session server and offline mode with a
// deterministic offline UUID. It never logs player session or auth tokens.
package auth

import "github.com/google/uuid"

// Profile is an authenticated player identity.
type Profile struct {
	// Name is the player's in-game name.
	Name string
	// UUID is the player's UUID (online-mode or offline-derived).
	UUID uuid.UUID
	// OnlineMode is true when this identity was verified against Mojang.
	OnlineMode bool
}

// OfflineUUID derives the stable UUID Mojang uses for a name in offline mode
// (a version-3 UUID over "OfflinePlayer:"+name).
func OfflineUUID(name string) uuid.UUID {
	return uuid.NewMD5(uuid.NameSpaceOID, []byte("OfflinePlayer:"+name))
}
