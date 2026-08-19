// Package proxy orchestrates a Minecraft protocol session for a client
// connecting through the gateway: it terminates the login, runs the limbo
// void, and warps the player into the real backend once it is ready. It builds
// on the auth, limbo, transfer, and protocol sub-packages.
package proxy

import (
	"context"
	"crypto/rand"
	"log"
	"net"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
	"github.com/mcm-panel/mcm/internal/proxy/limbo"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
	"github.com/mcm-panel/mcm/internal/proxy/transfer"
)

// SessionOptions configures one proxy client session.
type SessionOptions struct {
	Logger         *log.Logger
	Protocol       protocol.Version
	OnlineMode     bool
	Session        auth.SessionClient
	Message        string
	BackendConn    func(ctx context.Context) (net.Conn, error)
	HoldTimeout    time.Duration
	KeepAliveEvery time.Duration
	IdleDeadline   time.Duration
}

// HandleClient runs the full proxy session for an already-routed login
// connection. The client's Handshake has already been consumed by the caller;
// conn's next bytes are the Login Start packet.
func HandleClient(ctx context.Context, conn net.Conn, opts SessionOptions) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.HoldTimeout <= 0 {
		opts.HoldTimeout = 90 * time.Second
	}
	if opts.OnlineMode && opts.Session == nil {
		opts.Session = auth.NewMojangClient()
	}

	// Phase 1: login handshake with the client (auth).
	state, err := loginClient(ctx, conn, opts)
	if err != nil {
		return err
	}

	// Phase 2: limbo play state (End void + per-server message).
	limboSession := limbo.NewSession(state.conn, state.rs, state.ws, limbo.Options{
		Logger:         opts.Logger,
		Profile:        state.Profile,
		Message:        opts.Message,
		KeepAliveEvery: opts.KeepAliveEvery,
	})
	if err := limboSession.Start(); err != nil {
		return err
	}

	// Phase 3: wait for the backend and warp the player in.
	return transfer.Warp(ctx, state.conn, state.rs, state.ws, transfer.Options{
		Logger:       opts.Logger,
		Protocol:     opts.Protocol,
		Profile:      state.Profile,
		BackendConn:  opts.BackendConn,
		HoldTimeout:  opts.HoldTimeout,
		IdleDeadline: opts.IdleDeadline,
	})
}

// clientState carries the possibly-encrypted connection, the live frame state
// machines, and the authenticated profile after the login phase.
type clientState struct {
	conn    net.Conn
	rs      *protocol.ReaderState
	ws      *protocol.WriterState
	Profile auth.Profile
}

// loginClient terminates the client login: it reads Login Start, performs
// online or offline auth, sends Login Success (enabling encryption for online
// mode), and returns the (possibly encrypted) connection plus frame state.
func loginClient(ctx context.Context, conn net.Conn, opts SessionOptions) (*clientState, error) {
	rs := protocol.NewReaderState(conn)
	ws := protocol.NewWriterState()

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	start, err := rs.ReadPacket()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, err
	}
	if start.ID != 0x00 {
		return nil, &loginError{reason: "expected Login Start"}
	}
	name, err := readLoginStartName(start.Payload)
	if err != nil {
		return nil, err
	}

	profile := auth.Profile{Name: name, UUID: auth.OfflineUUID(name), OnlineMode: false}
	if opts.OnlineMode {
		var encConn net.Conn
		encConn, profile, err = onlineAuth(ctx, conn, rs, ws, name, opts.Session)
		if err != nil {
			return nil, err
		}
		conn = encConn
		// Encryption established; frame states are rebuilt over the cipher
		// connection.
		rs = protocol.NewReaderState(conn)
		ws = protocol.NewWriterState()
	} else {
		if err := writeLoginSuccess(conn, ws, profile); err != nil {
			return nil, err
		}
	}
	return &clientState{conn: conn, rs: rs, ws: ws, Profile: profile}, nil
}

// onlineAuth performs the Mojang online-mode handshake and switches the
// connection to encryption. It returns the (now encrypted) connection and the
// verified profile.
func onlineAuth(ctx context.Context, conn net.Conn, rs *protocol.ReaderState, ws *protocol.WriterState, name string, session auth.SessionClient) (net.Conn, auth.Profile, error) {
	if session == nil {
		session = auth.NewMojangClient()
	}
	pair, err := auth.GenerateRSAPair()
	if err != nil {
		return nil, auth.Profile{}, err
	}
	verifyToken := make([]byte, 4)
	if _, err := rand.Read(verifyToken); err != nil {
		return nil, auth.Profile{}, err
	}
	pubKey := pair.PublicKeyDER()
	if pubKey == nil {
		return nil, auth.Profile{}, &loginError{reason: "failed to marshal public key"}
	}
	reqPayload, err := buildLoginEncryptionRequest(pubKey, verifyToken)
	if err != nil {
		return nil, auth.Profile{}, err
	}
	frame, err := ws.WritePacket(0x01, reqPayload)
	if err != nil {
		return nil, auth.Profile{}, err
	}
	if _, err := conn.Write(frame); err != nil {
		return nil, auth.Profile{}, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	resp, err := rs.ReadPacket()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		return nil, auth.Profile{}, err
	}
	if resp.ID != 0x01 {
		return nil, auth.Profile{}, &loginError{reason: "expected Login Encryption Response"}
	}
	sharedSecret, gotToken, err := pair.DecryptResponse(resp.Payload)
	if err != nil {
		return nil, auth.Profile{}, err
	}
	if !equalBytes(gotToken, verifyToken) {
		return nil, auth.Profile{}, &loginError{reason: "invalid verify token"}
	}

	serverID := auth.ServerHash(" ", sharedSecret, pubKey)
	profile, err := session.HasJoined(ctx, name, serverID)
	if err != nil {
		return nil, auth.Profile{}, err
	}

	enc, err := auth.NewEncryptor(sharedSecret)
	if err != nil {
		return nil, auth.Profile{}, err
	}
	encConn := wrapConnEncrypted(conn, enc)
	if err := writeLoginSuccess(encConn, ws, profile); err != nil {
		return nil, auth.Profile{}, err
	}
	return encConn, profile, nil
}

// writeLoginSuccess writes a Login Success packet to the client.
func writeLoginSuccess(w interface{ Write([]byte) (int, error) }, ws *protocol.WriterState, profile auth.Profile) error {
	payload, err := buildLoginSuccess(profile)
	if err != nil {
		return err
	}
	frame, err := ws.WritePacket(0x02, payload)
	if err != nil {
		return err
	}
	_, err = w.Write(frame)
	return err
}

// loginError is a player-facing login failure.
type loginError struct{ reason string }

func (e *loginError) Error() string { return "login failed: " + e.reason }

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
