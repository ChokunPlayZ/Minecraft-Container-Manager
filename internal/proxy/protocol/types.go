package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const (
	maxStringLen  = 1 << 16
	maxChatLength = 262144
)

// ReadString reads a UTF-8 length-prefixed string.
func ReadString(r io.ByteReader) (string, error) {
	n, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	if n < 0 || n > maxStringLen {
		return "", fmt.Errorf("invalid string length %d", n)
	}
	b := make([]byte, n)
	for i := int32(0); i < n; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		b[i] = c
	}
	return string(b), nil
}

// WriteString writes a UTF-8 length-prefixed string.
func WriteString(w io.Writer, s string) error {
	if len(s) > maxStringLen {
		return fmt.Errorf("string too long: %d", len(s))
	}
	if err := WriteVarInt(w, int32(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

// ReadBool reads a protocol boolean (single byte 0/1).
func ReadBool(r io.ByteReader) (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// WriteBool writes a protocol boolean.
func WriteBool(w io.Writer, v bool) error {
	b := byte(0)
	if v {
		b = 1
	}
	_, err := w.Write([]byte{b})
	return err
}

// ReadUUID reads a UUID from two big-endian int64s.
func ReadUUID(r io.Reader) ([16]byte, error) {
	var u [16]byte
	if _, err := io.ReadFull(r, u[:]); err != nil {
		return u, err
	}
	return u, nil
}

// WriteUUID writes a UUID as two big-endian int64s.
func WriteUUID(w io.Writer, u [16]byte) error {
	_, err := w.Write(u[:])
	return err
}

// Position is a packed Minecraft world position (x, y, z).
type Position struct {
	X int32
	Y int32
	Z int32
}

// ReadPosition reads a packed 64-bit position.
func ReadPosition(r io.Reader) (Position, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return Position{}, err
	}
	v := int64(binary.BigEndian.Uint64(b[:]))
	u := uint64(v)
	x := int32(u & 0x3FFFFFF)
	y := int32((u >> 26) & 0xFFF)
	z := int32((u >> 38) & 0x3FFFFFF)
	if x >= 1<<25 {
		x -= 1 << 26
	}
	if y >= 1<<11 {
		y -= 1 << 12
	}
	if z >= 1<<25 {
		z -= 1 << 26
	}
	return Position{X: x, Y: y, Z: z}, nil
}

// WritePosition packs and writes a position.
func WritePosition(w io.Writer, p Position) error {
	var v uint64
	v |= uint64(p.X & 0x3FFFFFF)
	v |= uint64(p.Y&0xFFF) << 26
	v |= uint64(p.Z&0x3FFFFFF) << 38
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	_, err := w.Write(b[:])
	return err
}

// ReadInt reads a big-endian int32.
func ReadInt(r io.Reader) (int32, error) {
	var b [4]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b[:])), nil
}

// ReadShort reads a big-endian int16.
func ReadShort(r io.Reader) (int16, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(b[:])), nil
}

// WriteShort writes a big-endian int16.
func WriteShort(w io.Writer, v int16) error {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], uint16(v))
	_, err := w.Write(b[:])
	return err
}

// WriteInt writes a big-endian int32.
func WriteInt(w io.Writer, v int32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	_, err := w.Write(b[:])
	return err
}

// ReadLong reads a big-endian int64.
func ReadLong(r io.Reader) (int64, error) {
	var b [8]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b[:])), nil
}

// WriteLong writes a big-endian int64.
func WriteLong(w io.Writer, v int64) error {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	_, err := w.Write(b[:])
	return err
}

// WriteDouble writes a big-endian float64.
func WriteDouble(w io.Writer, v float64) error {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	_, err := w.Write(b[:])
	return err
}

// WriteFloat writes a big-endian float32.
func WriteFloat(w io.Writer, v float32) error {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], math.Float32bits(v))
	_, err := w.Write(b[:])
	return err
}

// ReadChatComponentAt reads a JSON chat component of a fixed size (bounded).
func ReadChatComponent(r io.ByteReader) (json.RawMessage, error) {
	n, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if n < 0 || n > maxChatLength {
		return nil, fmt.Errorf("invalid chat length %d", n)
	}
	b := make([]byte, n)
	for i := int32(0); i < n; i++ {
		c, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		b[i] = c
	}
	return b, nil
}

// WriteChatComponent writes a JSON chat component.
func WriteChatComponent(w io.Writer, raw json.RawMessage) error {
	if len(raw) > maxChatLength {
		return fmt.Errorf("chat length too long: %d", len(raw))
	}
	if err := WriteVarInt(w, int32(len(raw))); err != nil {
		return err
	}
	_, err := w.Write(raw)
	return err
}
