package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"io"
)

// RSAPair holds a generated RSA keypair used for the login encryption request.
type RSAPair struct {
	Private *rsa.PrivateKey
}

// GenerateRSAPair returns a fresh 1024-bit RSA key. The Minecraft client
// negotiates with a 1024-bit modulus.
func GenerateRSAPair() (*RSAPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}
	return &RSAPair{Private: key}, nil
}

// PublicKeyDER returns the DER-encoded PKIX public key, as sent to the client
// in the Login Encryption Request.
func (p *RSAPair) PublicKeyDER() []byte {
	der, err := x509.MarshalPKIXPublicKey(&p.Private.PublicKey)
	if err != nil {
		return nil
	}
	return der
}

// EncryptResponse decrypts a Login Encryption Response payload and returns the
// shared AES secret and the (encrypted) verify token.
func (p *RSAPair) DecryptResponse(payload []byte) (sharedSecret, verifyToken []byte, err error) {
	r := payload
	// Shared secret is RSA-encrypted secret, length-prefixed varint, then the
	// verify token length-prefixed varint.
	secLen, n, err := readVarInt(r)
	if err != nil {
		return nil, nil, err
	}
	r = r[n:]
	if secLen < 0 || int(secLen) > len(r) {
		return nil, nil, errors.New("invalid shared secret length")
	}
	encSecret := r[:secLen]
	r = r[secLen:]
	tokLen, n, err := readVarInt(r)
	if err != nil {
		return nil, nil, err
	}
	r = r[n:]
	if tokLen < 0 || int(tokLen) > len(r) {
		return nil, nil, errors.New("invalid verify token length")
	}
	encToken := r[:tokLen]

	sharedSecret, err = rsa.DecryptPKCS1v15(rand.Reader, p.Private, encSecret)
	if err != nil {
		return nil, nil, errors.New("failed to decrypt shared secret")
	}
	verifyToken, err = rsa.DecryptPKCS1v15(rand.Reader, p.Private, encToken)
	if err != nil {
		return nil, nil, errors.New("failed to decrypt verify token")
	}
	return sharedSecret, verifyToken, nil
}

// Encryptor provides AES/CFB8 stream encryption/decryption, the cipher used by
// the Minecraft protocol (1.19.3+ and earlier).
type Encryptor struct {
	block cipher.Block
}

// NewEncryptor creates an Encryptor from the shared 16-byte AES secret.
func NewEncryptor(secret []byte) (*Encryptor, error) {
	if len(secret) != 16 {
		return nil, errors.New("aes shared secret must be 16 bytes")
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	return &Encryptor{block: block}, nil
}

// Reader returns r wrapped so that bytes read from it are decrypted with
// AES/CFB8.
func (e *Encryptor) Reader(r io.Reader) io.Reader {
	return &cfb8Reader{src: r, block: e.block, ivec: make([]byte, aes.BlockSize)}
}

// Writer returns w wrapped so that bytes written to it are encrypted with
// AES/CFB8.
func (e *Encryptor) Writer(w io.Writer) io.Writer {
	return &cfb8Writer{dst: w, block: e.block, ivec: make([]byte, aes.BlockSize)}
}

// cfb8Reader implements CFB-8 decryption.
type cfb8Reader struct {
	src   io.Reader
	block cipher.Block
	ivec  []byte
	ks    [aes.BlockSize]byte
	have  bool
}

func (r *cfb8Reader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	for i := 0; i < n; i++ {
		if !r.have {
			r.block.Encrypt(r.ks[:], r.ivec)
		}
		pad := p[i] ^ r.ks[0]
		// Shift IV left by one, appending the ciphertext byte.
		copy(r.ivec, r.ivec[1:])
		r.ivec[aes.BlockSize-1] = p[i]
		p[i] = pad
		r.have = false
	}
	return n, err
}

// cfb8Writer implements CFB-8 encryption.
type cfb8Writer struct {
	dst   io.Writer
	block cipher.Block
	ivec  []byte
}

func (w *cfb8Writer) Write(p []byte) (int, error) {
	for _, b := range p {
		var ks [aes.BlockSize]byte
		w.block.Encrypt(ks[:], w.ivec)
		c := b ^ ks[0]
		copy(w.ivec, w.ivec[1:])
		w.ivec[aes.BlockSize-1] = c
		if _, err := w.dst.Write([]byte{c}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// readVarInt reads a varint from a byte slice, returning the value and the
// number of bytes consumed.
func readVarInt(b []byte) (int32, int, error) {
	var value uint32
	for i := 0; i < 5; i++ {
		if i >= len(b) {
			return 0, 0, io.ErrUnexpectedEOF
		}
		value |= uint32(b[i]&0x7F) << (7 * i)
		if b[i]&0x80 == 0 {
			return int32(value), i + 1, nil
		}
	}
	return 0, 0, errors.New("varint too long")
}
