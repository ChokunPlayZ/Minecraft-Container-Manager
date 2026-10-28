// Package buildinfo describes the source used to build the running executable.
package buildinfo

import "runtime/debug"

// These values are supplied by the build. Plain go builds use Go's VCS metadata.
var (
	Commit  = ""
	BuiltAt = ""
	Dirty   = ""
)

type Info struct {
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at,omitempty"`
	Dirty   bool   `json:"dirty"`
}

func Current() Info {
	info := Info{Commit: Commit, BuiltAt: BuiltAt, Dirty: Dirty == "true"}
	if metadata, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range metadata.Settings {
			switch setting.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = setting.Value
				}
			case "vcs.modified":
				if Dirty == "" {
					info.Dirty = setting.Value == "true"
				}
			}
		}
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	return info
}
