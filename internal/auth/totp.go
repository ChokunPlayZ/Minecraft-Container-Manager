package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// TOTPDigits is the number of digits in a TOTP code.
	TOTPDigits = 6
	// TOTPStepSeconds is the time step for TOTP codes.
	TOTPStepSeconds = 30
)

// GenerateTOTPSecret returns a fresh random base32-encoded TOTP secret.
// RFC 6238 requires at least 128 bits of entropy (20 bytes).
func GenerateTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}
	// Strip any trailing padding for cleaner output.
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "="), nil
}

// TOTPURI builds an otpauth://totp URI for QR-code enrollment.
func TOTPURI(issuer, account, secret string) string {
	label := issuer + ":" + account
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", fmt.Sprintf("%d", TOTPDigits))
	v.Set("period", fmt.Sprintf("%d", TOTPStepSeconds))
	return "otpauth://totp/" + url.PathEscape(label) + "?" + v.Encode()
}

// VerifyTOTP checks a 6-digit code against the given secret for the current
// time step and up to window steps in either direction to tolerate clock skew.
func VerifyTOTP(secret, code string, window int) bool {
	if code == "" {
		return false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}
	counter := uint64(time.Now().Unix() / TOTPStepSeconds)
	for i := -window; i <= window; i++ {
		if totpCode(key, counter+uint64(i)) == code {
			return true
		}
	}
	return false
}

func totpCode(key []byte, counter uint64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := bin % uint32(math.Pow10(TOTPDigits))
	return fmt.Sprintf("%0*d", TOTPDigits, mod)
}

// NewUserHandle returns a random byte slice suitable for use as a WebAuthn
// user handle. It is scoped here so auth callers share one implementation.
func NewUserHandle() []byte {
	return []byte(uuid.NewString())
}
