// Package gateway owns each server's public game port. It binds the port that
// players connect to, wakes a sleeping server on the first inbound connect,
// holds the connection in limbo until the backend accepts, relays to it, and
// serves the last-known-good MOTD on the server-list ping while sleeping.
//
// Phase 1 relays raw bytes after a handshake-aware hold; it does not terminate
// the Minecraft protocol. The durable limbo void and warp-in are Phase 2.
package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy"
	"github.com/mcm-panel/mcm/internal/proxy/protocol"
	"github.com/mcm-panel/mcm/internal/servers"
)

// defaultWaitMessage is the fallback wait/void message shown to a player while
// a server is waking up, used when neither a per-server nor global default is
// configured.
const defaultWaitMessage = "Server is starting up, please wait..."

// Store is the subset of *servers.Store the gateway needs.
type Store interface {
	List(ctx context.Context) ([]servers.Server, error)
	GetByPort(ctx context.Context, port int) (servers.Server, error)
	Status(ctx context.Context, id string) (servers.Server, error)
	GatewayInfo(ctx context.Context, id string, enabled bool) (servers.GatewayInfo, error)
	SetLastMotd(ctx context.Context, id, motd string) error
	WakeMessage(ctx context.Context, id string) (string, error)
	WakeMessageDefault(ctx context.Context, def string) (string, error)
}

// Docker is the subset of *docker.Manager the gateway needs.
type Docker interface {
	ContainerAddr(ctx context.Context, containerID string, hostPort int) (string, error)
	HostAddress() string
}

// Waker starts a stopped server. *spindown.Service satisfies it.
type Waker interface {
	Wake(ctx context.Context, id string) (servers.Server, error)
}

// EnabledFunc reports whether the gateway should currently be active. It is
// evaluated each reconcile so enabling/disabling spin-down takes effect live.
type EnabledFunc func(ctx context.Context) (bool, error)

// Options configures a Manager.
type Options struct {
	Logger  *log.Logger
	Store   Store
	Docker  Docker
	Waker   Waker
	Enabled EnabledFunc
	// OnlineMode enables Mojang session-server authentication for login
	// connections. When false, offline-mode auth is used. Callers that need
	// online mode should set this; most MCM deployments default to offline.
	OnlineMode          bool
	ReconcileInterval   time.Duration
	DialInterval        time.Duration
	HoldTimeout         time.Duration
	DialTimeout         time.Duration
	IdleDeadline        time.Duration
	RelayTotalTimeout   time.Duration
	MotdCaptureInterval time.Duration
}

type listenerHandle struct {
	listener  net.Listener
	port      int
	closeOnce sync.Once
}

// Manager owns the per-server gateway listeners and their accept loops.
type Manager struct {
	log     *log.Logger
	store   Store
	docker  Docker
	waker   Waker
	enabled EnabledFunc
	online  bool

	reconcileInterval   time.Duration
	dialInterval        time.Duration
	holdTimeout         time.Duration
	dialTimeout         time.Duration
	idleDeadline        time.Duration
	relayTotalTimeout   time.Duration
	motdCaptureInterval time.Duration

	mu        sync.Mutex
	listeners map[int]*listenerHandle
	byPort    map[int]servers.Server
	running   bool
	lastMotd  map[string]time.Time

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// New builds a Manager from Options, applying sane defaults.
func New(opts Options) *Manager {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.ReconcileInterval <= 0 {
		opts.ReconcileInterval = 30 * time.Second
	}
	if opts.DialInterval <= 0 {
		opts.DialInterval = 2 * time.Second
	}
	if opts.HoldTimeout <= 0 {
		opts.HoldTimeout = 90 * time.Second
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 5 * time.Second
	}
	if opts.IdleDeadline <= 0 {
		opts.IdleDeadline = 2 * time.Minute
	}
	if opts.RelayTotalTimeout <= 0 {
		opts.RelayTotalTimeout = 12 * time.Hour
	}
	if opts.MotdCaptureInterval <= 0 {
		opts.MotdCaptureInterval = time.Minute
	}
	return &Manager{
		log:                 opts.Logger,
		store:               opts.Store,
		docker:              opts.Docker,
		waker:               opts.Waker,
		enabled:             opts.Enabled,
		online:              opts.OnlineMode,
		reconcileInterval:   opts.ReconcileInterval,
		dialInterval:        opts.DialInterval,
		holdTimeout:         opts.HoldTimeout,
		dialTimeout:         opts.DialTimeout,
		idleDeadline:        opts.IdleDeadline,
		relayTotalTimeout:   opts.RelayTotalTimeout,
		motdCaptureInterval: opts.MotdCaptureInterval,
		listeners:           map[int]*listenerHandle{},
		byPort:              map[int]servers.Server{},
		lastMotd:            map[string]time.Time{},
	}
}

// Start launches the reconcile loop and performs an initial reconcile.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.done = make(chan struct{})
	m.mu.Unlock()

	go m.loop()
}

// Stop tears down all listeners and in-flight connections cleanly.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	cancel := m.cancel
	m.mu.Unlock()

	cancel()
	<-m.done
	m.mu.Lock()
	for _, h := range m.listeners {
		_ = h.listener.Close()
	}
	m.listeners = map[int]*listenerHandle{}
	m.mu.Unlock()
}

func (m *Manager) loop() {
	defer close(m.done)
	m.reconcile()
	ticker := time.NewTicker(m.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.reconcile()
		}
	}
}

// reconcile aligns the set of listeners with the current server list, opening
// listeners for every server (including sleeping ones) and removing those for
// deleted servers. When the gateway is disabled it closes all listeners.
func (m *Manager) reconcile() {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	active := true
	if m.enabled != nil {
		var err error
		active, err = m.enabled(ctx)
		if err != nil {
			m.log.Printf("gateway: enable check: %v", err)
			active = false
		}
	}
	if !active {
		m.closeAllListeners()
		return
	}

	list, err := m.store.List(ctx)
	if err != nil {
		m.log.Printf("gateway: list servers: %v", err)
		return
	}
	wanted := map[int]servers.Server{}
	for _, srv := range list {
		if srv.HostPort > 0 {
			wanted[srv.HostPort] = srv
		}
	}

	// Capture last-known-good MOTD for running servers, throttled per server.
	m.mu.Lock()
	now := time.Now()
	for _, srv := range list {
		if srv.State != servers.StateRunning {
			continue
		}
		if last, ok := m.lastMotd[srv.ID]; ok && now.Sub(last) < m.motdCaptureInterval {
			continue
		}
		m.lastMotd[srv.ID] = now
		server := srv
		go m.CaptureMotd(ctx, server)
	}
	m.mu.Unlock()

	// Open listeners for servers that do not yet have one.
	m.mu.Lock()
	for port, srv := range wanted {
		if _, ok := m.listeners[port]; ok {
			continue
		}
		h, err := m.openListener(ctx, port)
		if err != nil {
			m.log.Printf("gateway: bind port %d for %s: %v", port, srv.Name, err)
			continue
		}
		m.listeners[port] = h
		go m.acceptLoop(h)
	}
	// Refresh the by-port snapshot for accept-time resolution.
	m.byPort = wanted
	m.mu.Unlock()

	// Remove listeners for servers that are no longer present.
	m.mu.Lock()
	for port, h := range m.listeners {
		if _, ok := wanted[port]; !ok {
			_ = h.listener.Close()
			delete(m.listeners, port)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) closeAllListeners() {
	m.mu.Lock()
	for port, h := range m.listeners {
		_ = h.listener.Close()
		delete(m.listeners, port)
	}
	m.byPort = map[int]servers.Server{}
	m.mu.Unlock()
}

// openListener binds a single TCP listener on the given port.
func (m *Manager) openListener(ctx context.Context, port int) (*listenerHandle, error) {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	return &listenerHandle{listener: ln, port: port}, nil
}

func (m *Manager) acceptLoop(h *listenerHandle) {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			default:
				// Transient accept error (e.g. EMFILE); back off briefly.
				if errors.Is(err, net.ErrClosed) {
					return
				}
				m.log.Printf("gateway: accept on %d: %v", h.port, err)
				time.Sleep(time.Second)
				continue
			}
		}
		go m.handleConn(conn, h.port)
	}
}

// handleConn handles one inbound client connection on a gateway port.
func (m *Manager) handleConn(client net.Conn, port int) {
	defer client.Close()
	ctx, cancel := context.WithTimeout(m.ctx, m.holdTimeout+m.idleDeadline)
	defer cancel()

	srv, err := m.store.GetByPort(ctx, port)
	if err != nil {
		m.log.Printf("gateway: resolve port %d: %v", port, err)
		return
	}
	m.handleClient(ctx, client, srv)
}

func (m *Manager) handleClient(ctx context.Context, client net.Conn, srv servers.Server) {
	_ = client.SetReadDeadline(time.Now().Add(m.dialTimeout))
	br := bufio.NewReader(client)
	packetID, payload, err := readFrame(br)
	if err != nil {
		// Not a valid frame; nothing to relay.
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	if packetID == 0x00 {
		hs, herr := parseHandshake(payload)
		if herr == nil && hs.nextState == 1 {
			// Server-list status request: respond with last-known-good MOTD
			// without waking the server.
			m.respondStatus(ctx, client, br, srv)
			return
		}
		if herr == nil && hs.nextState == 2 {
			// Login handshake: run the protocol proxy (Phase 2), which wakes
			// the server, holds the player in the End void, and warps them in.
			m.handleLogin(ctx, client, br, srv, hs)
			return
		}
	}

	// Rebuild the full first frame (length + packet ID + payload) so we can
	// replay it to the backend once it accepts.
	first := encodeFrame(packetID, payload)

	// Wake-on-connect.
	current, serr := m.store.Status(ctx, srv.ID)
	if serr == nil && m.isSleeping(current) {
		m.log.Printf("gateway: waking sleeping server %s on connect", srv.ID)
		if _, werr := m.waker.Wake(ctx, srv.ID); werr != nil {
			m.log.Printf("gateway: wake %s: %v", srv.ID, werr)
		}
	}
	m.log.Printf("gateway: holding connection for %s until backend accepts", srv.ID)

	backend, derr := m.dialBackend(ctx, srv, first)
	if derr != nil {
		m.log.Printf("gateway: backend for %s unavailable after hold: %v", srv.ID, derr)
		return
	}
	defer backend.Close()
	m.log.Printf("gateway: backend for %s accepting; relaying", srv.ID)
	m.relay(backend, client, first)
}

// handleLogin runs the Phase 2 protocol proxy for a login handshake. It wakes
// a sleeping server, resolves the per-server wait message, and hands the
// connection to the proxy, which terminates the login, holds the player in the
// End void, and warps them into the backend once it accepts.
func (m *Manager) handleLogin(ctx context.Context, client net.Conn, br *bufio.Reader, srv servers.Server, hs *handshake) {
	ver, err := protocol.Lookup(int32(hs.protocol))
	if err != nil {
		// Unsupported client protocol: send a clear disconnect rather than
		// holding the player or crashing the accept loop.
		m.log.Printf("gateway: unsupported protocol %d from %s", hs.protocol, srv.ID)
		m.sendLoginDisconnect(client, protocol.DisconnectReason(int32(hs.protocol)))
		return
	}

	current, serr := m.store.Status(ctx, srv.ID)
	if serr == nil && m.isSleeping(current) {
		m.log.Printf("gateway: waking sleeping server %s on login", srv.ID)
		if _, werr := m.waker.Wake(ctx, srv.ID); werr != nil {
			m.log.Printf("gateway: wake %s: %v", srv.ID, werr)
		}
	}

	msg := m.wakeMessage(ctx, srv.ID)
	wrapped := &bufferedConn{Conn: client, r: br}
	perr := proxy.HandleClient(ctx, wrapped, proxy.SessionOptions{
		Logger:       m.log,
		Protocol:     ver,
		OnlineMode:   m.online,
		Message:      msg,
		BackendConn:  func(ctx context.Context) (net.Conn, error) { return m.dialBackendConn(ctx, srv) },
		HoldTimeout:  m.holdTimeout,
		IdleDeadline: m.idleDeadline,
	})
	if perr != nil && !errors.Is(perr, context.Canceled) && !errors.Is(perr, context.DeadlineExceeded) {
		m.log.Printf("gateway: proxy session for %s closed: %v", srv.ID, perr)
	}
}

// sendLoginDisconnect writes a login-state disconnect with the given reason to
// the client so it shows a readable message instead of timing out.
func (m *Manager) sendLoginDisconnect(client net.Conn, reason string) {
	raw, err := json.Marshal(map[string]string{"text": reason})
	if err != nil {
		return
	}
	var payload []byte
	pw := newByteWriter(&payload)
	_ = writeVarInt(pw, len(raw))
	payload = append(payload, raw...)
	if err := writeFrame(client, 0x00, payload); err != nil {
		m.log.Printf("gateway: login disconnect to %s: %v", client.RemoteAddr(), err)
	}
}

// wakeMessage resolves the effective wait message for a server: the per-server
// override, falling back to the global default.
func (m *Manager) wakeMessage(ctx context.Context, id string) string {
	if msg, err := m.store.WakeMessage(ctx, id); err == nil && msg != "" {
		return msg
	}
	if def, err := m.store.WakeMessageDefault(ctx, defaultWaitMessage); err == nil && def != "" {
		return def
	}
	return defaultWaitMessage
}

// dialBackendConn polls the backend address until a dial succeeds (bounded by
// holdTimeout) and returns the raw connection without writing anything. The
// proxy performs its own backend handshake.
func (m *Manager) dialBackendConn(ctx context.Context, srv servers.Server) (net.Conn, error) {
	deadline := time.Now().Add(m.holdTimeout)
	var lastErr error
	for {
		current, serr := m.store.Status(ctx, srv.ID)
		if serr == nil {
			srv = current
		}
		addr := m.backendAddr(srv)
		conn, err := net.DialTimeout("tcp", addr, m.dialTimeout)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.dialInterval):
		}
	}
}

// bufferedConn adapts a net.Conn that may already have a bufio.Reader holding
// buffered bytes (the already-consumed handshake remnant) so the proxy can read
// the Login Start packet without losing data. Reads go through the buffered
// reader; all other operations delegate to the underlying connection.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (m *Manager) isSleeping(srv servers.Server) bool {
	switch srv.State {
	case servers.StateStopped, servers.StateStopping, servers.StateError:
		return true
	default:
		return false
	}
}

// respondStatus answers a server-list ping with the last-known-good MOTD.
func (m *Manager) respondStatus(ctx context.Context, client net.Conn, br *bufio.Reader, srv servers.Server) {
	info, err := m.store.GatewayInfo(ctx, srv.ID, true)
	if err != nil {
		return
	}
	motd := info.LastMotd
	if motd == "" {
		motd = srv.Name
	}
	status, err := buildStatusJSON(motd, srv.Version)
	if err != nil {
		return
	}
	if err := writeFrame(client, 0x00, status); err != nil {
		return
	}
	// Read the following ping and echo it back so the client treats the server
	// as reachable and online.
	_ = client.SetReadDeadline(time.Now().Add(m.idleDeadline))
	if _, _, err := readFrame(br); err == nil {
		// Echo the ping payload so latency/favicon checks complete.
		_ = writeFrame(client, 0x01, nil)
	}
}

// dialBackend polls the server's backend address until a dial succeeds (up to
// holdTimeout), then relays the buffered first frame.
func (m *Manager) dialBackend(ctx context.Context, srv servers.Server, first []byte) (net.Conn, error) {
	deadline := time.Now().Add(m.holdTimeout)
	var lastErr error
	for {
		current, serr := m.store.Status(ctx, srv.ID)
		if serr == nil {
			srv = current
		}
		addr := m.backendAddr(srv)
		conn, err := net.DialTimeout("tcp", addr, m.dialTimeout)
		if err == nil {
			_ = conn.SetWriteDeadline(time.Now().Add(m.dialTimeout))
			if _, werr := conn.Write(first); werr != nil {
				conn.Close()
				lastErr = werr
			} else {
				_ = conn.SetWriteDeadline(time.Time{})
				return conn, nil
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.dialInterval):
		}
	}
}

// backendAddr resolves the address to dial for a server's game port.
func (m *Manager) backendAddr(srv servers.Server) string {
	ctx := m.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if srv.ContainerID != "" {
		addr, aerr := m.docker.ContainerAddr(ctx, srv.ContainerID, srv.HostPort)
		if aerr == nil {
			return addr
		}
		m.log.Printf("gateway: inspect %s: %v", srv.ID, aerr)
	}
	return net.JoinHostPort(m.docker.HostAddress(), strconv.Itoa(srv.HostPort))
}

// relay copies bytes bidirectionally between the backend and client, with
// half-close handling and idle/total deadlines so zombie connections do not
// leak.
func (m *Manager) relay(backend, client net.Conn, initial []byte) {
	if len(initial) > 0 {
		_, _ = backend.Write(initial)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	stop := make(chan struct{})
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			close(stop)
			_ = backend.Close()
			_ = client.Close()
		})
	}
	go func() {
		defer wg.Done()
		m.copyWithDeadline(client, backend)
		closeWrite(backend)
	}()
	go func() {
		defer wg.Done()
		m.copyWithDeadline(backend, client)
		closeWrite(client)
	}()
	time.AfterFunc(m.relayTotalTimeout, closeBoth)
	wg.Wait()
	closeBoth()
}

func (m *Manager) copyWithDeadline(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		_ = src.SetReadDeadline(time.Now().Add(m.idleDeadline))
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func closeWrite(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
