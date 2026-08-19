// Package limbo runs the proxy's play-state limbo: it drops an authenticated
// player into an empty End dimension in spectator mode with the per-server
// wait message and keeps the connection alive until the real backend is ready.
package limbo

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// Options configures a limbo session's play state.
type Options struct {
	Logger          *log.Logger
	Profile         auth.Profile
	Message         string
	KeepAliveEvery  time.Duration
	ReadIdleTimeout time.Duration
}

// Session owns the play-state limbo for one connection. It is not safe for
// concurrent use except as documented below.
type Session struct {
	conn    net.Conn
	rs      *protocol.ReaderState
	ws      *protocol.WriterState
	opts    Options
	log     *log.Logger
	writeMu sync.Mutex
}

// NewSession returns a limbo session over an already-logged-in client
// connection. rs and ws are the live frame state machines (compression and
// encryption already negotiated).
func NewSession(conn net.Conn, rs *protocol.ReaderState, ws *protocol.WriterState, opts Options) *Session {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.KeepAliveEvery <= 0 {
		opts.KeepAliveEvery = 15 * time.Second
	}
	if opts.ReadIdleTimeout <= 0 {
		opts.ReadIdleTimeout = 30 * time.Second
	}
	return &Session{conn: conn, rs: rs, ws: ws, opts: opts, log: opts.Logger}
}

// Start drops the player into the End void and returns once the play handshake
// has been sent. It is idempotent and safe to call once.
func (s *Session) Start() error {
	join, err := buildJoinGame(s.opts.Profile)
	if err != nil {
		return err
	}
	if err := s.writeFrame(cPlayJoinGame, join); err != nil {
		return err
	}
	info, err := buildPlayerInfoAdd(s.opts.Profile)
	if err != nil {
		return err
	}
	if err := s.writeFrame(cPlayPlayerInfo, info); err != nil {
		return err
	}
	pos, err := buildPlayerPosition(0.5, 64, 0.5, 0, 0)
	if err != nil {
		return err
	}
	if err := s.writeFrame(cPlayPlayerPosition, pos); err != nil {
		return err
	}
	if s.opts.Message != "" {
		bar, err := buildActionbarMessage(s.opts.Message)
		if err != nil {
			return err
		}
		if err := s.writeFrame(cPlaySetActionbar, bar); err != nil {
			return err
		}
	}
	return nil
}

// Service runs the keep-alive loop until ctx ends or the client disconnects.
// It responds to client keep-alives and periodically sends its own so the
// client does not time out while the backend boots. Movement and other play
// packets are ignored.
func (s *Session) Service(ctx context.Context) error {
	keepAliveID := int64(time.Now().Unix())
	kaCtx, kaCancel := context.WithCancel(ctx)
	defer kaCancel()

	// Background writer for periodic keep-alives.
	errc := make(chan error, 1)
	go func() {
		t := time.NewTicker(s.opts.KeepAliveEvery)
		defer t.Stop()
		for {
			select {
			case <-kaCtx.Done():
				errc <- nil
				return
			case <-t.C:
				keepAliveID++
				if err := s.writeFrame(cPlayKeepAlive, buildKeepAlive(keepAliveID)); err != nil {
					errc <- err
					return
				}
			}
		}
	}()

	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(s.opts.ReadIdleTimeout))
		pkt, err := s.rs.ReadPacket()
		if err != nil {
			// A timeout with no traffic is tolerated; a closed socket ends.
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		switch pkt.ID {
		case sPlayKeepAlive:
			// Echo the client's keep-alive id back so the client stays lively.
			if err := s.writeFrame(cPlayKeepAlive, pkt.Payload); err != nil {
				return err
			}
		default:
			// Movement, teleport confirm, client status, etc. are ignored in
			// limbo.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

// writeFrame encodes and writes one frame under the write mutex.
func (s *Session) writeFrame(id int32, payload []byte) error {
	frame, err := s.ws.WritePacket(id, payload)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.conn.Write(frame)
	return err
}

// WriteFrame allows the transfer step to inject packets (e.g. a goodbye) into
// the client stream while the limbo is active.
func (s *Session) WriteFrame(id int32, payload []byte) error {
	return s.writeFrame(id, payload)
}
