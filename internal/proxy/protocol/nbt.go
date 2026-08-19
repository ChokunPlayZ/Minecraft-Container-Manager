package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// NBT tag type IDs.
const (
	TagEnd       = 0
	TagByte      = 1
	TagShort     = 2
	TagInt       = 3
	TagLong      = 4
	TagFloat     = 5
	TagDouble    = 6
	TagByteArray = 7
	TagString    = 8
	TagList      = 9
	TagCompound  = 10
	TagIntArray  = 11
	TagLongArray = 12
)

// Tag is a decoded NBT value.
type Tag struct {
	Type byte
	Name string
	// Scalar values.
	Byte   byte
	Short  int16
	Int    int32
	Long   int64
	Float  float32
	Double float64
	Str    string
	// Arrays.
	ByteArray []byte
	IntArray  []int32
	LongArray []int64
	// ListType is the element type for a list.
	ListType byte
	List     []*Tag
	// Compound children.
	Children []*Tag
}

// NBTReader reads NBT from an underlying reader (used for inline tag values in
// packets where the NBT is not the root of a file).
type NBTReader struct {
	r io.Reader
}

// NewNBTReader returns an NBT reader over r.
func NewNBTReader(r io.Reader) *NBTReader { return &NBTReader{r: r} }

func (n *NBTReader) readByte() (byte, error) {
	var b [1]byte
	if _, err := io.ReadFull(n.r, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func (n *NBTReader) readShort() (int16, error) {
	var b [2]byte
	if _, err := io.ReadFull(n.r, b[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(b[:])), nil
}

func (n *NBTReader) readInt() (int32, error) {
	var b [4]byte
	if _, err := io.ReadFull(n.r, b[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(b[:])), nil
}

func (n *NBTReader) readLong() (int64, error) {
	var b [8]byte
	if _, err := io.ReadFull(n.r, b[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(b[:])), nil
}

func (n *NBTReader) readFloat() (float32, error) {
	v, err := n.readInt()
	return math.Float32frombits(uint32(v)), err
}

func (n *NBTReader) readDouble() (float64, error) {
	v, err := n.readLong()
	return math.Float64frombits(uint64(v)), err
}

func (n *NBTReader) readString() (string, error) {
	l, err := n.readShort()
	if err != nil {
		return "", err
	}
	if l < 0 {
		return "", fmt.Errorf("negative NBT string length")
	}
	b := make([]byte, l)
	if _, err := io.ReadFull(n.r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

// readTag reads a tag of the given type including its value, with an optional
// name already consumed by the caller.
func (n *NBTReader) readTag(typ byte) (*Tag, error) {
	t := &Tag{Type: typ}
	switch typ {
	case TagByte:
		b, err := n.readByte()
		if err != nil {
			return nil, err
		}
		t.Byte = b
	case TagShort:
		v, err := n.readShort()
		if err != nil {
			return nil, err
		}
		t.Short = v
	case TagInt:
		v, err := n.readInt()
		if err != nil {
			return nil, err
		}
		t.Int = v
	case TagLong:
		v, err := n.readLong()
		if err != nil {
			return nil, err
		}
		t.Long = v
	case TagFloat:
		v, err := n.readFloat()
		if err != nil {
			return nil, err
		}
		t.Float = v
	case TagDouble:
		v, err := n.readDouble()
		if err != nil {
			return nil, err
		}
		t.Double = v
	case TagByteArray:
		n0, err := n.readInt()
		if err != nil {
			return nil, err
		}
		if n0 < 0 {
			return nil, fmt.Errorf("negative byte array length")
		}
		b := make([]byte, n0)
		if _, err := io.ReadFull(n.r, b); err != nil {
			return nil, err
		}
		t.ByteArray = b
	case TagString:
		s, err := n.readString()
		if err != nil {
			return nil, err
		}
		t.Str = s
	case TagList:
		et, err := n.readByte()
		if err != nil {
			return nil, err
		}
		n0, err := n.readInt()
		if err != nil {
			return nil, err
		}
		if n0 < 0 {
			return nil, fmt.Errorf("negative list length")
		}
		t.ListType = et
		for i := int32(0); i < n0; i++ {
			child, err := n.readTag(et)
			if err != nil {
				return nil, err
			}
			t.List = append(t.List, child)
		}
	case TagCompound:
		for {
			ct, err := n.readByte()
			if err != nil {
				return nil, err
			}
			if ct == TagEnd {
				break
			}
			name, err := n.readString()
			if err != nil {
				return nil, err
			}
			child, err := n.readTag(ct)
			if err != nil {
				return nil, err
			}
			child.Name = name
			t.Children = append(t.Children, child)
		}
	case TagIntArray:
		n0, err := n.readInt()
		if err != nil {
			return nil, err
		}
		if n0 < 0 {
			return nil, fmt.Errorf("negative int array length")
		}
		for i := int32(0); i < n0; i++ {
			v, err := n.readInt()
			if err != nil {
				return nil, err
			}
			t.IntArray = append(t.IntArray, v)
		}
	case TagLongArray:
		n0, err := n.readInt()
		if err != nil {
			return nil, err
		}
		if n0 < 0 {
			return nil, fmt.Errorf("negative long array length")
		}
		for i := int32(0); i < n0; i++ {
			v, err := n.readLong()
			if err != nil {
				return nil, err
			}
			t.LongArray = append(t.LongArray, v)
		}
	default:
		return nil, fmt.Errorf("unsupported NBT tag type %d", typ)
	}
	return t, nil
}

// Read reads a root NBT tag (type + optional name + value).
func (n *NBTReader) Read() (*Tag, error) {
	typ, err := n.readByte()
	if err != nil {
		return nil, err
	}
	if typ == TagEnd {
		return &Tag{Type: TagEnd}, nil
	}
	if _, err := n.readString(); err != nil {
		return nil, err
	}
	return n.readTag(typ)
}

// NBTWriter writes NBT values packed into a bytes.Buffer.
type NBTWriter struct {
	buf bytes.Buffer
}

// NewNBTWriter returns an empty NBT writer.
func NewNBTWriter() *NBTWriter { return &NBTWriter{} }

func (n *NBTWriter) byte(v byte) { n.buf.WriteByte(v) }
func (n *NBTWriter) short(v int16) {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(v))
	n.buf.Write(b)
}
func (n *NBTWriter) int(v int32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	n.buf.Write(b)
}
func (n *NBTWriter) long(v int64) {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	n.buf.Write(b)
}
func (n *NBTWriter) str(s string) {
	n.short(int16(len(s)))
	n.buf.WriteString(s)
}

// Root starts a root compound with the given optional name.
func (n *NBTWriter) Root(name string) {
	n.byte(TagCompound)
	n.str(name)
}

// End writes a compound/root terminator.
func (n *NBTWriter) End() { n.byte(TagEnd) }

// Byte writes a named byte tag.
func (n *NBTWriter) Byte(name string, v byte) {
	n.byte(TagByte)
	n.str(name)
	n.byte(v)
}

// Int writes a named int tag.
func (n *NBTWriter) Int(name string, v int32) {
	n.byte(TagInt)
	n.str(name)
	n.int(v)
}

// Long writes a named long tag.
func (n *NBTWriter) Long(name string, v int64) {
	n.byte(TagLong)
	n.str(name)
	n.long(v)
}

// String writes a named string tag.
func (n *NBTWriter) String(name, s string) {
	n.byte(TagString)
	n.str(name)
	n.str(s)
}

// Bytes returns the encoded root NBT bytes.
func (n *NBTWriter) Bytes() []byte { return n.buf.Bytes() }
