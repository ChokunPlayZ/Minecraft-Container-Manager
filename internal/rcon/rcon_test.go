package rcon

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

func encodePacket(requestID, typ int32, payload string) []byte {
	body := make([]byte, 0, len(payload)+10)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(requestID))
	body = append(body, buf...)
	binary.LittleEndian.PutUint32(buf, uint32(typ))
	body = append(body, buf...)
	body = append(body, []byte(payload)...)
	body = append(body, 0, 0)
	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(body)))
	return append(header, body...)
}

func decodePacket(t *testing.T, b []byte) (requestID, typ int32, payload string) {
	t.Helper()
	if len(b) < 10 {
		t.Fatalf("packet too short: %d", len(b))
	}
	length := int(binary.LittleEndian.Uint32(b[0:4]))
	if length != len(b)-4 {
		t.Fatalf("length mismatch: header %d vs actual %d", length, len(b)-4)
	}
	requestID = int32(binary.LittleEndian.Uint32(b[4:8]))
	typ = int32(binary.LittleEndian.Uint32(b[8:12]))
	if length > 10 {
		payload = string(b[12 : 4+length-2])
	}
	return
}

func TestPacketRoundTrip(t *testing.T) {
	requestID, typ := int32(7), int32(TypeExecCommand)
	payload := "list"
	wire := encodePacket(requestID, typ, payload)
	rid, rt, rp := decodePacket(t, wire)
	if rid != requestID || rt != typ || rp != payload {
		t.Fatalf("round trip mismatch: got (%d, %d, %q)", rid, rt, rp)
	}
}

// fakeServer implements just enough of the RCON protocol to exercise the client.
type fakeServer struct {
	ln        net.Listener
	password  string
	responses map[string]string
	t         *testing.T
}

func newFakeServer(t *testing.T, password string, responses map[string]string) *fakeServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	fs := &fakeServer{ln: ln, password: password, responses: responses, t: t}
	go fs.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return fs
}

func (fs *fakeServer) addr() string { return fs.ln.Addr().String() }

func (fs *fakeServer) serve() {
	for {
		conn, err := fs.ln.Accept()
		if err != nil {
			return
		}
		go fs.handle(conn)
	}
}

func (fs *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	authenticated := false
	for {
		var lenBuf [4]byte
		if _, err := readFull(conn, lenBuf[:]); err != nil {
			return
		}
		length := int(binary.LittleEndian.Uint32(lenBuf[:]))
		if length < 10 {
			return
		}
		body := make([]byte, length)
		if _, err := readFull(conn, body); err != nil {
			return
		}
		reqID := int32(binary.LittleEndian.Uint32(body[0:4]))
		typ := int32(binary.LittleEndian.Uint32(body[4:8]))
		payload := string(body[8 : length-2])
		switch typ {
		case TypeAuth:
			if payload == fs.password {
				authenticated = true
				conn.Write(encodePacket(reqID, TypeAuthResponse, ""))
				conn.Write(encodePacket(-1, TypeResponseValue, ""))
			} else {
				conn.Write(encodePacket(-1, TypeAuthResponse, ""))
			}
		case TypeExecCommand:
			if !authenticated {
				conn.Write(encodePacket(-1, TypeResponseValue, ""))
				continue
			}
			resp := fs.responses[payload]
			conn.Write(encodePacket(reqID, TypeResponseValue, resp))
		}
	}
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func TestDialAuthAndCommand(t *testing.T) {
	fs := newFakeServer(t, "secret", map[string]string{
		"list": "There are 2 of a max 20 players online: Steve, Alex",
	})
	c, err := Dial(fs.addr(), "secret", 2*time.Second)
	if err != nil {
		t.Fatalf("dial/auth: %v", err)
	}
	defer c.Close()

	out, err := c.Command("list")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if !strings.Contains(out, "Steve") || !strings.Contains(out, "Alex") {
		t.Fatalf("unexpected response: %q", out)
	}
}

func TestAuthFailure(t *testing.T) {
	fs := newFakeServer(t, "secret", nil)
	_, err := Dial(fs.addr(), "wrong", 2*time.Second)
	if err == nil {
		t.Fatal("expected auth failure")
	}
	if !strings.Contains(err.Error(), ErrAuthFailed.Error()) {
		t.Fatalf("unexpected error: %v", err)
	}
}
