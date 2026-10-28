package tlsconfig

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/config"
)

func TestSelfSignedHTTPSAndReuse(t *testing.T) {
	cfg := &config.Config{
		DataDir: t.TempDir(), Addr: "192.0.2.10:8080",
		WebAuthn: config.WebAuthnConfig{RPID: "panel.example.com", RPOrigins: []string{"https://mcm.example.com:8080"}},
	}
	tlsCfg, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cert := tlsCfg.Certificates[0]
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "192.0.2.10", "panel.example.com", "mcm.example.com"} {
		if err := leaf.VerifyHostname(host); err != nil {
			t.Errorf("missing SAN for %s: %v", host, err)
		}
	}
	if err := leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		t.Fatalf("certificate is not self-signed: %v", err)
	}
	if time.Until(leaf.NotAfter) < 364*24*time.Hour {
		t.Fatal("certificate should be valid for a year")
	}
	keyPath := filepath.Join(cfg.DataDir, "tls", "key.pem")
	info, err := os.Stat(keyPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private key must be owner-readable only: %v, %v", info, err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "MCM HTTPS")
	}))
	server.TLS = tlsCfg
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}, Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("trusted HTTPS handshake failed: %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "MCM HTTPS" {
		t.Fatalf("unexpected HTTPS response: %q", body)
	}
	reloaded, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cert.Certificate[0], reloaded.Certificates[0].Certificate[0]) {
		t.Fatal("certificate changed on restart")
	}
}

func TestSuppliedCertificateAndFailures(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := ensureSelfSigned(certPath, keyPath, []string{"custom.example.com"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(certPath)
	cfg := &config.Config{DataDir: filepath.Join(dir, "unused"), TLSCert: certPath, TLSKey: keyPath}
	loaded, err := Load(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Certificates[0].Leaf.VerifyHostname("custom.example.com"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(certPath)
	if !bytes.Equal(before, after) {
		t.Fatal("supplied certificate was changed")
	}
	if _, err := os.Stat(cfg.DataDir); !os.IsNotExist(err) {
		t.Fatal("supplied certificates should not create fallback files")
	}
	for _, paths := range [][2]string{{certPath, ""}, {"", keyPath}, {"/nonexistent/cert.pem", keyPath}} {
		cfg.TLSCert, cfg.TLSKey = paths[0], paths[1]
		if _, err := Load(cfg); err == nil {
			t.Fatalf("expected invalid supplied pair %v to fail", paths)
		}
	}
}

func TestSelfSignedRenewal(t *testing.T) {
	for _, reason := range []string{"expiry", "hostname", "missing key"} {
		t.Run(reason, func(t *testing.T) {
			cfg := &config.Config{DataDir: t.TempDir(), Addr: ":8080"}
			first, err := Load(cfg)
			if err != nil {
				t.Fatal(err)
			}
			cert := first.Certificates[0]
			certPath, keyPath := filepath.Join(cfg.DataDir, "tls", "cert.pem"), filepath.Join(cfg.DataDir, "tls", "key.pem")
			switch reason {
			case "expiry":
				leaf, err := x509.ParseCertificate(cert.Certificate[0])
				if err != nil {
					t.Fatal(err)
				}
				leaf.NotAfter = time.Now().Add(24 * time.Hour)
				key := cert.PrivateKey.(*ecdsa.PrivateKey)
				der, err := x509.CreateCertificate(rand.Reader, leaf, leaf, &key.PublicKey, key)
				if err != nil {
					t.Fatal(err)
				}
				if err := writePEM(certPath, &pem.Block{Type: "CERTIFICATE", Bytes: der}, 0644); err != nil {
					t.Fatal(err)
				}
			case "hostname":
				cfg.WebAuthn.RPID = "new.example.com"
			case "missing key":
				if err := os.Remove(keyPath); err != nil {
					t.Fatal(err)
				}
			}
			renewed, err := Load(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(cert.Certificate[0], renewed.Certificates[0].Certificate[0]) {
				t.Fatal("certificate was not renewed")
			}
		})
	}
}

func TestInvalidStoredPairAndUnwritableDirectoryFail(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir()}
	if _, err := Load(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, "tls", "key.pem"), []byte("broken key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfg); err == nil {
		t.Fatal("corrupt stored key should fail explicitly")
	}
	cfg.DataDir = filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(cfg.DataDir, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(cfg); err == nil {
		t.Fatal("unwritable certificate directory should fail explicitly")
	}
}
