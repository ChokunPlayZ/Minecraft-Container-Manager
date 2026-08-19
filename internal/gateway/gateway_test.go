package gateway

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

type fakeStore struct {
	mu      sync.Mutex
	servers map[int]servers.Server
	states  map[string]string
	motds   map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		servers: map[int]servers.Server{},
		states:  map[string]string{},
		motds:   map[string]string{},
	}
}

func (f *fakeStore) List(context.Context) ([]servers.Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]servers.Server, 0, len(f.servers))
	for _, s := range f.servers {
		s.State = f.states[s.ID]
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeStore) GetByPort(_ context.Context, port int) (servers.Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.servers[port]
	if !ok {
		return servers.Server{}, servers.ErrNotFound
	}
	s.State = f.states[s.ID]
	return s, nil
}

func (f *fakeStore) Status(_ context.Context, id string) (servers.Server, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.servers {
		if s.ID == id {
			s.State = f.states[id]
			return s, nil
		}
	}
	return servers.Server{}, servers.ErrNotFound
}

func (f *fakeStore) GatewayInfo(_ context.Context, id string, enabled bool) (servers.GatewayInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return servers.GatewayInfo{Enabled: enabled, WakeMessage: "", LastMotd: f.motds[id]}, nil
}

func (f *fakeStore) SetLastMotd(_ context.Context, id, motd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.motds[id] = motd
	return nil
}

func (f *fakeStore) add(srv servers.Server) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.servers[srv.HostPort] = srv
	f.states[srv.ID] = srv.State
}

type fakeDocker struct {
	addr string
	host string
}

func (d *fakeDocker) ContainerAddr(context.Context, string, int) (string, error) {
	return d.addr, nil
}

func (d *fakeDocker) HostAddress() string { return d.host }

type fakeWaker struct {
	mu    sync.Mutex
	wakes []string
}

func (w *fakeWaker) Wake(_ context.Context, id string) (servers.Server, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wakes = append(w.wakes, id)
	return servers.Server{ID: id, State: servers.StateRunning}, nil
}

func (w *fakeWaker) wakeCount(id string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, x := range w.wakes {
		if x == id {
			n++
		}
	}
	return n
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func newTestManager(t *testing.T, store *fakeStore, docker *fakeDocker) (*Manager, *fakeWaker) {
	t.Helper()
	waker := &fakeWaker{}
	m := New(Options{
		Logger:            log.New(io.Discard, "", 0),
		Store:             store,
		Docker:            docker,
		Waker:             waker,
		Enabled:           func(context.Context) (bool, error) { return true, nil },
		ReconcileInterval: time.Hour, // start then reconcile manually
		DialInterval:      10 * time.Millisecond,
		HoldTimeout:       5 * time.Second,
		DialTimeout:       time.Second,
		IdleDeadline:      time.Second,
	})
	m.Start()
	t.Cleanup(m.Stop)
	m.reconcile()
	return m, waker
}

func TestWakeOnConnectAndReplay(t *testing.T) {
	port := freePort(t)
	// Stub backend that reads the replayed first frame.
	backend, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer backend.Close()
	backendPort := backend.Addr().(*net.TCPAddr).Port

	backendRecv := make(chan []byte, 1)
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		_, body, err := readFrame(br)
		if err == nil {
			backendRecv <- body
		}
		io.Copy(io.Discard, c)
	}()

	store := newFakeStore()
	store.add(servers.Server{ID: "srv1", Name: "Srv", Version: "1.21.1", HostPort: port, ContainerID: "abc", State: servers.StateStopped})
	docker := &fakeDocker{addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(backendPort)), host: "127.0.0.1"}
	m, waker := newTestManager(t, store, docker)
	_ = m

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()

	// Send a login handshake (next state 2).
	var payload []byte
	pw := newByteWriter(&payload)
	_ = writeVarInt(pw, 763)
	_ = writeMCString(pw, "localhost")
	_ = writeUShort(pw, 25565)
	_ = writeVarInt(pw, 2)
	if err := writeFrame(conn, 0x00, payload); err != nil {
		t.Fatalf("send handshake: %v", err)
	}

	select {
	case body := <-backendRecv:
		if len(body) == 0 {
			t.Fatal("backend received empty frame")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("backend did not receive the replayed handshake")
	}
	if got := waker.wakeCount("srv1"); got != 1 {
		t.Errorf("wake count = %d, want 1", got)
	}
}

func TestStatusResponderServesMotdWithoutWake(t *testing.T) {
	port := freePort(t)
	store := newFakeStore()
	store.add(servers.Server{ID: "srv1", Name: "Srv", Version: "1.21.1", HostPort: port, State: servers.StateStopped})
	store.motds["srv1"] = "My Cool Server"
	docker := &fakeDocker{addr: "127.0.0.1:1", host: "127.0.0.1"}
	_, waker := newTestManager(t, store, docker)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	defer conn.Close()
	br := bufio.NewReader(conn)

	// Handshake with next state 1 (status).
	var hp []byte
	pw := newByteWriter(&hp)
	_ = writeVarInt(pw, 763)
	_ = writeMCString(pw, "localhost")
	_ = writeUShort(pw, 25565)
	_ = writeVarInt(pw, 1)
	if err := writeFrame(conn, 0x00, hp); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	// Status request.
	if err := writeFrame(conn, 0x00, nil); err != nil {
		t.Fatalf("status request: %v", err)
	}
	// Ping.
	if err := writeFrame(conn, 0x01, nil); err != nil {
		t.Fatalf("ping: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	packetID, body, err := readFrame(br)
	if err != nil {
		t.Fatalf("read status response: %v", err)
	}
	if packetID != 0x00 {
		t.Fatalf("status packet id = 0x%x, want 0", packetID)
	}
	if !containsJSON(body, "My Cool Server") {
		t.Errorf("status body %q missing MOTD", body)
	}
	if got := waker.wakeCount("srv1"); got != 0 {
		t.Errorf("status ping should not wake server, wake count = %d", got)
	}
}

func TestReconcileClosesListenerForDeletedServer(t *testing.T) {
	store := newFakeStore()
	m := New(Options{
		Logger:  log.New(io.Discard, "", 0),
		Store:   store,
		Docker:  &fakeDocker{host: "127.0.0.1"},
		Waker:   &fakeWaker{},
		Enabled: func(context.Context) (bool, error) { return true, nil },
	})
	m.Start()
	defer m.Stop()

	port := freePort(t)
	store.add(servers.Server{ID: "srv1", HostPort: port, State: servers.StateStopped})
	m.reconcile()
	if n := m.listenerCount(); n != 1 {
		t.Fatalf("listener count after add = %d, want 1", n)
	}
	// Simulate delete.
	store.mu.Lock()
	delete(store.servers, port)
	delete(store.states, "srv1")
	store.mu.Unlock()
	m.reconcile()
	if n := m.listenerCount(); n != 0 {
		t.Errorf("listener count after delete = %d, want 0", n)
	}
}

func (m *Manager) listenerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.listeners)
}

func containsJSON(body []byte, sub string) bool {
	for i := 0; i+len(sub) <= len(body); i++ {
		if string(body[i:i+len(sub)]) == sub {
			return true
		}
	}
	return false
}
