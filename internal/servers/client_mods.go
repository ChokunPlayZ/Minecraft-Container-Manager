package servers

import (
	"path/filepath"
	"regexp"
	"strings"
)

// clientOnlyModTokens is a curated list of normalized mod identifiers/slugs known to be
// client-only (or causing headless/AWT crashes on dedicated servers).
var clientOnlyModTokens = map[string]string{
	"missingmodschecker":     "Missing Mods Checker (GUI-only dependency prompt)",
	"missingmods":            "Missing Mods Checker",
	"oculus":                 "Oculus (client shader engine)",
	"rubidium":               "Rubidium (client renderer)",
	"embeddium":              "Embeddium (client renderer)",
	"sodium":                 "Sodium (client renderer)",
	"iris":                   "Iris (client shader engine)",
	"optifine":               "OptiFine (client graphics/shaders)",
	"entityculling":          "Entity Culling (client rendering optimization)",
	"dynamiclights":          "Dynamic Lights (client visual lighting)",
	"lambdynamiclights":      "LambDynamicLights (client visual lighting)",
	"soundphysics":           "Sound Physics (client audio processing)",
	"soundphysicsremastered": "Sound Physics Remastered (client audio processing)",
	"presencefootsteps":      "Presence Footsteps (client acoustic sound engine)",
	"waveycapes":             "Wavey Capes (client cosmetics)",
	"notenoughanimations":    "Not Enough Animations (client visual animations)",
	"fancymenu":              "FancyMenu (client GUI customizer)",
	"drippyloadingscreen":    "Drippy Loading Screen (client GUI)",
	"controlling":            "Controlling (client keybinding GUI)",
	"defaultoptions":         "Default Options (client keybinds/settings saver)",
	"customskinloader":       "Custom Skin Loader (client skin renderer)",
	"itemphysiclite":         "ItemPhysic Lite (client visual item drop physics)",
	"reauth":                 "ReAuth (client session authenticator)",
	"fallingleaves":          "Falling Leaves (client leaf particle renderer)",
	"ambientenvironment":     "Ambient Environment (client biome color blender)",
	"chatheads":              "Chat Heads (client chat player avatars)",
	"betterf3":               "Better F3 (client debug HUD)",
	"fpsreducer":             "FPS Reducer (client idle frame throttler)",
	"inventoryhud":           "Inventory HUD (client on-screen display)",
	"toastcontrol":           "Toast Control (client notification popup blocker)",
	"reessodiumoptions":      "Reese's Sodium Options (client video settings GUI)",
	"sodiumextra":            "Sodium Extra (client rendering options)",
	"indium":                 "Indium (client rendering adapter)",
	"borderlessmining":       "Borderless Mining (client display window mode)",
	"customcrosshairmod":     "Custom Crosshair Mod (client HUD crosshair)",
	"shoulder架sur":           "Shoulder Surfing Reloaded (client third-person camera)",
	"shouldersurfing":        "Shoulder Surfing Reloaded (client third-person camera)",
	"smoothboot":             "Smooth Boot (client CPU scheduling tweak)",
}

// clientOnlyExactModIDs maps exact mod IDs (from fabric.mod.json or mods.toml) to reasons.
var clientOnlyExactModIDs = map[string]string{
	"missingmodschecker":     "Missing Mods Checker",
	"oculus":                 "Oculus",
	"rubidium":               "Rubidium",
	"embeddium":              "Embeddium",
	"sodium":                 "Sodium",
	"iris":                   "Iris",
	"optifine":               "OptiFine",
	"entityculling":          "Entity Culling",
	"lambdynamiclights":      "LambDynamicLights",
	"sound_physics_remastered": "Sound Physics Remastered",
	"presencefootsteps":      "Presence Footsteps",
	"waveycapes":             "Wavey Capes",
	"notenoughanimations":    "Not Enough Animations",
	"fancymenu":              "FancyMenu",
	"drippyloadingscreen":    "Drippy Loading Screen",
	"controlling":            "Controlling",
	"defaultoptions":         "Default Options",
	"customskinloader":       "Custom Skin Loader",
	"itemphysiclite":         "ItemPhysic Lite",
	"reauth":                 "ReAuth",
	"fallingleaves":          "Falling Leaves",
	"ambientenvironment":     "Ambient Environment",
	"chat_heads":             "Chat Heads",
	"betterf3":               "Better F3",
	"fpsreducer":             "FPS Reducer",
	"toastcontrol":           "Toast Control",
	"reeses_sodium_options":  "Reese's Sodium Options",
	"sodium_extra":           "Sodium Extra",
	"indium":                 "Indium",
	"borderlessmining":       "Borderless Mining",
	"shouldersurfing":        "Shoulder Surfing",
	"smoothboot":             "Smooth Boot",
}

var modVersionPattern = regexp.MustCompile(`[+\-_](?:mc)?(?:v)?\d+.*$`)

// normalizeToken strips non-alphanumeric characters and converts to lowercase.
func normalizeToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// extractBaseModName removes directory prefixes, extensions, and version strings.
// e.g. "mods/missing-mods-checker-1.20.1-1.0.jar" -> "missing-mods-checker"
func extractBaseModName(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".disabled")
	base = strings.TrimSuffix(base, ".jar")
	// Strip trailing version pattern if present
	loc := modVersionPattern.FindStringIndex(base)
	if loc != nil && loc[0] > 0 {
		base = base[:loc[0]]
	}
	return strings.ToLower(base)
}

// IsClientOnlyFilename checks if a filename or path matches a known client-only mod (Tier 1).
func IsClientOnlyFilename(path string) (bool, string) {
	baseName := extractBaseModName(path)
	norm := normalizeToken(baseName)

	if reason, ok := clientOnlyModTokens[norm]; ok {
		return true, reason
	}

	// Also check if any known client token is an exact prefix or suffix segment
	for token, reason := range clientOnlyModTokens {
		if norm == token {
			return true, reason
		}
		// Match word boundary token in norm (e.g. "missingmodscheckerforge" or "oculusmc")
		if strings.HasPrefix(norm, token) {
			rest := norm[len(token):]
			if rest == "" || strings.HasPrefix(rest, "forge") || strings.HasPrefix(rest, "fabric") || strings.HasPrefix(rest, "neoforge") || strings.HasPrefix(rest, "mc") {
				return true, reason
			}
		}
	}

	// Fallback: check if the raw filename before any delimiter starts with a client token
	rawBase := filepath.Base(path)
	rawBase = strings.TrimSuffix(rawBase, ".disabled")
	rawBase = strings.TrimSuffix(rawBase, ".jar")
	rawTokens := strings.FieldsFunc(rawBase, func(r rune) bool {
		return r == '-' || r == '_' || r == '+' || r == '.' || r == ' '
	})
	if len(rawTokens) > 0 {
		firstToken := normalizeToken(rawTokens[0])
		if reason, ok := clientOnlyModTokens[firstToken]; ok {
			return true, reason
		}
	}

	return false, ""
}

// IsClientOnlyModID checks if a mod ID from manifest matches a known client-only mod.
func IsClientOnlyModID(modID string) (bool, string) {
	if modID == "" {
		return false, ""
	}
	cleaned := strings.ToLower(strings.TrimSpace(modID))
	if reason, ok := clientOnlyExactModIDs[cleaned]; ok {
		return true, reason
	}
	norm := normalizeToken(cleaned)
	if reason, ok := clientOnlyModTokens[norm]; ok {
		return true, reason
	}
	return false, ""
}
