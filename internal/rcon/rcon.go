// Package rcon implements a minimal Minecraft RCON client (protocol
// SERVERDATA_EXECCOMMAND) over TCP, with no external dependencies.
package rcon

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// RCON packet types.
const (
	TypeResponseValue = 0
	TypeAuthResponse  = 2
	TypeExecCommand   = 2
	TypeAuth          = 3
)

// maxPacket is a defensive cap on the size of a packet we accept.
const maxPacket = 1 << 16

// ErrAuthFailed is returned when the server rejects the RCON password.
var ErrAuthFailed = errors.New("rcon authentication failed")

// Client is a small RCON client bound to a single TCP connection.
type Client struct {
	conn    net.Conn
	rw      *bufio.ReadWriter
	timeout time.Duration
}

// Dial connects to the RCON server at address and authenticates with the given
// password. It returns an error if the connection or authentication fails.
func Dial(address, password string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial rcon: %w", err)
	}
	c := &Client{
		conn:    conn,
		rw:      bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
		timeout: timeout,
	}
	if err := c.authenticate(password); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) authenticate(password string) error {
	reqID := int32(1)
	if err := c.writePacket(reqID, TypeAuth, password); err != nil {
		return err
	}
	respID, typ, payload, err := c.readPacket()
	if err != nil {
		return err
	}
	if respID == -1 {
		return ErrAuthFailed
	}
	// A successful auth response may arrive in more than one packet; drain any
	// trailing empty response frames so the next command is not confused.
	if typ == TypeAuthResponse && respID == reqID {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))
		for {
			nr, nt, _, rerr := c.tryReadPacket()
			if rerr != nil {
				break
			}
			if nt == TypeResponseValue && nr == -1 {
				continue
			}
			if nt == TypeResponseValue {
				break
			}
		}
		_ = c.conn.SetReadDeadline(time.Time{})
	}
	_ = payload
	return nil
}

// Command sends an RCON command and returns the raw response payload.
func (c *Client) Command(command string) (string, error) {
	reqID := int32(2)
	if err := c.writePacket(reqID, TypeExecCommand, command); err != nil {
		return "", err
	}
	respID, _, payload, err := c.readPacket()
	if err != nil {
		return "", err
	}
	if respID == -1 {
		return "", errors.New("rcon command rejected")
	}
	return payload, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) writePacket(requestID, typ int32, payload string) error {
	body := make([]byte, 0, len(payload)+10)
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, uint32(requestID))
	body = append(body, buf...)
	binary.LittleEndian.PutUint32(buf, uint32(typ))
	body = append(body, buf...)
	body = append(body, []byte(payload)...)
	body = append(body, 0, 0) // string terminator + padding

	header := make([]byte, 4)
	binary.LittleEndian.PutUint32(header, uint32(len(body)))

	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := c.rw.Write(header); err != nil {
		return fmt.Errorf("write rcon header: %w", err)
	}
	if _, err := c.rw.Write(body); err != nil {
		return fmt.Errorf("write rcon body: %w", err)
	}
	if err := c.rw.Flush(); err != nil {
		return fmt.Errorf("flush rcon: %w", err)
	}
	return nil
}

func (c *Client) readPacket() (requestID, typ int32, payload string, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	defer c.conn.SetReadDeadline(time.Time{})
	return c.tryReadPacket()
}

func (c *Client) tryReadPacket() (requestID, typ int32, payload string, err error) {
	var lengthBuf [4]byte
	if _, err = io.ReadFull(c.rw, lengthBuf[:]); err != nil {
		return 0, 0, "", fmt.Errorf("read rcon length: %w", err)
	}
	length := int(binary.LittleEndian.Uint32(lengthBuf[:]))
	if length < 10 || length > maxPacket {
		return 0, 0, "", fmt.Errorf("invalid rcon packet length %d", length)
	}
	body := make([]byte, length)
	if _, err = io.ReadFull(c.rw, body); err != nil {
		return 0, 0, "", fmt.Errorf("read rcon body: %w", err)
	}
	requestID = int32(binary.LittleEndian.Uint32(body[0:4]))
	typ = int32(binary.LittleEndian.Uint32(body[4:8]))
	if length > 10 {
		payload = string(body[8 : length-2])
	}
	return requestID, typ, payload, nil
}
