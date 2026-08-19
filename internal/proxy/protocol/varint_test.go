package protocol

import (
	"bytes"
	"testing"
)

func TestVarIntKnownVectors(t *testing.T) {
	cases := []struct {
		value int32
		bytes []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7F}},
		{128, []byte{0x80, 0x01}},
		{300, []byte{0xAC, 0x02}},
		{25565, []byte{0xDD, 0xC7, 0x01}},
		{2147483647, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x07}},
		{-1, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := WriteVarInt(&buf, c.value); err != nil {
			t.Fatalf("WriteVarInt(%d): %v", c.value, err)
		}
		if !bytes.Equal(buf.Bytes(), c.bytes) {
			t.Errorf("WriteVarInt(%d) = %x, want %x", c.value, buf.Bytes(), c.bytes)
		}
		got, err := ReadVarInt(&ByteReader{buf: buf.Bytes()})
		if err != nil {
			t.Fatalf("ReadVarInt for %d: %v", c.value, err)
		}
		if got != c.value {
			t.Errorf("ReadVarInt = %d, want %d", got, c.value)
		}
	}
}

func TestVarLongRoundTrip(t *testing.T) {
	values := []int64{0, 1, 127, 300, 2147483647, 9223372036854775807, -1}
	for _, v := range values {
		var buf bytes.Buffer
		if err := WriteVarLong(&buf, v); err != nil {
			t.Fatalf("WriteVarLong(%d): %v", v, err)
		}
		got, err := ReadVarLong(&ByteReader{buf: buf.Bytes()})
		if err != nil {
			t.Fatalf("ReadVarLong(%d): %v", v, err)
		}
		if got != v {
			t.Errorf("VarLong roundtrip(%d) = %d", v, got)
		}
	}
}

func TestReadVarIntTooLong(t *testing.T) {
	_, err := ReadVarInt(&ByteReader{buf: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}})
	if err != ErrVarIntTooLong {
		t.Errorf("expected ErrVarIntTooLong, got %v", err)
	}
}
