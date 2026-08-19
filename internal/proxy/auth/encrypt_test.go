package auth

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

// TestWrapConnPersistsCFB8Stream proves the AES/CFB8 keystream survives across
// multiple separate Read and Write calls on an encrypted connection. If the
// reader/writer were recreated with a fresh IV on each call, only the first
// frame would decrypt correctly.
func TestWrapConnPersistsCFB8Stream(t *testing.T) {
	secret := []byte("0123456789abcdef")
	encSender, err := NewEncryptor(secret)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	encReceiver, err := NewEncryptor(secret)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	sender, receiver := net.Pipe()
	defer sender.Close()
	defer receiver.Close()

	senderConn := WrapConn(sender, encSender)
	receiverConn := WrapConn(receiver, encReceiver)

	frames := [][]byte{
		[]byte("frame one payload"),
		[]byte("frame-two-payload-2"),
		[]byte("frame three payload!!"),
	}

	// Sender writes all three frames across separate Write calls while the
	// receiver reads them across separate Read calls. Frames two and three
	// corrupt unless the IV persists.
	go func() {
		for _, f := range frames {
			if err := writeFrame(senderConn, f); err != nil {
				t.Errorf("sender write: %v", err)
				return
			}
		}
	}()
	for i, want := range frames {
		got, err := readFrame(receiverConn)
		if err != nil {
			t.Fatalf("receiver read frame %d: %v", i, err)
		}
		if string(got) != string(want) {
			t.Fatalf("frame %d = %q, want %q", i, got, want)
		}
	}

	// Confirm the reverse direction likewise survives multiple calls.
	go func() {
		for _, f := range frames {
			if err := writeFrame(receiverConn, f); err != nil {
				t.Errorf("receiver write: %v", err)
				return
			}
		}
	}()
	for i, want := range frames {
		got, err := readFrame(senderConn)
		if err != nil {
			t.Fatalf("sender read frame %d: %v", i, err)
		}
		if string(got) != string(want) {
			t.Fatalf("reverse frame %d = %q, want %q", i, got, want)
		}
	}
}

func writeFrame(c io.Writer, payload []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := c.Write(hdr[:]); err != nil {
		return err
	}
	_, err := c.Write(payload)
	return err
}

func readFrame(c io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > 1<<20 {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c, buf); err != nil {
		return nil, err
	}
	return buf, nil
}
