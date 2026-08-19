package protocol

import (
	"bytes"
	"testing"
)

func TestPositionRoundTrip(t *testing.T) {
	cases := []Position{
		{0, 0, 0},
		{1, 2, 3},
		{-1, -2, -3},
		{30000000, 255, -30000000},
		{123456, -64, 987654},
	}
	for _, p := range cases {
		var buf bytes.Buffer
		if err := WritePosition(&buf, p); err != nil {
			t.Fatalf("WritePosition(%+v): %v", p, err)
		}
		got, err := ReadPosition(&buf)
		if err != nil {
			t.Fatalf("ReadPosition(%+v): %v", p, err)
		}
		if got != p {
			t.Errorf("Position roundtrip(%+v) = %+v", p, got)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, s := range []string{"", "hello", "Tʜᴇ ᴇɴᴅ", "localhost"} {
		var buf bytes.Buffer
		if err := WriteString(&buf, s); err != nil {
			t.Fatalf("WriteString(%q): %v", s, err)
		}
		got, err := ReadString(&ByteReader{buf: buf.Bytes()})
		if err != nil {
			t.Fatalf("ReadString(%q): %v", s, err)
		}
		if got != s {
			t.Errorf("String roundtrip(%q) = %q", s, got)
		}
	}
}

func TestUUIDRoundTrip(t *testing.T) {
	u := [16]byte{0xDE, 0xAD, 0xBE, 0xEF, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	var buf bytes.Buffer
	if err := WriteUUID(&buf, u); err != nil {
		t.Fatalf("WriteUUID: %v", err)
	}
	got, err := ReadUUID(&buf)
	if err != nil {
		t.Fatalf("ReadUUID: %v", err)
	}
	if got != u {
		t.Errorf("UUID roundtrip mismatch")
	}
}
