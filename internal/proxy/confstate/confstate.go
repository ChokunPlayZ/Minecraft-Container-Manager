package confstate

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/mcm-panel/mcm/internal/proxy/protocol"
)

// defaultTimeout bounds the entire configuration handshake including each read
// of a serverbound config packet. It is conservative so a stalled client
// cannot hang the proxy indefinitely.
const defaultTimeout = 10 * time.Second

// Options configures one configuration-state handshake.
type Options struct {
	Logger  *log.Logger
	Version protocol.Version
	// Timeout bounds each serverbound read and, combined with a sane loop, the
	// whole handshake. <=0 uses defaultTimeout.
	Timeout time.Duration
}

// ConfigError is a clean configuration-state failure that the caller can log
// via the existing session error handling (it is not a panic path).
type ConfigError struct{ reason string }

func (e *ConfigError) Error() string { return "configuration failed: " + e.reason }

// RunConfig drives the configuration-state handshake over conn using the live
// frame states rs/ws (encryption and compression are already negotiated by the
// login phase). It sends Registry Data, Feature Flags (revisions that have
// it), Update Tags, then Finish Configuration, and consumes serverbound
// configuration packets until the client acknowledges Finish Configuration,
// at which point the caller transitions to Play state. A stray serverbound
// config packet (Plugin Message, Cookie Response, Known Packs, Pong, etc.) is
// consumed and ignored so it does not tear the connection.
func RunConfig(ctx context.Context, conn net.Conn, rs *protocol.ReaderState, ws *protocol.WriterState, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	layout, ok := layoutFor(opts.Version.Protocol)
	if !ok {
		return &ConfigError{reason: fmt.Sprintf("protocol %d has no configuration layout", opts.Version.Protocol)}
	}
	ids := configClientbound[layout]

	write := func(id int32, payload []byte) error {
		frame, err := ws.WritePacket(id, payload)
		if err != nil {
			return err
		}
		_, err = conn.Write(frame)
		return err
	}

	// Canonical server order: registry and feature data first, tags, then
	// Finish Configuration which signals the client to acknowledge.
	reg, err := buildRegistryData()
	if err != nil {
		return err
	}
	if err := write(ids.registryData, reg); err != nil {
		return err
	}
	if ids.sendFeatureFlags {
		flags, err := buildFeatureFlags()
		if err != nil {
			return err
		}
		if err := write(ids.featureFlags, flags); err != nil {
			return err
		}
	}
	tags, err := buildUpdateTags()
	if err != nil {
		return err
	}
	if err := write(ids.updateTags, tags); err != nil {
		return err
	}
	if err := write(ids.finishConfig, nil); err != nil {
		return err
	}

	// Consume serverbound config packets until the Acknowledge Finish
	// Configuration packet signals the transition to Play.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		pkt, err := rs.ReadPacket()
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil {
			return err
		}
		switch pkt.ID {
		case sConfigAckFinishConfig:
			return nil
		default:
			// Client Information, Plugin Message, Cookie Response, Known
			// Packs, Pong, Resource Pack Response, etc. are consumed and
			// ignored.
		}
	}
}

// buildRegistryData builds a minimal-but-structurally-valid Registry Data
// payload: a count of registries, then per registry a String id and an NBT
// compound of its entries. A single empty dimension_type registry mirrors the
// minimal dimension codec the limbo sends; real servers populate full
// registries.
func buildRegistryData() ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteVarInt(w, 1); err != nil {
		return nil, err
	}
	if err := protocol.WriteString(w, configDimensionTypeRegistry); err != nil {
		return nil, err
	}
	n := protocol.NewNBTWriter()
	n.Root("")
	n.End()
	_, _ = w.Write(n.Bytes())
	return w.Bytes(), nil
}

// buildFeatureFlags builds a Feature Flags payload (1.21.3+). An empty flag
// list is structurally valid; real servers advertise their data pack / feature
// set.
func buildFeatureFlags() ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteVarInt(w, 0); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// buildUpdateTags builds an Update Tags payload. The wire format is a count of
// registries, each with a tag list; count 0 (no registries) is structurally
// valid. Real servers send the full tag hierarchy.
func buildUpdateTags() ([]byte, error) {
	w := protocol.NewWriter()
	if err := protocol.WriteVarInt(w, 0); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
