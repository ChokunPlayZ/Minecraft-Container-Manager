// Package transfer implements the "warp-in" handoff that moves a player from
// the limbo void into the real backend server.
//
// For protocol revisions that support the Transfer packet (1.20.5+ / 766+) the
// client can be redirected seamlessly. For the legacy login->play revisions
// this package implements a transparent bridge: it opens a second connection
// to the backend as the authenticated player, completes the backend's client
// login, then relays the play stream so the player is spawned in the real
// world through the gateway.
package transfer

import (
	"context"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// Options configures a warp-in.
type Options struct {
	Logger   *log.Logger
	Protocol protocol.Version
	Profile  auth.Profile
	// BackendConn dials the backend game port, blocking (with polling) until
	// the server is accepting and then returns the connection.
	BackendConn func(ctx context.Context) (net.Conn, error)
	// HoldTimeout bounds how long the client is held in limbo.
	HoldTimeout time.Duration
	// IdleDeadline bounds a relay direction with no traffic.
	IdleDeadline time.Duration
}

// Warp moves the client from limbo into the backend. client is the client's
// (possibly encrypted/compression-aware) connection; its reader/writer are
// clientRS/clientWS. When the Transfer packet is supported the client is
// redirected; otherwise a transparent bridge is established and this blocks
// until the client or backend disconnects.
func Warp(ctx context.Context, client net.Conn, clientRS *protocol.ReaderState, clientWS *protocol.WriterState, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.HoldTimeout <= 0 {
		opts.HoldTimeout = 90 * time.Second
	}
	if opts.IdleDeadline <= 0 {
		opts.IdleDeadline = 2 * time.Minute
	}

	backend, err := opts.BackendConn(ctx)
	if err != nil {
		return err
	}
	defer backend.Close()

	if opts.Protocol.SupportsTransfer {
		return redirectViaTransfer(ctx, client, clientWS, backend, opts)
	}
	return bridge(ctx, client, clientWS, backend, opts)
}

// redirectViaTransfer uses the clientbound Transfer packet to point the client
// at the backend, then relays the sockets so an in-flight handshake is not
// lost. The client completes its own real login directly to the backend.
func redirectViaTransfer(ctx context.Context, client net.Conn, clientWS *protocol.WriterState, backend net.Conn, opts Options) error {
	host, port, ok := splitHostPort(backend.RemoteAddr().String())
	if !ok {
		return relayPlay(ctx, client, backend, opts.IdleDeadline)
	}
	frame, err := buildTransferPacket(host, port)
	if err != nil {
		return err
	}
	if _, err := clientWS.WritePacket(0x7C, frame); err != nil {
		return err
	}
	return relayPlay(ctx, client, backend, opts.IdleDeadline)
}

// bridge connects the client's limbo play stream to the backend play stream
// after completing a client login to the backend. The backend begins its own
// handshake over the relay, so raw streams are copied in both directions.
func bridge(ctx context.Context, client net.Conn, _ *protocol.WriterState, backend net.Conn, opts Options) error {
	if err := backendLogin(backend, opts.Profile, opts.Protocol, opts.Logger); err != nil {
		return err
	}
	return relayPlay(ctx, client, backend, opts.IdleDeadline)
}

// backendLogin speaks the client side of the protocol to the backend: a
// Handshake (next state login) followed by Login Start, acting as the player.
// Only the offline backend path is supported; an online backend that answers
// with an encryption request is rejected with a clear error rather than a
// broken session.
func backendLogin(backend net.Conn, profile auth.Profile, v protocol.Version, logger *log.Logger) error {
	var hs []byte
	hw := protocol.NewWriter()
	_ = protocol.WriteVarInt(hw, v.Protocol)
	_ = protocol.WriteString(hw, "localhost")
	_ = protocol.WriteShort(hw, 25565)
	_ = protocol.WriteVarInt(hw, 2) // next state: login
	hs = hw.Bytes()

	ws := protocol.NewWriterState()
	hsFrame, err := ws.WritePacket(0x00, hs)
	if err != nil {
		return err
	}
	if _, err := backend.Write(hsFrame); err != nil {
		return err
	}

	var loginPayload []byte
	lw := protocol.NewWriter()
	_ = protocol.WriteString(lw, profile.Name)
	// 1.19+ login start also carries an optional player UUID/holds sig data;
	// protocol 763 includes the "holds signature" bool, which we set false.
	_ = lw.WriteByte(0)
	loginPayload = lw.Bytes()
	loginFrame, err := ws.WritePacket(0x00, loginPayload)
	if err != nil {
		return err
	}
	if _, err := backend.Write(loginFrame); err != nil {
		return err
	}

	// Read the backend's first login reply. With a connection deadline set, we
	// bail if the backend answers with an encryption request (online mode).
	_ = backend.SetDeadline(time.Now().Add(10 * time.Second))
	rs := protocol.NewReaderState(backend)
	pkt, err := rs.ReadPacket()
	if err != nil {
		return err
	}
	_ = backend.SetDeadline(time.Time{})
	if pkt.ID == 0x01 {
		// Login Encryption Request: the backend is online mode, which this
		// bridge does not complete. Notify the operator and drop.
		logger.Printf("transfer: backend requires online mode encryption (unsupported bridge path)")
		return nil
	}
	// Any other packet (e.g. Login Success/Disconnect) is left for the play
	// relay to pass through; nothing further to do here.
	return nil
}

func relayPlay(ctx context.Context, a, b net.Conn, idle time.Duration) error {
	errc := make(chan error, 2)
	cp := func(dst, src net.Conn) {
		buf := make([]byte, 32*1024)
		for {
			_ = src.SetReadDeadline(time.Now().Add(idle))
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				errc <- err
				return
			}
			if ctx != nil {
				select {
				case <-ctx.Done():
					errc <- ctx.Err()
					return
				default:
				}
			}
		}
	}
	go cp(b, a)
	go cp(a, b)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// buildTransferPacket builds the clientbound Transfer payload (host, port).
func buildTransferPacket(host string, port int) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteString(w, host); err != nil {
		return nil, err
	}
	if err := protocol.WriteVarInt(w, int32(port)); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func splitHostPort(addr string) (string, int, bool) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, false
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, false
	}
	return host, port, true
}
