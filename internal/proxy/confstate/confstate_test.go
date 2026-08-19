package confstate

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

func TestRunConfigCompletes(t *testing.T) {
	tests := []struct {
		protocol        int32
		wantClientbound []int32
	}{
		{protocol: 764, wantClientbound: []int32{0x07, 0x0C, 0x03}},       // 1.20.2: registry, tags, finish
		{protocol: 766, wantClientbound: []int32{0x07, 0x0E, 0x03}},       // 1.20.5: registry, tags, finish
		{protocol: 772, wantClientbound: []int32{0x08, 0x0E, 0x0F, 0x04}}, // 1.21.7: registry, flags, tags, finish
		{protocol: 776, wantClientbound: []int32{0x08, 0x0E, 0x0F, 0x04}}, // 26.2: registry, flags, tags, finish
	}

	for _, tt := range tests {
		tt := tt
		t.Run(fmt.Sprintf("protocol%d", tt.protocol), func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()

			opts := testOptions(tt.protocol)

			// Stub client: read every clientbound config packet the server
			// sends, then send Client Information and Acknowledge Finish
			// Configuration.
			recv := make(chan protocol.Packet, 8)
			clientDone := make(chan error, 1)
			go func() {
				rs := protocol.NewReaderState(client)
				ws := protocol.NewWriterState()
				for range tt.wantClientbound {
					pkt, err := rs.ReadPacket()
					if err != nil {
						clientDone <- err
						return
					}
					recv <- pkt
				}
				ci, err := ws.WritePacket(sConfigClientInformation, buildClientInformation())
				if err != nil {
					clientDone <- err
					return
				}
				if _, err := client.Write(ci); err != nil {
					clientDone <- err
					return
				}
				ack, err := ws.WritePacket(sConfigAckFinishConfig, nil)
				if err != nil {
					clientDone <- err
					return
				}
				if _, err := client.Write(ack); err != nil {
					clientDone <- err
					return
				}
				clientDone <- nil
			}()

			err := RunConfig(context.Background(), server,
				protocol.NewReaderState(server), protocol.NewWriterState(), opts)
			if err != nil {
				t.Fatalf("RunConfig: %v", err)
			}

			for i, want := range tt.wantClientbound {
				got := expectPacket(t, recv, 3*time.Second)
				if got.ID != want {
					t.Errorf("clientbound packet[%d] id = 0x%x, want 0x%x", i, got.ID, want)
				}
			}
			if err := <-clientDone; err != nil {
				t.Fatalf("stub client: %v", err)
			}
		})
	}
}

func TestRunConfigIgnoresStrayPackets(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	clientDone := make(chan error, 1)
	go func() {
		rs := protocol.NewReaderState(client)
		ws := protocol.NewWriterState()
		// Drain the four clientbound config packets.
		for i := 0; i < 4; i++ {
			if _, err := rs.ReadPacket(); err != nil {
				clientDone <- err
				return
			}
		}
		// Client Information, then a Plugin Message (0x03) that the table says
		// to ignore, then the acknowledge. The stray packet must not abort.
		stray := []struct {
			id      int32
			payload []byte
		}{
			{id: sConfigClientInformation, payload: buildClientInformation()},
			{id: sConfigPluginMessage, payload: []byte{0x00}}, // arbitrary plugin channel marker
			{id: sConfigAckFinishConfig, payload: nil},
		}
		for _, p := range stray {
			frame, err := ws.WritePacket(p.id, p.payload)
			if err != nil {
				clientDone <- err
				return
			}
			if _, err := client.Write(frame); err != nil {
				clientDone <- err
				return
			}
		}
		clientDone <- nil
	}()

	err := RunConfig(context.Background(), server,
		protocol.NewReaderState(server), protocol.NewWriterState(),
		testOptions(776))
	if err != nil {
		t.Fatalf("RunConfig: %v", err)
	}
	if err := <-clientDone; err != nil {
		t.Fatalf("stub client: %v", err)
	}
}

func TestRunConfigOverEncryptedConn(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	secret := []byte("0123456789abcdef") // 16-byte AES secret
	enc, err := auth.NewEncryptor(secret)
	if err != nil {
		t.Fatalf("NewEncryptor: %v", err)
	}
	// Reuse the cipher reader/writer for the whole handshake. The framing does
	// many separate Read/Write calls, and AES/CFB8 is a streaming cipher whose
	// keystream depends on prior ciphertext, so the stream state must persist
	// across calls.
	serverEnc := &streamConn{Conn: server, r: enc.Reader(server), w: enc.Writer(server)}
	clientEnc := &streamConn{Conn: client, r: enc.Reader(client), w: enc.Writer(client)}

	recv := make(chan protocol.Packet, 8)
	clientDone := make(chan error, 1)
	go func() {
		rs := protocol.NewReaderState(clientEnc)
		ws := protocol.NewWriterState()
		for i := 0; i < 4; i++ {
			pkt, err := rs.ReadPacket()
			if err != nil {
				clientDone <- err
				return
			}
			recv <- pkt
		}
		ack, err := ws.WritePacket(sConfigClientInformation, buildClientInformation())
		if err != nil {
			clientDone <- err
			return
		}
		if _, err := clientEnc.Write(ack); err != nil {
			clientDone <- err
			return
		}
		ack2, err := ws.WritePacket(sConfigAckFinishConfig, nil)
		if err != nil {
			clientDone <- err
			return
		}
		if _, err := clientEnc.Write(ack2); err != nil {
			clientDone <- err
			return
		}
		clientDone <- nil
	}()

	err = RunConfig(context.Background(), serverEnc,
		protocol.NewReaderState(serverEnc), protocol.NewWriterState(),
		testOptions(776))
	if err != nil {
		t.Fatalf("RunConfig over encrypted conn: %v", err)
	}
	for i, want := range []int32{0x08, 0x0E, 0x0F, 0x04} {
		got := expectPacket(t, recv, 3*time.Second)
		if got.ID != want {
			t.Errorf("encrypted clientbound packet[%d] id = 0x%x, want 0x%x", i, got.ID, want)
		}
	}
	if err := <-clientDone; err != nil {
		t.Fatalf("stub client: %v", err)
	}
}

// streamConn adapts a net.Conn so reads and writes route through persistent
// cipher streams, preserving the AES/CFB8 keystream across framing calls.
type streamConn struct {
	net.Conn
	r io.Reader
	w io.Writer
}

func (c *streamConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *streamConn) Write(p []byte) (int, error) { return c.w.Write(p) }

func TestRunConfigUnsupportedLayout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Protocol 763 (1.20.1) has no configuration phase; RunConfig must return a
	// clean error rather than send play packets or panic.
	err := RunConfig(context.Background(), server,
		protocol.NewReaderState(server), protocol.NewWriterState(),
		testOptions(763))
	if err == nil {
		t.Fatal("expected error for protocol without a configuration layout")
	}
	if _, ok := err.(*ConfigError); !ok {
		t.Fatalf("expected *ConfigError, got %T: %v", err, err)
	}
}

func TestRunConfigTimesOut(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		rs := protocol.NewReaderState(client)
		// Read the clientbound packets but never acknowledge; the server must
		// time out instead of hanging.
		for {
			if _, err := rs.ReadPacket(); err != nil {
				return
			}
		}
	}()

	opts := testOptions(776)
	opts.Timeout = 200 * time.Millisecond
	start := time.Now()
	err := RunConfig(context.Background(), server,
		protocol.NewReaderState(server), protocol.NewWriterState(), opts)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("handshake did not time out promptly: %v", elapsed)
	}
	// Close the client to unblock the reader goroutine, then wait for it to
	// exit.
	client.Close()
	select {
	case <-clientDone:
	case <-time.After(2 * time.Second):
		t.Fatal("stub client did not exit")
	}
}

func testOptions(protocolID int32) Options {
	return Options{
		Logger:  log.New(io.Discard, "", 0),
		Version: protocol.Version{Protocol: protocolID, HasConfigurationPhase: true},
	}
}

// buildClientInformation builds a minimal Client Information payload. The
// exact schema is not validated; it only needs to be a non-nil payload so the
// server has something to consume.
func buildClientInformation() []byte {
	w := protocol.NewWriter()
	_ = protocol.WriteString(w, "en_us")
	_ = w.WriteByte(0) // view distance
	_ = w.WriteByte(0) // chat mode
	_ = w.WriteByte(0) // main hand
	return w.Bytes()
}

func expectPacket(t *testing.T, recv <-chan protocol.Packet, d time.Duration) protocol.Packet {
	t.Helper()
	select {
	case p := <-recv:
		return p
	case <-time.After(d):
		t.Fatal("timed out waiting for packet")
		return protocol.Packet{}
	}
}
