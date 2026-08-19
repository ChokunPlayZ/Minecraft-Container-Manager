package confstate

// Configuration-state packet IDs for the client-setup phase introduced in
// 1.20.2 (protocol 764). Clientbound configuration packet IDs changed across
// revisions as new packets (Timing, Known Packs, Feature Flags) were
// inserted, so they are bucketed by protocol. As with limbo/packets763.go
// these IDs have not been validated against a live server and must be
// verified before production use; they are kept here so a correction is a
// one-line change.

// configLayout selects the clientbound configuration packet id layout for a
// protocol revision.
type configLayout int32

const (
	layout764_765 configLayout = iota // 1.20.2-1.20.4
	layout766_768                     // 1.20.5-1.21.2
	layout769Plus                     // 1.21.3+
)

// clientboundConfig carries the clientbound configuration packet IDs for one
// layout. featureFlags is -1 when the revision predates the Feature Flags
// packet; those layouts do not send it (sendFeatureFlags false).
type clientboundConfig struct {
	registryData     int32
	updateTags       int32
	featureFlags     int32
	finishConfig     int32
	sendFeatureFlags bool
}

var configClientbound = map[configLayout]clientboundConfig{
	// 1.20.2-1.20.4: Registry Data, Update Tags, Finish Configuration. The
	// Feature Flags packet does not exist yet.
	layout764_765: {registryData: 0x07, updateTags: 0x0C, featureFlags: -1, finishConfig: 0x03},
	// 1.20.5-1.21.2: A Known Packs packet shifted Update Tags. Feature Flags
	// exists at 0x0D but is not sent by this minimal handshake; a real server
	// sends the full enabled-features set that the client needs.
	layout766_768: {registryData: 0x07, updateTags: 0x0E, featureFlags: 0x0D, finishConfig: 0x03},
	// 1.21.3+: A Timing packet was inserted at 0x00, shifting every ID by one.
	// Feature Flags is sent as part of the handshake.
	layout769Plus: {registryData: 0x08, updateTags: 0x0F, featureFlags: 0x0E, finishConfig: 0x04, sendFeatureFlags: true},
}

// layoutFor returns the configuration layout bucket for a protocol ID, or an
// error when the revision has no configuration phase (<764) or is unmapped.
func layoutFor(protocol int32) (configLayout, bool) {
	switch {
	case protocol >= 769:
		return layout769Plus, true
	case protocol >= 766:
		return layout766_768, true
	case protocol >= 764:
		return layout764_765, true
	default:
		return 0, false
	}
}

// Serverbound configuration packet IDs. These are stable across the
// configuration revisions 764+ and therefore shared by every layout.
const (
	sConfigClientInformation    = 0x00
	sConfigAckFinishConfig      = 0x01
	sConfigCookieResponse       = 0x02
	sConfigPluginMessage        = 0x03
	sConfigKnownPacks           = 0x04
	sConfigCustomQuery          = 0x05
	sConfigKeepAlive            = 0x06
	sConfigPong                 = 0x07
	sConfigResourcePackResponse = 0x08
)

// Config registry identifier used by the minimal Registry Data packet.
const configDimensionTypeRegistry = "minecraft:dimension_type"
