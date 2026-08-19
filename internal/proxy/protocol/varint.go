// Package protocol implements the primitives and framing of the Minecraft
// Java protocol: VarInt/VarLong, strings, positions, UUIDs, chat components,
// a minimal NBT reader/writer, and packet framing with optional compression.
package protocol

import (
	"errors"
	"io"
)

// maxVarIntBytes is the maximum encoded length of a VarInt (5 bytes, 35 bits
// including continuation bits).
const maxVarIntBytes = 5

// maxVarLongBytes is the maximum encoded length of a VarLong (10 bytes,
// 70 bits including continuation bits).
const maxVarLongBytes = 10

// ErrVarIntTooLong is returned when a VarInt/VarLong exceeds its allowed byte
// length, which guards against a malicious stream.
var ErrVarIntTooLong = errors.New("varint exceeds maximum length")

// ReadVarInt reads a Minecraft variable-length int (up to 5 bytes) from r.
func ReadVarInt(r io.ByteReader) (int32, error) {
	var value uint32
	for i := 0; i < maxVarIntBytes; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= uint32(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return int32(value), nil
		}
	}
	return 0, ErrVarIntTooLong
}

// WriteVarInt writes v as a Minecraft variable-length int.
func WriteVarInt(w io.Writer, v int32) error {
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

// ReadVarLong reads a Minecraft variable-length long (up to 10 bytes).
func ReadVarLong(r io.ByteReader) (int64, error) {
	var value uint64
	for i := 0; i < maxVarLongBytes; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= uint64(b&0x7F) << (7 * i)
		if b&0x80 == 0 {
			return int64(value), nil
		}
	}
	return 0, ErrVarIntTooLong
}

// WriteVarLong writes v as a Minecraft variable-length long.
func WriteVarLong(w io.Writer, v int64) error {
	u := uint64(v)
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
