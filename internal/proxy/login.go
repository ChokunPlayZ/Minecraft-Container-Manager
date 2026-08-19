package proxy

import (
	"encoding/json"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// readLoginStartName parses the player name from a Login Start payload.
func readLoginStartName(payload []byte) (string, error) {
	return protocol.ReadString(protocol.NewByteReader(payload))
}

// buildLoginEncryptionRequest builds a clientbound Login Encryption Request
// payload for the given public key and verify token.
func buildLoginEncryptionRequest(pubKey, verifyToken []byte) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteString(w, " "); err != nil {
		return nil, err
	}
	if err := protocol.WriteVarInt(w, int32(len(pubKey))); err != nil {
		return nil, err
	}
	_, _ = w.Write(pubKey)
	if err := protocol.WriteVarInt(w, int32(len(verifyToken))); err != nil {
		return nil, err
	}
	_, _ = w.Write(verifyToken)
	return w.Bytes(), nil
}

// buildLoginSuccess builds a clientbound Login Success payload for the given
// profile. The UUID is sent as a hyphenated string.
func buildLoginSuccess(p auth.Profile) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteString(w, p.UUID.String()); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(w, p.Name); err != nil {
		return nil, err
	}
	// Properties: empty array (count 0) suffices.
	if err := protocol.WriteVarInt(w, 0); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// buildLoginDisconnect builds a clientbound Login Disconnect payload.
func buildLoginDisconnect(reason string) ([]byte, error) {
	raw, _ := json.Marshal(map[string]string{"text": reason})
	w := protocol.NewWriter()
	if err := protocol.WriteChatComponent(w, raw); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
