package protocol

import "fmt"

// Version describes a supported Minecraft Java edition protocol revision.
type Version struct {
	// Protocol is the network protocol ID.
	Protocol int32
	// Name is the user-facing version string (e.g. "1.21.1").
	Name string
	// HasConfigurationPhase reports whether this version uses the configuration
	// state (1.20.2+). Versions without it use the legacy login->play flow.
	HasConfigurationPhase bool
	// SupportsTransfer reports whether this version supports the Transfer
	// packet (1.20.5+ / protocol 766+).
	SupportsTransfer bool
}

// SupportedVersions lists the revisions the proxy understands. The play
// (limbo) handshake is implemented for the classic login->play flow, so
// versions requiring the configuration phase are recognized but routed to a
// graceful disconnect rather than a broken session.
var SupportedVersions = []Version{
	{Protocol: 767, Name: "1.21.1", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 766, Name: "1.20.6", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 765, Name: "1.20.5", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 764, Name: "1.20.3", HasConfigurationPhase: true, SupportsTransfer: false},
	{Protocol: 763, Name: "1.20.1", HasConfigurationPhase: false, SupportsTransfer: false},
	{Protocol: 762, Name: "1.19.4", HasConfigurationPhase: false, SupportsTransfer: false},
}

// Lookup returns the Version for a protocol ID, or an error when unsupported.
func Lookup(protocol int32) (Version, error) {
	for _, v := range SupportedVersions {
		if v.Protocol == protocol {
			return v, nil
		}
	}
	return Version{}, fmt.Errorf("unsupported protocol version %d", protocol)
}

// DisconnectReason builds a human-readable reason for unsupported versions.
func DisconnectReason(protocol int32) string {
	return fmt.Sprintf("MCM gateway does not support Minecraft protocol %d. Supported versions: 1.20.1+ (protocol 763+).", protocol)
}
