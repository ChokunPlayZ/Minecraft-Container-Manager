package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/config"
)

func TestSamePortHTTPRedirectAndHTTPS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	tlsCfg, err := Load(&config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var appRequests atomic.Int32
	server := &http.Server{TLSConfig: tlsCfg, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appRequests.Add(1)
		if r.TLS == nil {
			http.Error(w, "app received plaintext request", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "secure app")
	})}
	done := make(chan error, 1)
	go func() { done <- serveWithRedirect(server, listener) }()
	t.Cleanup(func() {
		server.Close()
		select {
		case err := <-done:
			if err != http.ErrServerClosed {
				t.Errorf("server close: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("shared listener did not close")
		}
	})
	addr := listener.Addr().String()
	plain := &http.Client{
		Transport:     &http.Transport{},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       time.Second,
	}
	defer plain.CloseIdleConnections()
	for _, tc := range []struct{ method, host, path string }{
		{http.MethodGet, "mc.ckl.moe:8080", "/"},
		{http.MethodGet, "mc.ckl.moe:8080", "/servers/a%2Fb?tab=console&search=a%2Bb"},
		{http.MethodHead, "[::1]:8080", "/login?next=%2Fdashboard"},
		{http.MethodPost, addr, "/api/onboarding"},
	} {
		req, err := http.NewRequest(tc.method, "http://"+addr+tc.path, strings.NewReader("test"))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = tc.host
		response, err := plain.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		want := "https://" + tc.host + tc.path
		if response.StatusCode != http.StatusMovedPermanently || response.Header.Get("Location") != want {
			t.Errorf("%s %s: got %d %q, want 301 %q", tc.method, tc.path, response.StatusCode, response.Header.Get("Location"), want)
		}
		if response.Header.Get("Set-Cookie") != "" {
			t.Error("plaintext redirect should not issue app cookies")
		}
	}
	if got := appRequests.Load(); got != 0 {
		t.Fatalf("plaintext requests reached the app: %d", got)
	}

	// A client that sends no bytes must not block either protocol.
	slow, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	roots := x509.NewCertPool()
	roots.AddCert(tlsCfg.Certificates[0].Leaf)
	secure := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}, ForceAttemptHTTP2: true},
		Timeout:   time.Second,
	}
	defer secure.CloseIdleConnections()
	response, err := secure.Get("https://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("HTTPS with an idle plaintext connection: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != "secure app" || response.ProtoMajor != 2 {
		t.Fatalf("HTTPS response = %q, protocol %s, error %v", body, response.Proto, err)
	}
	response, err = secure.Get("http://" + addr + "/healthz?redirect=follow")
	if err != nil {
		t.Fatalf("following same-port redirect: %v", err)
	}
	body, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != "secure app" || response.Request.URL.Scheme != "https" || appRequests.Load() != 2 {
		t.Fatalf("redirect did not reach HTTPS app exactly once: response %q, requests %d, error %v", body, appRequests.Load(), err)
	}

	server.Close()
	slow.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := slow.Read(make([]byte, 1)); err == nil {
		t.Fatal("unclassified connection survived server shutdown")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("server shutdown left an unclassified connection open")
	}
}
