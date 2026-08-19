package gateway

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/mcm-panel/mcm/internal/servers"
)

// statusJSON is the subset of a server-list status response we record.
type statusJSON struct {
	Version struct {
		Name string `json:"name"`
	} `json:"version"`
	Description json.RawMessage `json:"description"`
}

// CaptureMotd probes a running server for its server-list description and
// persists it as the last-known-good MOTD. It is best-effort and never blocks
// the caller for longer than the dial/read timeouts.
func (m *Manager) CaptureMotd(ctx context.Context, srv servers.Server) {
	if srv.ContainerID == "" {
		return
	}
	addr := m.backendAddr(srv)
	conn, err := net.DialTimeout("tcp", addr, m.dialTimeout)
	if err != nil {
		m.log.Printf("gateway: motd probe dial %s (%s): %v", srv.ID, addr, err)
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(m.dialTimeout * 3))

	br := bufio.NewReader(conn)
	// Handshake: protocol = 0, host = addr, port = 25565, next state = status.
	var payload []byte
	pw := newByteWriter(&payload)
	_ = writeVarInt(pw, 0)
	if err := writeMCString(pw, addr); err != nil {
		m.log.Printf("gateway: motd probe handshake for %s: %v", srv.ID, err)
		return
	}
	if err := writeUShort(pw, uint16(srv.HostPort)); err != nil {
		m.log.Printf("gateway: motd probe handshake for %s: %v", srv.ID, err)
		return
	}
	_ = writeVarInt(pw, 1)
	if err := writeFrame(conn, 0x00, payload); err != nil {
		m.log.Printf("gateway: motd probe handshake for %s: %v", srv.ID, err)
		return
	}
	// Status request (empty payload).
	if err := writeFrame(conn, 0x00, nil); err != nil {
		m.log.Printf("gateway: motd probe status request for %s: %v", srv.ID, err)
		return
	}
	// Read the status response.
	packetID, body, err := readFrame(br)
	if err != nil {
		m.log.Printf("gateway: motd probe response for %s: %v", srv.ID, err)
		return
	}
	if packetID != 0x00 {
		m.log.Printf("gateway: motd probe unexpected packet id 0x%x for %s", packetID, srv.ID)
		return
	}
	var sj statusJSON
	if err := json.Unmarshal(body, &sj); err != nil {
		m.log.Printf("gateway: motd probe parse for %s: %v", srv.ID, err)
		return
	}
	motd := motdText(sj.Description)
	if motd == "" {
		motd = sj.Version.Name
	}
	if motd == "" {
		return
	}
	if err := m.store.SetLastMotd(ctx, srv.ID, motd); err != nil {
		m.log.Printf("gateway: persist motd for %s: %v", srv.ID, err)
		return
	}
	m.log.Printf("gateway: captured last-known-good MOTD for %s: %q", srv.ID, motd)
}

// motdText extracts a plain-text description from a status description, which
// may be a JSON string or a chat component object.
func motdText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var comp struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &comp); err == nil {
		return comp.Text
	}
	return ""
}

func writeMCString(w io.Writer, s string) error {
	if err := writeVarInt(w, len(s)); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

func writeUShort(w io.Writer, v uint16) error {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	_, err := w.Write(b[:])
	return err
}
