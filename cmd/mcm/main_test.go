package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run the real entrypoint in a child process so fatal startup errors are
// caught without terminating the test runner.
func TestRunMCM(t *testing.T) {
	if os.Getenv("MCM_TEST_RUN_MAIN") == "1" {
		main()
	}
}

func TestPanelRedirectModesUseOnlyConfiguredPort(t *testing.T) {
	for _, mode := range []struct {
		name, legacyAddr, redirect, httpOnly string
	}{
		{"redirect on, old port 80 ignored", ":80", "true", "false"},
		{"redirect on, old conflicting port ignored", "same as panel", "true", "false"},
		{"redirect off", ":80", "false", "false"},
		{"HTTP only", ":80", "true", "true"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			reservation, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := reservation.Addr().String()
			reservation.Close()
			if mode.legacyAddr == "same as panel" {
				mode.legacyAddr = addr
			}
			dataDir := t.TempDir()
			logPath := filepath.Join(dataDir, "mcm.log")
			log, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer log.Close()
			cmd := exec.Command(os.Args[0], "-test.run=^TestRunMCM$")
			cmd.Env = append(os.Environ(),
				"MCM_TEST_RUN_MAIN=1", "MCM_ADDR="+addr,
				"MCM_DATA_DIR="+dataDir, "MCM_DB_PATH="+filepath.Join(dataDir, "mcm.db"),
				"MCM_CONTAINER_RUNTIME=docker", "MCM_CONTAINER_HOST=unix:///tmp/mcm-port-test-unused.sock",
				"MCM_SESSION_SECRET=temporary-test-secret", "MCM_HTTP_ONLY="+mode.httpOnly,
				"MCM_TLS_CERT=", "MCM_TLS_KEY=", "MCM_TLS_REDIRECT="+mode.redirect, "MCM_TLS_REDIRECT_ADDR="+mode.legacyAddr,
				"MCM_WEB_AUTHN_RPID=localhost", "MCM_WEB_AUTHN_RP_ORIGIN=", "MCM_PORT_RANGE=25565-25665",
			)
			cmd.Stdout, cmd.Stderr = log, log
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() { cmd.Wait(); close(done) }()
			defer func() { cmd.Process.Kill(); <-done }()
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
				select {
				case <-done:
					output, _ := os.ReadFile(logPath)
					t.Fatalf("MCM exited instead of serving on %s: %s", addr, output)
				default:
				}
				roots := x509.NewCertPool()
				scheme := "http"
				if mode.httpOnly != "true" {
					scheme = "https"
					cert, err := os.ReadFile(filepath.Join(dataDir, "tls", "cert.pem"))
					if err != nil {
						continue
					}
					if !roots.AppendCertsFromPEM(cert) {
						t.Fatal("invalid generated public certificate")
					}
				}
				client := &http.Client{
					Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}, Timeout: time.Second,
					CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
				}
				response, err := client.Get(scheme + "://" + addr + "/healthz")
				if err != nil {
					client.CloseIdleConnections()
					continue
				}
				response.Body.Close()
				client.CloseIdleConnections()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("health response = %d", response.StatusCode)
				}
				response, err = client.Get("http://" + addr + "/healthz?tab=console")
				if err != nil {
					t.Fatalf("plain HTTP check: %v", err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				client.CloseIdleConnections()
				switch {
				case mode.httpOnly == "true":
					if response.StatusCode != http.StatusOK || response.Header.Get("Location") != "" {
						t.Fatal("HTTP-only mode should serve the app without redirecting")
					}
				case mode.redirect == "true":
					if response.StatusCode != http.StatusMovedPermanently || response.Header.Get("Location") != "https://"+addr+"/healthz?tab=console" {
						t.Fatalf("same-port redirect = %d %q", response.StatusCode, response.Header.Get("Location"))
					}
				default:
					if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "HTTPS server") {
						t.Fatalf("redirect disabled: got %d %q", response.StatusCode, body)
					}
				}
				select {
				case <-done:
					output, _ := os.ReadFile(logPath)
					t.Fatalf("MCM exited after becoming healthy: %s", output)
				case <-time.After(100 * time.Millisecond):
				}
				output, _ := os.ReadFile(logPath)
				if strings.Contains(string(output), "redirecting HTTP") {
					t.Fatalf("legacy settings started a second listener: %s", output)
				}
				return
			}
			output, _ := os.ReadFile(logPath)
			t.Fatalf("MCM did not serve HTTPS on configured address %s: %s", addr, output)
		})
	}
}
