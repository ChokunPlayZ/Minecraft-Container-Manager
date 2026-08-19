package protocol

import (
	"bytes"
	"testing"
)

func TestFrameRoundTripUncompressed(t *testing.T) {
	rs := NewReaderState(bytes.NewReader(nil))
	ws := NewWriterState()
	for _, tc := range []struct {
		id      int32
		payload []byte
	}{
		{0x00, []byte{0x01, 0x02}},
		{0x03, bigPayload(4096)},
	} {
		frame, err := ws.WritePacket(tc.id, tc.payload)
		if err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
		rs.src = bytes.NewReader(frame)
		pkt, err := rs.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket: %v", err)
		}
		if pkt.ID != tc.id {
			t.Errorf("packet id = %d, want %d", pkt.ID, tc.id)
		}
		if !bytes.Equal(pkt.Payload, tc.payload) {
			t.Errorf("payload mismatch")
		}
	}
}

func TestFrameRoundTripCompressed(t *testing.T) {
	// A small payload stays uncompressed; a large one gets zlib-compressed.
	for _, tc := range []struct {
		payload []byte
		large   bool
	}{
		{[]byte{0x01, 0x02}, false},
		{bigPayload(8192), true},
	} {
		ws := NewWriterState()
		ws.SetCompression(256)
		frame, err := ws.WritePacket(0x12, tc.payload)
		if err != nil {
			t.Fatalf("WritePacket: %v", err)
		}
		rs := NewReaderState(bytes.NewReader(frame))
		rs.SetCompression(256)
		pkt, err := rs.ReadPacket()
		if err != nil {
			t.Fatalf("ReadPacket: %v", err)
		}
		if !bytes.Equal(pkt.Payload, tc.payload) {
			t.Errorf("payload mismatch for large=%v", tc.large)
		}
	}
}

func bigPayload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}
