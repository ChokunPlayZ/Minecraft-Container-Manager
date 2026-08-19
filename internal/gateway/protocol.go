package gateway

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// readVarInt reads a Minecraft variable-length integer from r.
func readVarInt(r io.ByteReader) (int, error) {
	var value int
	var shift uint
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= int(b&0x7F) << shift
		if b&0x80 == 0 {
			return value, nil
		}
		shift += 7
		if shift >= 35 {
			return 0, fmt.Errorf("varint too large")
		}
	}
}

// writeVarInt writes a Minecraft variable-length integer to w.
func writeVarInt(w io.Writer, v int) error {
	u := uint32(v)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		if _, err := w.Write([]byte{b}); err != nil {
			return err
		}
		if u == 0 {
			return nil
		}
	}
}

// readFrame reads a length-prefixed Minecraft packet and returns its packet
// ID and remaining payload. The payload excludes the length and packet ID.
func readFrame(r *bufio.Reader) (packetID int, payload []byte, err error) {
	length, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length < 0 || length > 1<<20 {
		return 0, nil, fmt.Errorf("invalid frame length %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	br := newByteReader(body)
	packetID, err = readVarInt(br)
	if err != nil {
		return 0, nil, err
	}
	return packetID, body[br.i:], nil
}

// writeFrame writes a length-prefixed Minecraft packet with the given ID and
// payload.
func writeFrame(w io.Writer, packetID int, payload []byte) error {
	var body []byte
	bw := newByteWriter(&body)
	if err := writeVarInt(bw, packetID); err != nil {
		return err
	}
	body = append(body, payload...)
	if err := writeVarInt(w, len(body)); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

type byteWriter struct {
	b *[]byte
}

func newByteWriter(b *[]byte) *byteWriter { return &byteWriter{b: b} }

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.b = append(*w.b, p...)
	return len(p), nil
}

type byteReader struct {
	b []byte
	i int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }

func (r *byteReader) ReadByte() (byte, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	b := r.b[r.i]
	r.i++
	return b, nil
}

// readMCString reads a length-prefixed UTF-8 string.
func readMCString(r io.ByteReader) (string, error) {
	n, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if n < 0 || n > 1<<16 {
		return "", fmt.Errorf("invalid string length %d", n)
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		b[i] = c
	}
	return string(b), nil
}

// readUShort reads a big-endian unsigned short.
func readUShort(r io.Reader) (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

// handshake is the parsed first packet a client sends (Handshake).
type handshake struct {
	protocol   int
	serverAddr string
	port       uint16
	nextState  int
}

// parseHandshake parses a Handshake packet payload into its fields.
func parseHandshake(payload []byte) (*handshake, error) {
	if len(payload) < 3 {
		return nil, fmt.Errorf("handshake too short")
	}
	br := newByteReader(payload)
	hs := &handshake{}
	var err error
	if hs.protocol, err = readVarInt(br); err != nil {
		return nil, err
	}
	if hs.serverAddr, err = readMCString(br); err != nil {
		return nil, err
	}
	if br.i+2 > len(payload) {
		return nil, fmt.Errorf("handshake missing port")
	}
	hs.port = binary.BigEndian.Uint16(payload[br.i : br.i+2])
	br.i += 2
	if hs.nextState, err = readVarInt(br); err != nil {
		return nil, err
	}
	return hs, nil
}

// encodeFrame rebuilds the full length-prefixed frame for a packet so it can
// be replayed verbatim to a backend.
func encodeFrame(packetID int, payload []byte) []byte {
	var body []byte
	bw := newByteWriter(&body)
	_ = writeVarInt(bw, packetID)
	body = append(body, payload...)
	var out []byte
	ow := newByteWriter(&out)
	_ = writeVarInt(ow, len(body))
	out = append(out, body...)
	return out
}

// statusResponse is the JSON body of a server-list status response.
type statusResponse struct {
	Version     statusVersion `json:"version"`
	Players     statusPlayers `json:"players"`
	Description any           `json:"description"`
	Favicon     string        `json:"favicon,omitempty"`
}

type statusVersion struct {
	Name     string `json:"name"`
	Protocol int    `json:"protocol"`
}

type statusPlayers struct {
	Max    int `json:"max"`
	Online int `json:"online"`
}

// buildStatusJSON builds the server-list status response for a server while
// sleeping, using the last-known-good MOTD as the description.
func buildStatusJSON(motd, version string) ([]byte, error) {
	resp := statusResponse{
		Version: statusVersion{Name: version, Protocol: 0},
		Players: statusPlayers{Max: 0, Online: 0},
		Description: struct {
			Text string `json:"text"`
		}{Text: motd},
	}
	return json.Marshal(resp)
}
