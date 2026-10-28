package nbt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestAllTypesAndCompressionRoundTrip(t *testing.T) {
	root := Tag{Type: 10, Name: "root\x00🧭", Children: []Tag{
		{Type: 1, Name: "byte", Value: "-128"}, {Type: 2, Name: "short", Value: "32767"},
		{Type: 3, Name: "int", Value: "-2147483648"}, {Type: 4, Name: "long", Value: "9223372036854775807"},
		{Type: 5, Name: "float", Value: "-0"}, {Type: 6, Name: "double", Value: "1.2345678901234567"},
		{Type: 7, Name: "bytes", Values: []string{"-128", "0", "127"}},
		{Type: 8, Name: "string", Value: "NUL\x00 and 🧭 ภาษาไทย"},
		{Type: 9, Name: "list", ListType: 10, Children: []Tag{{Type: 10, Children: []Tag{{Type: 8, Name: "item", Value: "minecraft:diamond"}}}}},
		{Type: 10, Name: "compound"}, {Type: 11, Name: "ints", Values: []string{"-2147483648", "2147483647"}},
		{Type: 12, Name: "longs", Values: []string{"-9223372036854775808", "9223372036854775807"}},
		{Type: 9, Name: "empty", ListType: 10}, {Type: 5, Name: "nan", Value: "NaN:0x7fa00001"},
		{Type: 6, Name: "infinity", Value: "+Inf"},
	}}
	for _, compression := range []string{"none", "gzip", "zlib"} {
		t.Run(compression, func(t *testing.T) {
			original := Document{Root: root, Compression: compression}
			data, err := Encode(original)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			// Include the actual JSON boundary used by the browser.
			body, _ := json.Marshal(got)
			var edited Document
			if err = json.Unmarshal(body, &edited); err != nil {
				t.Fatal(err)
			}
			next, err := Encode(edited)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, next) {
				t.Fatal("unchanged NBT bytes changed across JSON")
			}
			if got.Compression != compression || got.Root.Children[3].Value != "9223372036854775807" {
				t.Fatalf("lost type/compression/precision: %+v", got)
			}
		})
	}
}

func TestKnownJavaFixture(t *testing.T) {
	// Named root compound -> Health float(20), XpLevel int(42), End.
	data := []byte{10, 0, 0, 5, 0, 6, 'H', 'e', 'a', 'l', 't', 'h', 0x41, 0xa0, 0, 0, 3, 0, 7, 'X', 'p', 'L', 'e', 'v', 'e', 'l', 0, 0, 0, 42, 0}
	doc, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Root.Children, []Tag{{Type: 5, Name: "Health", Value: "20"}, {Type: 3, Name: "XpLevel", Value: "42"}}) {
		t.Fatalf("wrong decode: %+v", doc)
	}
	doc.Root.Children[0].Value = "10"
	got, err := Encode(doc)
	if err != nil {
		t.Fatal(err)
	}
	data[13] = 0x20
	if !bytes.Equal(got, data) {
		t.Fatalf("wrong edit: %x", got)
	}
}

func TestRejectMalformedAndOversized(t *testing.T) {
	bad := [][]byte{
		{}, {0}, {13, 0, 0}, {10, 0, 0}, {10, 0, 0, 0, 42},
		{7, 0, 0, 0xff, 0xff, 0xff, 0xff},
		{12, 0, 0, 0x7f, 0xff, 0xff, 0xff},
		{9, 0, 0, 0, 0, 0, 0, 1},          // nonempty End list
		{9, 0, 0, 13, 0, 0, 0, 0},         // invalid empty list type
		{8, 0, 0, 0, 3, 0xed, 0xa0, 0x80}, // unpaired surrogate
	}
	for _, data := range bad {
		if _, err := Decode(data); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %x: %v", data, err)
		}
	}
	var bomb bytes.Buffer
	z := gzip.NewWriter(&bomb)
	_, _ = z.Write(bytes.Repeat([]byte{0}, MaxBytes+1))
	_ = z.Close()
	if _, err := Decode(bomb.Bytes()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted decompression bomb: %v", err)
	}
	// Very deep compounds must fail without recursion exhausting the stack.
	var deep bytes.Buffer
	for i := 0; i < 100; i++ {
		deep.Write([]byte{10, 0, 0})
	}
	deep.Write(make([]byte, 100))
	if _, err := Decode(deep.Bytes()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted deep nesting: %v", err)
	}
	var large bytes.Buffer
	large.Write([]byte{9, 0, 0, 10})
	_ = binary.Write(&large, binary.BigEndian, int32(MaxNodes+1))
	large.Write(make([]byte, MaxNodes+1))
	if _, err := Decode(large.Bytes()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted huge list: %v", err)
	}
}

func TestRejectInvalidEdits(t *testing.T) {
	for _, tag := range []Tag{
		{Type: 1, Value: "128"}, {Type: 4, Value: "9223372036854775808"}, {Type: 3, Value: "1.5"},
		{Type: 5, Value: "1e100"}, {Type: 12, Values: []string{"9223372036854775808"}},
		{Type: 9, ListType: 3, Children: []Tag{{Type: 4, Value: "1"}}},
		{Type: 9, ListType: 3, Children: []Tag{{Type: 3, Name: "named", Value: "1"}}},
		{Type: 10, Value: "ignored"}, {Type: 1, Value: "1", Children: []Tag{{Type: 1, Value: "2"}}},
		{Type: 10, Children: []Tag{{Type: 1, Name: "duplicate", Value: "1"}, {Type: 1, Name: "duplicate", Value: "2"}}},
	} {
		if _, err := Encode(Document{Compression: "none", Root: tag}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v: %v", tag, err)
		}
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{10, 0, 0, 0})
	f.Add([]byte{9, 0, 0, 3, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if doc, err := Decode(data); err == nil {
			encoded, err := Encode(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Decode(encoded); err != nil {
				t.Fatal(err)
			}
		}
	})
}
