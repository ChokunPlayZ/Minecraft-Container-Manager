package protocol

import "io"

// Reader reads Minecraft protocol data types from an underlying source.
type Reader struct {
	src io.Reader
}

// NewReader wraps src in a Reader.
func NewReader(src io.Reader) *Reader { return &Reader{src: src} }

// ReadByte implements io.ByteReader by reading a single raw byte.
func (r *Reader) ReadByte() (byte, error) {
	var b [1]byte
	if _, err := io.ReadFull(r.src, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

// Bytes reads n raw bytes.
func (r *Reader) Bytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r.src, b); err != nil {
		return nil, err
	}
	return b, nil
}

// Writer writes Minecraft protocol data types to an underlying sink. It
// accumulates into an internal buffer that can be flushed frame-by-frame.
type Writer struct {
	buf []byte
}

// NewWriter returns an empty Writer.
func NewWriter() *Writer { return &Writer{} }

// Write implements io.Writer, appending to the internal buffer.
func (w *Writer) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

// Len returns the number of buffered bytes.
func (w *Writer) Len() int { return len(w.buf) }

// Reset clears the buffer.
func (w *Writer) Reset() { w.buf = w.buf[:0] }

// Bytes returns a copy of the buffered bytes.
func (w *Writer) Bytes() []byte { return append([]byte(nil), w.buf...) }

// WriteByte writes a single raw byte.
func (w *Writer) WriteByte(b byte) error {
	w.buf = append(w.buf, b)
	return nil
}

// ByteReader adapts a buffer so the varint readers can consume it inline.
type ByteReader struct {
	buf    []byte
	offset int
}

// NewByteReader returns a ByteReader over data.
func NewByteReader(data []byte) *ByteReader {
	return &ByteReader{buf: data}
}

// ReadByte reads the next buffered byte.
func (br *ByteReader) ReadByte() (byte, error) {
	if br.offset >= len(br.buf) {
		return 0, io.EOF
	}
	b := br.buf[br.offset]
	br.offset++
	return b, nil
}

// ReadVarInt reads a VarInt from the buffer.
func (br *ByteReader) ReadVarInt() (int32, error) {
	return ReadVarInt(br)
}

// Remaining returns the number of unread buffered bytes.
func (br *ByteReader) Remaining() int { return len(br.buf) - br.offset }
