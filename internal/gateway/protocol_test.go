package gateway

import (
	"bufio"
	"bytes"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	values := []int{0, 1, 127, 128, 300, 25565, 763, 2147483647}
	for _, v := range values {
		var buf bytes.Buffer
		if err := writeVarInt(&buf, v); err != nil {
			t.Fatalf("writeVarInt(%d): %v", v, err)
		}
		got, err := readVarInt(bufio.NewReader(&buf))
		if err != nil {
			t.Fatalf("readVarInt(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("VarInt roundtrip(%d) = %d", v, got)
		}
	}
}

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	var buf bytes.Buffer
	if err := writeFrame(&buf, 0x02, payload); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	id, body, err := readFrame(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if id != 0x02 {
		t.Errorf("packet id = %d, want 2", id)
	}
	if !bytes.Equal(body, payload) {
		t.Errorf("payload = %x, want %x", body, payload)
	}
}

func TestParseHandshakeStatus(t *testing.T) {
	// Build a handshake: protocol 763, host "example.com", port 25565, next
	// state 1 (status).
	var payload []byte
	pw := newByteWriter(&payload)
	_ = writeVarInt(pw, 763)
	_ = writeMCString(pw, "example.com")
	_ = writeUShort(pw, 25565)
	_ = writeVarInt(pw, 1)

	hs, err := parseHandshake(payload)
	if err != nil {
		t.Fatalf("parseHandshake: %v", err)
	}
	if hs.protocol != 763 || hs.serverAddr != "example.com" || hs.port != 25565 || hs.nextState != 1 {
		t.Errorf("handshake = %+v", hs)
	}
}

func TestParseHandshakeLogin(t *testing.T) {
	var payload []byte
	pw := newByteWriter(&payload)
	_ = writeVarInt(pw, 763)
	_ = writeMCString(pw, "localhost")
	_ = writeUShort(pw, 25565)
	_ = writeVarInt(pw, 2)

	hs, err := parseHandshake(payload)
	if err != nil {
		t.Fatalf("parseHandshake: %v", err)
	}
	if hs.nextState != 2 {
		t.Errorf("next state = %d, want 2", hs.nextState)
	}
}

func TestEncodeFrameReplaysVerbatim(t *testing.T) {
	// encodeFrame should reproduce the exact bytes that writeFrame produces so
	// the backend sees the original frame.
	payload := []byte{0x01, 0x02, 0x03}
	var original bytes.Buffer
	if err := writeFrame(&original, 0x00, payload); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	rebuilt := encodeFrame(0x00, payload)
	if !bytes.Equal(rebuilt, original.Bytes()) {
		t.Errorf("encodeFrame = %x, want %x", rebuilt, original.Bytes())
	}
}
