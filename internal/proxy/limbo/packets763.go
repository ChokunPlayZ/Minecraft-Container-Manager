package limbo

// This file centralizes the protocol packet IDs and payload builders used by
// the limbo for protocol 763 (Minecraft 1.19.3 / 1.20.1). Packet IDs are the
// primary external dependency of this feature and must be validated against a
// live server before production use; they are kept here so a correction is a
// one-line change.

import (
	"encoding/json"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// Play-state packet IDs (protocol 763, clientbound unless marked serverbound).
const (
	cPlayJoinGame        = 0x28
	cPlayKeepAlive       = 0x25
	cPlayPlayerInfo      = 0x36
	cPlayPlayerPosition  = 0x3C
	cPlaySetActionbar    = 0x60
	sPlayKeepAlive       = 0x12
	sPlayTeleportConfirm = 0x00
	sPlayClientStatus    = 0x03
)

// Limbo constants.
const (
	dimensionEnd      = "minecraft:the_end"
	gamemodeSpectator = 3
	limboEntityID     = 0
)

// chatComponent returns a JSON chat component with the given text.
func chatComponent(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"text": text})
	return b
}

// buildKeepAlive builds a clientbound Keep Alive payload for a limbo id.
func buildKeepAlive(id int64) []byte {
	w := protocol.NewWriter()
	_ = protocol.WriteLong(w, id)
	return w.Bytes()
}

// buildActionbarMessage builds a clientbound Set Action Bar payload (1.19.3+).
func buildActionbarMessage(text string) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteChatComponent(w, chatComponent(text)); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// buildPlayerInfoAdd builds a clientbound Player Info Update payload that adds
// a single player (action 0 = add player).
func buildPlayerInfoAdd(p auth.Profile) ([]byte, error) {
	w := protocol.NewWriter()
	_ = protocol.WriteVarInt(w, 0) // action: add player
	_ = protocol.WriteVarInt(w, 1) // number of players
	if err := protocol.WriteUUID(w, p.UUID); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(w, p.Name); err != nil {
		return nil, err
	}
	_ = protocol.WriteVarInt(w, 0) // number of properties
	_ = protocol.WriteVarInt(w, gamemodeSpectator)
	if err := protocol.WriteVarInt(w, 0); err != nil {
		return nil, err
	}
	_ = protocol.WriteBool(w, false) // has display name
	return w.Bytes(), nil
}

// buildPlayerPosition builds a clientbound Player Position And Look payload
// that teleports the player to a fixed point in the void.
func buildPlayerPosition(x, y, z, yaw, pitch float64) ([]byte, error) {
	w := protocol.NewWriter()
	_ = protocol.WriteDouble(w, x)
	_ = protocol.WriteDouble(w, y)
	_ = protocol.WriteDouble(w, z)
	_ = protocol.WriteFloat(w, float32(yaw))
	_ = protocol.WriteFloat(w, float32(pitch))
	_ = w.WriteByte(0) // flags: absolute
	_ = protocol.WriteVarInt(w, 0)
	_ = protocol.WriteBool(w, false)
	return w.Bytes(), nil
}

// buildJoinGame builds a clientbound Join Game payload for the limbo. The
// dimension is the End and the gamemode is spectator, so the player appears in
// an empty void.
func buildJoinGame(p auth.Profile) ([]byte, error) {
	w := protocol.NewWriter()
	_ = protocol.WriteInt(w, limboEntityID)
	_ = protocol.WriteBool(w, false) // hardcore
	_ = w.WriteByte(gamemodeSpectator)
	_ = w.WriteByte(0xFF)          // previous gamemode (-1)
	_ = protocol.WriteVarInt(w, 1) // world names count
	if err := protocol.WriteString(w, dimensionEnd); err != nil {
		return nil, err
	}
	codec, err := buildDimensionCodec()
	if err != nil {
		return nil, err
	}
	_, _ = w.Write(codec)
	if err := protocol.WriteString(w, dimensionEnd); err != nil {
		return nil, err
	}
	_ = protocol.WriteLong(w, 0) // hashed seed
	_ = protocol.WriteVarInt(w, 20)
	_ = protocol.WriteVarInt(w, 2) // view distance
	_ = protocol.WriteVarInt(w, 2) // simulation distance
	_ = protocol.WriteBool(w, false)
	_ = protocol.WriteBool(w, true) // enable respawn screen
	_ = protocol.WriteBool(w, false)
	if err := protocol.WriteString(w, dimensionEnd); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// buildDimensionCodec builds a minimal dimension registry NBT used by
// Join Game. It is intentionally small; real servers send full registry data
// and the observations of a live server should be used to fill it out.
func buildDimensionCodec() ([]byte, error) {
	n := protocol.NewNBTWriter()
	n.Root("")
	n.End()
	return n.Bytes(), nil
}
