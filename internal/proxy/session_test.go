package proxy

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// TestHandleClientOfflineConfigPhase exercises HandleClient end-to-end for a
// configuration-phase version (protocol 776 "26.2") in offline mode across an
// in-process net.Pipe. It verifies the login (Login Start -> Login Success),
// the configuration-state handshake (Registry Data/Feature Flags/Update
// Tags/Finish Configuration, then the client's Client Information + Acknowledge
// Finish Configuration), and the transition into the play-state limbo (a Join
// Game / first limbo play packet is emitted).
//
// Play packet IDs asserted here use the protocol 763 table (Join Game 0x28),
// which is all the limbo has; see the caveat in session.go and the gateway
// spec about unvalidated play IDs for 764+ revisions.
func TestHandleClientOfflineConfigPhase(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// BackendConn blocks until the context is cancelled. The limbo packets are
	// sent before Warp ever dials the backend, so the test can observe them
	// without a real backend.
	backendDialed := make(chan struct{}, 1)
	backendConn := func(c context.Context) (net.Conn, error) {
		close(backendDialed)
		<-c.Done()
		return nil, c.Err()
	}

	opts := SessionOptions{
		Logger:         log.New(io.Discard, "", 0),
		Protocol:       protocol.Version{Protocol: 776, Name: "26.2", HasConfigurationPhase: true, SupportsTransfer: true},
		Message:        "Waking up the world...",
		BackendConn:    backendConn,
		HoldTimeout:    2 * time.Second,
		KeepAliveEvery: 30 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- HandleClient(ctx, server, opts)
	}()

	// Client side: drive login, configuration, and read the limbo packets.
	recv := make(chan protocol.Packet, 16)
	clientDone := make(chan error, 1)
	go func() {
		rs := protocol.NewReaderState(client)
		ws := protocol.NewWriterState()

		// 1. Login Start (serverbound login 0x00): name + holds-signature bool.
		loginPayload := protocol.NewWriter()
		if err := protocol.WriteString(loginPayload, "Steve"); err != nil {
			clientDone <- err
			return
		}
		_ = loginPayload.WriteByte(0)
		ls, err := ws.WritePacket(0x00, loginPayload.Bytes())
		if err != nil {
			clientDone <- err
			return
		}
		if _, err := client.Write(ls); err != nil {
			clientDone <- err
			return
		}

		// 2. Login Success (clientbound login 0x02).
		succ, err := rs.ReadPacket()
		if err != nil {
			clientDone <- err
			return
		}
		recv <- succ

		// 3. Configuration clientbound packets for layout 769+ (protocol 776):
		// Registry Data 0x08, Feature Flags 0x0E, Update Tags 0x0F, Finish
		// Configuration 0x04.
		for _, want := range []int32{0x08, 0x0E, 0x0F, 0x04} {
			pkt, err := rs.ReadPacket()
			if err != nil {
				clientDone <- err
				return
			}
			if pkt.ID != want {
				clientDone <- &protocolVersionError{got: pkt.ID, want: want}
				return
			}
			recv <- pkt
		}

		// 4. Client Information (serverbound config 0x00), then Acknowledge
		// Finish Configuration (serverbound config 0x01).
		ci, err := ws.WritePacket(0x00, buildTestClientInformation())
		if err != nil {
			clientDone <- err
			return
		}
		if _, err := client.Write(ci); err != nil {
			clientDone <- err
			return
		}
		ack, err := ws.WritePacket(0x01, nil)
		if err != nil {
			clientDone <- err
			return
		}
		if _, err := client.Write(ack); err != nil {
			clientDone <- err
			return
		}

		// 5. Play-state limbo: Join Game (0x28 in the 763 table) then the rest
		// of the limbo handshake. The proxy's Start() sends Join Game, Player
		// Info, Player Position, and (optionally) the actionbar message; the
		// client must drain them all so the proxy can proceed to Warp.
		for i := 0; i < 4; i++ {
			pkt, err := rs.ReadPacket()
			if err != nil {
				clientDone <- err
				return
			}
			recv <- pkt
		}
		clientDone <- nil
	}()

	// Expect Login Success first.
	succ := expectSessionPacket(t, recv, 3*time.Second)
	if succ.ID != 0x02 {
		t.Fatalf("expected Login Success (0x02), got 0x%x", succ.ID)
	}

	// The four configuration clientbound packets are read and asserted inside
	// the client goroutine (it errors on a mismatch), so drain them here.
	for i := 0; i < 4; i++ {
		expectSessionPacket(t, recv, 3*time.Second)
	}

	// The play limbo must emit a Join Game (0x28 in the 763 table) first, after
	// the config handshake completes. Drain the remaining three limbo packets
	// so the proxy can finish Start and reach Warp.
	join := expectSessionPacket(t, recv, 3*time.Second)
	if join.ID != 0x28 {
		t.Fatalf("expected play limbo Join Game (0x28, 763 table), got 0x%x", join.ID)
	}
	for i := 0; i < 3; i++ {
		expectSessionPacket(t, recv, 3*time.Second)
	}

	// Ensure the client goroutine reported no protocol error.
	if err := <-clientDone; err != nil {
		t.Fatalf("client stub: %v", err)
	}

	// Let HandleClient settle into Warp (blocking on the backend), then stop
	// it so the test exits cleanly.
	select {
	case <-backendDialed:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleClient never attempted to dial the backend")
	}
	cancel()
	select {
	case <-serverErr:
	case <-time.After(2 * time.Second):
		t.Fatal("HandleClient did not return after cancel")
	}
}

func expectSessionPacket(t *testing.T, recv <-chan protocol.Packet, d time.Duration) protocol.Packet {
	t.Helper()
	select {
	case p := <-recv:
		return p
	case <-time.After(d):
		t.Fatal("timed out waiting for packet")
		return protocol.Packet{}
	}
}

func buildTestClientInformation() []byte {
	w := protocol.NewWriter()
	_ = protocol.WriteString(w, "en_us")
	_ = w.WriteByte(0)
	_ = w.WriteByte(0)
	_ = w.WriteByte(0)
	return w.Bytes()
}

// protocolVersionError reports an unexpected packet ID.
type protocolVersionError struct {
	got  int32
	want int32
}

func (e *protocolVersionError) Error() string {
	return "unexpected packet id: got 0x" + itoaHex(e.got) + ", want 0x" + itoaHex(e.want)
}

func itoaHex(v int32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	u := uint32(v)
	for u > 0 {
		i--
		buf[i] = digits[u&0xf]
		u >>= 4
	}
	return string(buf[i:])
}
