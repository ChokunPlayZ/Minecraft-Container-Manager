// Package nbt reads and writes bounded Java Edition NBT documents. Numeric
// values cross JSON as strings so JavaScript never rounds a 64-bit integer.
package nbt

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	MaxBytes    = 16 << 20
	MaxNodes    = 100000
	MaxElements = 1000000
)

// A compound's Children retain order and names. A list's Children are unnamed
// and share ListType, even when empty. Arrays use Values rather than child tags.
type Tag struct {
	Type     byte     `json:"type"`
	Name     string   `json:"name"`
	Value    string   `json:"value,omitempty"`
	Children []Tag    `json:"children,omitempty"`
	ListType byte     `json:"list_type,omitempty"`
	Values   []string `json:"values,omitempty"`
}

type Document struct {
	Compression string `json:"compression"`
	Root        Tag    `json:"root"`
}

var ErrInvalid = errors.New("invalid NBT")

func invalid(message string) error { return fmt.Errorf("%w: %s", ErrInvalid, message) }

func Decode(data []byte) (Document, error) {
	doc := Document{Compression: "none"}
	if len(data) > MaxBytes {
		return doc, invalid("file exceeds 16 MiB")
	}
	var stream io.ReadCloser
	var err error
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		doc.Compression = "gzip"
		stream, err = gzip.NewReader(bytes.NewReader(data))
	} else if len(data) >= 2 && data[0]&15 == 8 && (int(data[0])*256+int(data[1]))%31 == 0 {
		doc.Compression = "zlib"
		stream, err = zlib.NewReader(bytes.NewReader(data))
	}
	if err != nil {
		return doc, invalid("cannot decompress file")
	}
	if stream != nil {
		defer stream.Close()
		data, err = io.ReadAll(io.LimitReader(stream, MaxBytes+1))
		if err != nil {
			return doc, invalid("corrupt compressed data")
		}
		if len(data) > MaxBytes {
			return doc, invalid("decompressed file exceeds 16 MiB")
		}
	}
	r := reader{Reader: bytes.NewReader(data)}
	kind, err := r.ReadByte()
	if err != nil || kind < 1 || kind > 12 {
		return doc, invalid("expected a named Java Edition NBT tag")
	}
	name, err := r.text()
	if err != nil {
		return doc, invalid("invalid root name")
	}
	doc.Root, err = r.tag(kind, name, 0)
	if err != nil {
		return doc, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if r.Len() != 0 {
		return doc, invalid("unexpected bytes after root tag")
	}
	return doc, nil
}

type reader struct {
	*bytes.Reader
	nodes, elements int
}

func (r *reader) number(v any) error { return binary.Read(r, binary.BigEndian, v) }
func (r *reader) text() (string, error) {
	var size uint16
	if err := r.number(&size); err != nil {
		return "", err
	}
	b := make([]byte, int(size))
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return decodeText(b)
}

func (r *reader) count(width int) (int, error) {
	var n int32
	if err := r.number(&n); err != nil {
		return 0, err
	}
	if n < 0 || int64(n)*int64(width) > int64(r.Len()) || int(n) > MaxElements-r.elements {
		return 0, invalid("invalid collection length")
	}
	r.elements += int(n)
	return int(n), nil
}

func (r *reader) tag(kind byte, name string, depth int) (Tag, error) {
	t := Tag{Type: kind, Name: name}
	r.nodes++
	if depth > 64 || r.nodes > MaxNodes {
		return t, invalid("document is too complex")
	}
	var err error
	switch kind {
	case 1:
		var v int8
		err = r.number(&v)
		t.Value = strconv.FormatInt(int64(v), 10)
	case 2:
		var v int16
		err = r.number(&v)
		t.Value = strconv.FormatInt(int64(v), 10)
	case 3:
		var v int32
		err = r.number(&v)
		t.Value = strconv.FormatInt(int64(v), 10)
	case 4:
		var v int64
		err = r.number(&v)
		t.Value = strconv.FormatInt(v, 10)
	case 5:
		var v uint32
		err = r.number(&v)
		t.Value = formatFloat(uint64(v), 32)
	case 6:
		var v uint64
		err = r.number(&v)
		t.Value = formatFloat(v, 64)
	case 8:
		t.Value, err = r.text()
	case 7, 11, 12:
		width := map[byte]int{7: 1, 11: 4, 12: 8}[kind]
		var n int
		n, err = r.count(width)
		if err != nil {
			return t, err
		}
		t.Values = make([]string, n)
		for i := range t.Values {
			var v int64
			switch kind {
			case 7:
				var x int8
				err = r.number(&x)
				v = int64(x)
			case 11:
				var x int32
				err = r.number(&x)
				v = int64(x)
			case 12:
				err = r.number(&v)
			}
			if err != nil {
				return t, err
			}
			t.Values[i] = strconv.FormatInt(v, 10)
		}
	case 9:
		t.ListType, err = r.ReadByte()
		if err != nil {
			return t, err
		}
		var n int
		n, err = r.count(1)
		if err != nil {
			return t, err
		}
		if t.ListType > 12 || (t.ListType == 0 && n != 0) || n > MaxNodes-r.nodes {
			return t, invalid("invalid list type or length")
		}
		for i := 0; i < n; i++ {
			child, e := r.tag(t.ListType, "", depth+1)
			if e != nil {
				return t, e
			}
			t.Children = append(t.Children, child)
		}
	case 10:
		names := map[string]bool{}
		for {
			var k byte
			k, err = r.ReadByte()
			if err != nil || k == 0 {
				break
			}
			var childName string
			childName, err = r.text()
			if err != nil {
				break
			}
			if names[childName] {
				return t, invalid("duplicate compound tag name")
			}
			names[childName] = true
			child, e := r.tag(k, childName, depth+1)
			if e != nil {
				return t, e
			}
			t.Children = append(t.Children, child)
		}
	default:
		return t, invalid("unknown tag type")
	}
	return t, err
}

// Preserve NaN payload bits too: untouched mod data must survive an edit.
func formatFloat(bits uint64, size int) string {
	v := math.Float64frombits(bits)
	if size == 32 {
		v = float64(math.Float32frombits(uint32(bits)))
	}
	if math.IsNaN(v) {
		return fmt.Sprintf("NaN:0x%x", bits)
	}
	return strconv.FormatFloat(v, 'g', -1, size)
}

func Encode(doc Document) ([]byte, error) {
	w := writer{}
	w.WriteByte(doc.Root.Type)
	if err := w.text(doc.Root.Name); err != nil {
		return nil, err
	}
	if err := w.tag(doc.Root, 0); err != nil {
		return nil, err
	}
	if w.Len() > MaxBytes {
		return nil, invalid("document exceeds 16 MiB")
	}
	if doc.Compression == "none" {
		return w.Bytes(), nil
	}
	var compressed bytes.Buffer
	var z io.WriteCloser
	switch doc.Compression {
	case "gzip":
		z = gzip.NewWriter(&compressed)
	case "zlib":
		z = zlib.NewWriter(&compressed)
	default:
		return nil, invalid("unknown compression")
	}
	if _, err := z.Write(w.Bytes()); err != nil {
		return nil, err
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if compressed.Len() > MaxBytes {
		return nil, invalid("compressed file exceeds 16 MiB")
	}
	return compressed.Bytes(), nil
}

type writer struct {
	bytes.Buffer
	nodes, elements int
}

func (w *writer) number(v any) { _ = binary.Write(&w.Buffer, binary.BigEndian, v) }
func (w *writer) text(s string) error {
	if !utf8.ValidString(s) || len(s) > 65535 {
		return invalid("invalid or oversized string")
	}
	b := encodeText(s)
	if len(b) > 65535 {
		return invalid("string exceeds 65535 encoded bytes")
	}
	w.number(uint16(len(b)))
	w.Write(b)
	return nil
}
func (w *writer) tag(t Tag, depth int) error {
	w.nodes++
	if depth > 64 || w.nodes > MaxNodes || w.Len() > MaxBytes {
		return invalid("document exceeds size or complexity limits")
	}
	fail := func() error {
		return invalid(fmt.Sprintf("invalid value or structure for tag %q (type %d)", t.Name, t.Type))
	}
	if t.Type < 1 || t.Type > 12 {
		return fail()
	}
	if t.Type != 9 && t.Type != 10 && len(t.Children) != 0 {
		return fail()
	}
	if t.Type != 7 && t.Type != 11 && t.Type != 12 && len(t.Values) != 0 {
		return fail()
	}
	if t.Type != 9 && t.ListType != 0 {
		return fail()
	}
	if (t.Type == 7 || t.Type == 9 || t.Type == 10 || t.Type == 11 || t.Type == 12) && t.Value != "" {
		return fail()
	}
	switch t.Type {
	case 1, 2, 3, 4:
		bits := map[byte]int{1: 8, 2: 16, 3: 32, 4: 64}[t.Type]
		v, err := strconv.ParseInt(t.Value, 10, bits)
		if err != nil {
			return fail()
		}
		switch t.Type {
		case 1:
			w.number(int8(v))
		case 2:
			w.number(int16(v))
		case 3:
			w.number(int32(v))
		case 4:
			w.number(v)
		}
	case 5, 6:
		size := 64
		if t.Type == 5 {
			size = 32
		}
		var bits uint64
		if len(t.Value) > 6 && t.Value[:6] == "NaN:0x" {
			v, err := strconv.ParseUint(t.Value[6:], 16, size)
			if err != nil {
				return fail()
			}
			bits = v
			if formatFloat(bits, size) != t.Value {
				return fail()
			}
		} else {
			v, err := strconv.ParseFloat(t.Value, size)
			if err != nil {
				return fail()
			}
			if size == 32 {
				bits = uint64(math.Float32bits(float32(v)))
			} else {
				bits = math.Float64bits(v)
			}
		}
		if size == 32 {
			w.number(uint32(bits))
		} else {
			w.number(bits)
		}
	case 8:
		if err := w.text(t.Value); err != nil {
			return err
		}
	case 7, 11, 12:
		if len(t.Values) > MaxElements-w.elements {
			return invalid("too many array elements")
		}
		w.elements += len(t.Values)
		w.number(int32(len(t.Values)))
		bits := map[byte]int{7: 8, 11: 32, 12: 64}[t.Type]
		for _, value := range t.Values {
			v, err := strconv.ParseInt(value, 10, bits)
			if err != nil {
				return fail()
			}
			switch t.Type {
			case 7:
				w.number(int8(v))
			case 11:
				w.number(int32(v))
			case 12:
				w.number(v)
			}
		}
	case 9:
		if t.ListType > 12 || (t.ListType == 0 && len(t.Children) > 0) || len(t.Children) > MaxElements-w.elements {
			return fail()
		}
		w.elements += len(t.Children)
		w.WriteByte(t.ListType)
		w.number(int32(len(t.Children)))
		for _, child := range t.Children {
			if child.Type != t.ListType || child.Name != "" {
				return fail()
			}
			if err := w.tag(child, depth+1); err != nil {
				return err
			}
		}
	case 10:
		names := map[string]bool{}
		for _, child := range t.Children {
			if names[child.Name] {
				return invalid("duplicate compound tag name")
			}
			names[child.Name] = true
			w.WriteByte(child.Type)
			if err := w.text(child.Name); err != nil {
				return err
			}
			if err := w.tag(child, depth+1); err != nil {
				return err
			}
		}
		w.WriteByte(0)
	}
	return nil
}

// Java's DataInput uses modified UTF-8: NUL is two bytes and supplementary
// characters are encoded as UTF-16 surrogate pairs. Reject unpaired surrogates
// instead of silently replacing them when a document crosses JSON.
func decodeText(b []byte) (string, error) {
	units := make([]uint16, 0, len(b))
	for i := 0; i < len(b); {
		c := b[i]
		i++
		switch {
		case c < 0x80:
			units = append(units, uint16(c))
		case c&0xe0 == 0xc0:
			if i >= len(b) || b[i]&0xc0 != 0x80 {
				return "", invalid("invalid modified UTF-8")
			}
			v := uint16(c&31)<<6 | uint16(b[i]&63)
			i++
			if v < 128 && v != 0 {
				return "", invalid("invalid modified UTF-8")
			}
			units = append(units, v)
		case c&0xf0 == 0xe0:
			if i+1 >= len(b) || b[i]&0xc0 != 0x80 || b[i+1]&0xc0 != 0x80 {
				return "", invalid("invalid modified UTF-8")
			}
			v := uint16(c&15)<<12 | uint16(b[i]&63)<<6 | uint16(b[i+1]&63)
			i += 2
			if v < 2048 {
				return "", invalid("invalid modified UTF-8")
			}
			units = append(units, v)
		default:
			return "", invalid("invalid modified UTF-8")
		}
	}
	for i := 0; i < len(units); i++ {
		if units[i] >= 0xd800 && units[i] <= 0xdbff {
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", invalid("unpaired UTF-16 surrogate")
			}
			i++
		} else if units[i] >= 0xdc00 && units[i] <= 0xdfff {
			return "", invalid("unpaired UTF-16 surrogate")
		}
	}
	return string(utf16.Decode(units)), nil
}
func encodeText(s string) []byte {
	b := make([]byte, 0, len(s))
	for _, u := range utf16.Encode([]rune(s)) {
		switch {
		case u > 0 && u < 128:
			b = append(b, byte(u))
		case u < 2048:
			b = append(b, 0xc0|byte(u>>6), 0x80|byte(u&63))
		default:
			b = append(b, 0xe0|byte(u>>12), 0x80|byte((u>>6)&63), 0x80|byte(u&63))
		}
	}
	return b
}
