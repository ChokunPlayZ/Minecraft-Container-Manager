package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGenerateTOTPSecret(t *testing.T) {
	a, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == b {
		t.Fatal("expected distinct secrets")
	}
	if len(strings.ReplaceAll(a, "=", "")) < 32 {
		t.Fatalf("secret too short: %q", a)
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	counter := uint64(time.Now().Unix() / TOTPStepSeconds)
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
	code := fmt.Sprintf("%0*d", TOTPDigits, bin%1000000)

	if !VerifyTOTP(secret, code, 1) {
		t.Fatalf("expected code %s to verify", code)
	}
	if VerifyTOTP(secret, "000000", 0) {
		t.Fatal("expected wrong code to fail")
	}
	if VerifyTOTP(secret, "", 1) {
		t.Fatal("expected empty code to fail")
	}
	if VerifyTOTP("not-base32!", "000000", 1) {
		t.Fatal("expected invalid secret to fail")
	}
}

func TestTOTPURI(t *testing.T) {
	uri := TOTPURI("MCM", "admin@example.test", "SECRET")
	want := "otpauth://totp/MCM:admin@example.test?algorithm=SHA1&digits=6&issuer=MCM&period=30&secret=SECRET"
	if uri != want {
		t.Fatalf("unexpected uri:\n got %s\nwant %s", uri, want)
	}
}
