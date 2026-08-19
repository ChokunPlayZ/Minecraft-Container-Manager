package auth

import (
	"crypto/rand"
	"errors"
)

// EncryptionRequest is the decoded payload of a server's Login Encryption
// Request, as received when the proxy logs into an online-mode backend.
type EncryptionRequest struct {
	ServerID    string
	PublicKey   []byte
	VerifyToken []byte
}

// ParseEncryptionRequest decodes a Login Encryption Request payload. Newer
// protocol revisions append a trailing shouldAuthenticate flag (and a boolean
// for 1.20.5+); parsing is lenient and ignores any trailing fields.
func ParseEncryptionRequest(payload []byte) (*EncryptionRequest, error) {
	r := payload
	serverID, n, err := readVarIntString(r)
	if err != nil {
		return nil, err
	}
	r = r[n:]
	keyLen, n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	r = r[n:]
	if keyLen <= 0 || int(keyLen) > len(r) {
		return nil, errors.New("invalid encryption request public key length")
	}
	pubKey := r[:keyLen]
	r = r[keyLen:]
	tokLen, n, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	r = r[n:]
	if tokLen <= 0 || int(tokLen) > len(r) {
		return nil, errors.New("invalid encryption request verify token length")
	}
	verifyToken := r[:tokLen]
	return &EncryptionRequest{ServerID: serverID, PublicKey: pubKey, VerifyToken: verifyToken}, nil
}

// BuildEncryptionResponse is the client-side counterpart to
// RSAPair.DecryptResponse: it generates a fresh 16-byte shared secret,
// encrypts it and the server's verify token with the server's public key, and
// returns the shared secret plus the Login Encryption Response payload to send
// back to the server.
func BuildEncryptionResponse(req *EncryptionRequest) (sharedSecret []byte, payload []byte, err error) {
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		return nil, nil, err
	}
	encSecret, err := EncryptWithPublicKey(req.PublicKey, secret)
	if err != nil {
		return nil, nil, err
	}
	encToken, err := EncryptWithPublicKey(req.PublicKey, req.VerifyToken)
	if err != nil {
		return nil, nil, err
	}
	w := newVarIntWriter()
	w.writeVarInt(int32(len(encSecret)))
	w.writeBytes(encSecret)
	w.writeVarInt(int32(len(encToken)))
	w.writeBytes(encToken)
	return secret, w.buf, nil
}

// readVarIntString reads a length-prefixed UTF-8 string from b.
func readVarIntString(b []byte) (string, int, error) {
	n, consumed, err := readVarInt(b)
	if err != nil {
		return "", 0, err
	}
	if n < 0 || int(n) > len(b)-consumed {
		return "", 0, errors.New("invalid string length")
	}
	return string(b[consumed : consumed+int(n)]), consumed + int(n), nil
}

type varIntWriter struct {
	buf []byte
}

func newVarIntWriter() *varIntWriter { return &varIntWriter{} }

func (w *varIntWriter) writeVarInt(v int32) {
	u := uint32(v)
	for {
		b := byte(u & 0x7F)
		u >>= 7
		if u != 0 {
			b |= 0x80
		}
		w.buf = append(w.buf, b)
		if u == 0 {
			return
		}
	}
}

func (w *varIntWriter) writeBytes(b []byte) {
	w.buf = append(w.buf, b...)
}
