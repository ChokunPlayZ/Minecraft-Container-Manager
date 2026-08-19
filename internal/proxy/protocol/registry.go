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

// SupportedVersions lists the revisions the proxy understands, newest first.
// Versions 764+ (1.20.2+) require the configuration-state handshake, which is
// dispatched on Version.HasConfigurationPhase; 762-763 use the classic
// login->play flow. Versions that use the configuration phase carry a
// per-version play packet table in internal/proxy/limbo and are served a
// minimal limbo; the transfer packet is used for 766+ when SupportsTransfer.
var SupportedVersions = []Version{
	{Protocol: 776, Name: "26.2", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 775, Name: "26.1", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 774, Name: "26.0", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 773, Name: "1.21.8", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 772, Name: "1.21.7", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 771, Name: "1.21.6", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 770, Name: "1.21.5", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 769, Name: "1.21.4", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 768, Name: "1.21.2", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 767, Name: "1.21.1", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 766, Name: "1.20.6", HasConfigurationPhase: true, SupportsTransfer: true},
	{Protocol: 765, Name: "1.20.4", HasConfigurationPhase: true, SupportsTransfer: false},
	{Protocol: 764, Name: "1.20.2", HasConfigurationPhase: true, SupportsTransfer: false},
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
	return fmt.Sprintf("MCM gateway does not support Minecraft protocol %d. Supported versions: 1.19.4+ (protocol 762+).", protocol)
}
