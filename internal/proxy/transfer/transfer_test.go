package transfer

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

func TestBridgeLogsIntoBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientServer, _ := net.Pipe()
	defer clientServer.Close()
	backendServer, backendProxy := net.Pipe()
	defer backendServer.Close()

	profile := auth.Profile{Name: "Alex", UUID: auth.OfflineUUID("Alex")}
	gotName := make(chan string, 1)
	go func() {
		rs := protocol.NewReaderState(backendServer)
		ws := protocol.NewWriterState()
		// Handshake.
		if _, err := rs.ReadPacket(); err != nil {
			return
		}
		// Login Start.
		start, err := rs.ReadPacket()
		if err != nil {
			return
		}
		name, err := protocol.ReadString(protocol.NewByteReader(start.Payload))
		if err != nil {
			return
		}
		gotName <- name
		// Reply Login Success.
		w := protocol.NewWriter()
		_ = protocol.WriteString(w, profile.UUID.String())
		_ = protocol.WriteString(w, name)
		_ = protocol.WriteVarInt(w, 0)
		frame, _ := ws.WritePacket(0x02, w.Bytes())
		_, _ = backendServer.Write(frame)
		io.Copy(io.Discard, backendServer)
	}()

	opts := Options{
		Logger:       log.New(io.Discard, "", 0),
		Protocol:     protocol.Version{Protocol: 763, Name: "1.20.1"},
		Profile:      profile,
		BackendConn:  func(context.Context) (net.Conn, error) { return backendProxy, nil },
		HoldTimeout:  3 * time.Second,
		IdleDeadline: time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- Warp(ctx, clientServer, protocol.NewReaderState(clientServer), protocol.NewWriterState(), opts)
	}()

	select {
	case name := <-gotName:
		if name != "Alex" {
			t.Errorf("backend login name = %q, want Alex", name)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("backend did not receive login")
	}
}
