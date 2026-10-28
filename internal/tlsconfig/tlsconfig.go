// Package tlsconfig loads supplied certificates or persists a self-signed
// certificate for installations without their own TLS certificate.
package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mcm-panel/mcm/internal/config"
)

// Load uses operator-provided certificates unchanged. Otherwise it reuses a
// generated pair in DataDir/tls, renewing it when fewer than 30 days remain or
// the installation's hostnames change. Invalid supplied or stored pairs fail
// explicitly instead of falling back to HTTP.
func Load(cfg *config.Config) (*tls.Config, error) {
	certPath, keyPath := cfg.TLSCert, cfg.TLSKey
	if (certPath == "") != (keyPath == "") {
		return nil, fmt.Errorf("both TLS certificate and key paths are required")
	}
	if certPath == "" {
		dir := filepath.Join(cfg.DataDir, "tls")
		certPath, keyPath = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
		if err := ensureSelfSigned(certPath, keyPath, certificateHosts(cfg)); err != nil {
			return nil, fmt.Errorf("prepare self-signed TLS certificate: %w", err)
		}
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}, nil
}

func certificateHosts(cfg *config.Config) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if host, err := os.Hostname(); err == nil && host != "" {
		hosts = append(hosts, host)
	}
	if host, _, err := net.SplitHostPort(cfg.Addr); err == nil && host != "" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
			hosts = append(hosts, host)
		}
	}
	if cfg.WebAuthn.RPID != "" {
		hosts = append(hosts, cfg.WebAuthn.RPID)
	}
	for _, origin := range cfg.WebAuthn.RPOrigins {
		if u, err := url.Parse(origin); err == nil && u.Hostname() != "" {
			hosts = append(hosts, u.Hostname())
		}
	}
	// Include LAN addresses so direct connections to the host can validate
	// after the administrator trusts the generated public certificate.
	if addresses, err := net.InterfaceAddrs(); err == nil {
		for _, address := range addresses {
			if ip, _, err := net.ParseCIDR(address.String()); err == nil && ip.IsGlobalUnicast() {
				hosts = append(hosts, ip.String())
			}
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

func ensureSelfSigned(certPath, keyPath string, hosts []string) error {
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if certErr != nil && !os.IsNotExist(certErr) {
		return certErr
	}
	if keyErr != nil && !os.IsNotExist(keyErr) {
		return keyErr
	}
	if certErr == nil && keyErr == nil {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("load stored certificate: %w", err)
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
		renew := time.Now().Before(leaf.NotBefore) || time.Until(leaf.NotAfter) < 30*24*time.Hour
		for _, host := range hosts {
			if leaf.VerifyHostname(host) != nil {
				renew = true
			}
		}
		if !renew {
			return nil
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "MCM", Organization: []string{"Minecraft Container Manager"}},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return err
	}
	if err := writePEM(keyPath, &pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}, 0600); err != nil {
		return err
	}
	return writePEM(certPath, &pem.Block{Type: "CERTIFICATE", Bytes: der}, 0644)
}

// Publish complete PEM files, keeping temporary private keys owner-readable.
func writePEM(path string, block *pem.Block, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tls-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := pem.Encode(f, block); err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
