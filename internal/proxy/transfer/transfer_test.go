package transfer

import (
	"context"
	"crypto/rand"
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

// TestBackendLoginOnlineMode runs the full online-mode backend login: the stub
// server issues a real encryption request, the proxy completes it, and the two
// sides switch to AES/CFB8 encryption before the server sends Login Success.
func TestBackendLoginOnlineMode(t *testing.T) {
	backend, backendProxy := net.Pipe()
	defer backend.Close()

	profile := auth.Profile{Name: "Notch", UUID: auth.OfflineUUID("Notch")}
	serverErr := make(chan error, 1)
	go runOnlineBackend(t, backend, profile, serverErr)

	logger := log.New(io.Discard, "", 0)
	v := protocol.Version{Protocol: 763, Name: "1.20.1"}
	err := backendLogin(backendProxy, profile, v, logger)
	backendProxy.Close()
	if err != nil {
		t.Fatalf("backendLogin: %v", err)
	}
	if serr := <-serverErr; serr != nil {
		t.Fatalf("stub backend: %v", serr)
	}
}

// runOnlineBackend drives the server side of an online-mode login over conn.
func runOnlineBackend(t *testing.T, conn net.Conn, profile auth.Profile, errc chan<- error) {
	const loginSuccessID = 0x02
	rs := protocol.NewReaderState(conn)
	ws := protocol.NewWriterState()

	// Handshake.
	if _, err := rs.ReadPacket(); err != nil {
		errc <- err
		return
	}
	// Login Start.
	if _, err := rs.ReadPacket(); err != nil {
		errc <- err
		return
	}

	// Issue a real Login Encryption Request.
	pair, err := auth.GenerateRSAPair()
	if err != nil {
		errc <- err
		return
	}
	verifyToken := make([]byte, 4)
	if _, err := rand.Read(verifyToken); err != nil {
		errc <- err
		return
	}
	req, err := buildEncryptionRequest(pair.PublicKeyDER(), verifyToken)
	if err != nil {
		errc <- err
		return
	}
	frame, err := ws.WritePacket(0x01, req)
	if err != nil {
		errc <- err
		return
	}
	if _, err := conn.Write(frame); err != nil {
		errc <- err
		return
	}

	// Read the proxy's Login Encryption Response (raw, pre-encryption).
	resp, err := rs.ReadPacket()
	if err != nil {
		errc <- err
		return
	}
	if resp.ID != 0x01 {
		errc <- &testError{"expected Login Encryption Response"}
		return
	}
	sharedSecret, gotToken, err := pair.DecryptResponse(resp.Payload)
	if err != nil {
		errc <- err
		return
	}
	if len(sharedSecret) != 16 {
		errc <- &testError{"shared secret not 16 bytes"}
		return
	}
	if string(gotToken) != string(verifyToken) {
		errc <- &testError{"verify token mismatch"}
		return
	}

	// Switch to encryption and send an encrypted Login Success.
	enc, err := auth.NewEncryptor(sharedSecret)
	if err != nil {
		errc <- err
		return
	}
	encConn := auth.WrapConn(conn, enc)
	ew := protocol.NewWriterState()
	success, err := buildLoginSuccessPayload(profile)
	if err != nil {
		errc <- err
		return
	}
	sf, err := ew.WritePacket(loginSuccessID, success)
	if err != nil {
		errc <- err
		return
	}
	if _, err := encConn.Write(sf); err != nil {
		errc <- err
		return
	}
	io.Copy(io.Discard, encConn)
	errc <- nil
}

func buildEncryptionRequest(pubKey, verifyToken []byte) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteString(w, " "); err != nil {
		return nil, err
	}
	if err := protocol.WriteVarInt(w, int32(len(pubKey))); err != nil {
		return nil, err
	}
	if _, err := w.Write(pubKey); err != nil {
		return nil, err
	}
	if err := protocol.WriteVarInt(w, int32(len(verifyToken))); err != nil {
		return nil, err
	}
	if _, err := w.Write(verifyToken); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func buildLoginSuccessPayload(p auth.Profile) ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteString(w, p.UUID.String()); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(w, p.Name); err != nil {
		return nil, err
	}
	if err := protocol.WriteVarInt(w, 0); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
