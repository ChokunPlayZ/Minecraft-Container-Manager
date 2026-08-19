package proxy

import (
	"net"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/auth"
)

// encryptedConn wraps a net.Conn so bytes read are decrypted and bytes written
// are encrypted with the AES/CFB8 cipher negotiated during online-mode login.
// It transparently forwards deadlines and close semantics to the underlying
// connection.
type encryptedConn struct {
	net.Conn
	enc *auth.Encryptor
}

func (e *encryptedConn) Read(p []byte) (int, error) {
	return e.enc.Reader(e.Conn).Read(p)
}

func (e *encryptedConn) Write(p []byte) (int, error) {
	return e.enc.Writer(e.Conn).Write(p)
}

func (e *encryptedConn) SetDeadline(t time.Time) error     { return e.Conn.SetDeadline(t) }
func (e *encryptedConn) SetReadDeadline(t time.Time) error { return e.Conn.SetReadDeadline(t) }
func (e *encryptedConn) SetWriteDeadline(t time.Time) error {
	return e.Conn.SetWriteDeadline(t)
}

// wrapConnEncrypted returns conn wrapped so subsequent reads/writes use the
// negotiated cipher.
func wrapConnEncrypted(conn net.Conn, enc *auth.Encryptor) net.Conn {
	return &encryptedConn{Conn: conn, enc: enc}
}
