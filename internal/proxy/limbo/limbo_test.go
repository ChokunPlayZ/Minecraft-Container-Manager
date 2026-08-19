package limbo

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

func TestLimboSendsVoidHandshakeAndKeepAlive(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	profile := auth.Profile{Name: "Steve", UUID: auth.OfflineUUID("Steve")}
	sess := NewSession(server, protocol.NewReaderState(server), protocol.NewWriterState(), Options{
		Logger:          log.New(io.Discard, "", 0),
		Profile:         profile,
		Message:         "Waking up the world...",
		KeepAliveEvery:  20 * time.Millisecond,
		ReadIdleTimeout: time.Second,
	})

	// Client side reads frames concurrently (net.Pipe blocks writes until the
	// peer reads).
	recv := make(chan protocol.Packet, 16)
	go func() {
		rs := protocol.NewReaderState(client)
		for {
			pkt, err := rs.ReadPacket()
			if err != nil {
				return
			}
			recv <- pkt
		}
	}()

	if err := sess.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serviceDone := make(chan struct{})
	go func() {
		_ = sess.Service(ctx)
		close(serviceDone)
	}()

	// Expect Join Game, Player Info, Player Position, then actionbar message.
	want := []int32{cPlayJoinGame, cPlayPlayerInfo, cPlayPlayerPosition, cPlaySetActionbar}
	for _, id := range want {
		got := expectPacket(t, recv, 3*time.Second)
		if got.ID != id {
			t.Errorf("packet id = 0x%x, want 0x%x", got.ID, id)
		}
	}

	// A periodic keep-alive should arrive.
	ka := expectPacket(t, recv, 3*time.Second)
	if ka.ID != cPlayKeepAlive {
		t.Fatalf("expected keep-alive, got 0x%x", ka.ID)
	}

	// Respond with a keep-alive and expect the proxy to echo it back.
	ws := protocol.NewWriterState()
	frame, err := ws.WritePacket(sPlayKeepAlive, ka.Payload)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	if _, err := client.Write(frame); err != nil {
		t.Fatalf("write response: %v", err)
	}
	echo := expectPacket(t, recv, 3*time.Second)
	if echo.ID != cPlayKeepAlive || !bytes.Equal(echo.Payload, ka.Payload) {
		t.Errorf("keep-alive echo mismatch: id=0x%x payload=%x", echo.ID, echo.Payload)
	}

	// Drain the goroutine so the test exits cleanly.
	cancel()
	select {
	case <-serviceDone:
	case <-time.After(2 * time.Second):
	}
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
