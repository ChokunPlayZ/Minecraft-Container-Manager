package protocol

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

const (
	// MaxPacketLength bounds a single uncompressed frame payload (1 MiB).
	MaxPacketLength = 1 << 20
)

// Packet is a decoded frame: its protocol packet ID and payload (excluding the
// packet ID and length/compression header).
type Packet struct {
	ID      int32
	Payload []byte
}

// ReaderState reads length-prefixed (and optionally compressed) frames from an
// underlying stream.
type ReaderState struct {
	src        io.Reader
	compressed bool
	threshold  int32
}

// NewReaderState returns a frame reader that initially decodes uncompressed
// frames. Use SetCompression to enable compressed framing.
func NewReaderState(src io.Reader) *ReaderState {
	return &ReaderState{src: src}
}

// SetCompression enables compressed framing with the given threshold.
func (f *ReaderState) SetCompression(threshold int32) {
	f.compressed = threshold >= 0
	f.threshold = threshold
}

// ReadPacket reads one frame and returns its packet.
func (f *ReaderState) ReadPacket() (Packet, error) {
	length, err := ReadVarInt(byteReaderOf(f.src))
	if err != nil {
		return Packet{}, err
	}
	if length < 0 || length > MaxPacketLength {
		return Packet{}, fmt.Errorf("invalid frame length %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(f.src, body); err != nil {
		return Packet{}, err
	}
	return f.decode(body)
}

// decode transforms a raw frame body (which may itself contain a compressed
// payload) into a decoded packet.
func (f *ReaderState) decode(body []byte) (Packet, error) {
	var payload []byte
	if f.compressed {
		br := &ByteReader{buf: body}
		dataLen, err := br.ReadVarInt()
		if err != nil {
			return Packet{}, err
		}
		if dataLen == 0 {
			// Uncompressed frame: the remainder is the raw packet body.
			payload = body[br.offset:]
		} else {
			compressed := body[br.offset:]
			zr, zerr := zlib.NewReader(bytes.NewReader(compressed))
			if zerr != nil {
				return Packet{}, fmt.Errorf("zlib open: %w", zerr)
			}
			defer zr.Close()
			decompressed, derr := io.ReadAll(io.LimitReader(zr, MaxPacketLength+1))
			if derr != nil {
				return Packet{}, fmt.Errorf("zlib read: %w", derr)
			}
			if int32(len(decompressed)) != dataLen {
				return Packet{}, fmt.Errorf("decompressed length %d, declared %d", len(decompressed), dataLen)
			}
			payload = decompressed
		}
	} else {
		payload = body
	}
	br := &ByteReader{buf: payload}
	id, err := br.ReadVarInt()
	if err != nil {
		return Packet{}, err
	}
	return Packet{ID: id, Payload: payload[br.offset:]}, nil
}

// WriterState encodes frames with optional compression.
type WriterState struct {
	compressed bool
	threshold  int32
}

// NewWriterState returns a frame writer, initially uncompressed.
func NewWriterState() *WriterState { return &WriterState{} }

// SetCompression enables compressed framing with the given threshold.
func (f *WriterState) SetCompression(threshold int32) {
	f.compressed = threshold >= 0
	f.threshold = threshold
}

// WritePacket encodes a packet to its frame bytes.
func (f *WriterState) WritePacket(id int32, payload []byte) ([]byte, error) {
	var packetBody []byte
	pw := &bytes.Buffer{}
	if err := WriteVarInt(pw, id); err != nil {
		return nil, err
	}
	packetBody = append(packetBody, pw.Bytes()...)
	packetBody = append(packetBody, payload...)

	var frameBody []byte
	if f.compressed {
		var out bytes.Buffer
		var bw = &bytes.Buffer{}
		if len(packetBody) >= int(f.threshold) {
			zw := zlib.NewWriter(&out)
			if _, err := zw.Write(packetBody); err != nil {
				return nil, err
			}
			if err := zw.Close(); err != nil {
				return nil, err
			}
			if err := WriteVarInt(bw, int32(len(packetBody))); err != nil {
				return nil, err
			}
			frameBody = append(frameBody, bw.Bytes()...)
			frameBody = append(frameBody, out.Bytes()...)
		} else {
			if err := WriteVarInt(bw, 0); err != nil {
				return nil, err
			}
			frameBody = append(frameBody, bw.Bytes()...)
			frameBody = append(frameBody, packetBody...)
		}
	} else {
		frameBody = packetBody
	}

	var lengthBuf bytes.Buffer
	if err := WriteVarInt(&lengthBuf, int32(len(frameBody))); err != nil {
		return nil, err
	}
	out := append(lengthBuf.Bytes(), frameBody...)
	return out, nil
}

// byteReaderOf wraps an io.Reader as an io.ByteReader.
func byteReaderOf(r io.Reader) io.ByteReader {
	if br, ok := r.(io.ByteReader); ok {
		return br
	}
	return &readWrapper{r: r}
}

type readWrapper struct {
	r io.Reader
}

func (w *readWrapper) ReadByte() (byte, error) {
	var b [1]byte
	if _, err := io.ReadFull(w.r, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}
