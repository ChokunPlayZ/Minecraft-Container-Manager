package tlsconfig

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// ListenAndServe serves HTTPS on server.Addr. When redirectHTTP is enabled,
// plaintext HTTP on that same socket is handled by a redirect-only server.
// No additional TCP listener or redirect port is created.
func ListenAndServe(server *http.Server, redirectHTTP bool) error {
	if !redirectHTTP {
		return server.ListenAndServeTLS("", "")
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return err
	}
	return serveWithRedirect(server, listener)
}

func serveWithRedirect(server *http.Server, listener net.Listener) error {
	ctx, cancel := context.WithCancel(context.Background())
	mux := &protocolMux{Listener: listener, ctx: ctx, cancel: cancel}
	defer mux.Close()
	plain := &protocolListener{protocolMux: mux, connections: make(chan net.Conn)}
	secure := &protocolListener{protocolMux: mux, connections: make(chan net.Conn)}
	redirect := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := *r.URL
			u.Scheme, u.Host, u.User = "https", r.Host, nil
			u.Opaque, u.OmitHost = "", false
			http.Redirect(w, r, u.String(), http.StatusMovedPermanently)
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	redirect.SetKeepAlivesEnabled(false)
	defer redirect.Close()
	go func() {
		if err := redirect.Serve(plain); err != nil && err != http.ErrServerClosed {
			mux.closeWithError(err)
		}
	}()
	go mux.route(plain, secure)
	// Let net/http perform its normal TLS setup, preserving Request.TLS,
	// certificate validation, and HTTP/2 negotiation.
	return server.ServeTLS(secure, "", "")
}

type protocolMux struct {
	net.Listener
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func (m *protocolMux) Close() error {
	m.closeWithError(net.ErrClosed)
	return nil
}

func (m *protocolMux) closeWithError(err error) {
	m.once.Do(func() {
		m.err = err
		m.cancel()
		m.Listener.Close()
	})
}

func (m *protocolMux) route(plain, secure *protocolListener) {
	for {
		conn, err := m.Listener.Accept()
		if err != nil {
			m.closeWithError(err)
			return
		}
		// Inspect each connection independently so a client that has not sent
		// any bytes cannot block other HTTP or HTTPS connections.
		go func() {
			stopClose := context.AfterFunc(m.ctx, func() { conn.Close() })
			defer stopClose()
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			reader := bufio.NewReader(conn)
			first, err := reader.Peek(1)
			if err != nil {
				conn.Close()
				return
			}
			conn.SetReadDeadline(time.Time{})
			target := plain
			if first[0] == 0x16 { // TLS handshake record.
				target = secure
			}
			select {
			case target.connections <- &bufferedConn{Conn: conn, reader: reader}:
			case <-m.ctx.Done():
				conn.Close()
			}
		}()
	}
}

type protocolListener struct {
	*protocolMux
	connections chan net.Conn
}

func (l *protocolListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.ctx.Done():
		return nil, l.err
	}
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
