package auth

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Permission describes an exact capability. Wildcards are never accepted in
// custom roles. Server capabilities also require servers.view and server scope.
type Permission struct {
	ID           string `json:"id"`
	Group        string `json:"group"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	ServerScoped bool   `json:"server_scoped"`
}

var PermissionCatalog = []Permission{
	{"servers.view", "Servers", "View servers", "List accessible servers and their status.", true},
	{"servers.create", "Servers", "Create servers", "Create servers. Requires access to all servers.", false},
	{"servers.edit", "Servers", "Edit configuration", "Change server settings, properties and runtime configuration.", true},
	{"servers.delete", "Servers", "Delete servers", "Permanently remove servers and their data.", true},
	{"servers.power", "Servers", "Power controls", "Start, stop, restart and force kill servers.", true},
	{"servers.rebuild", "Servers", "Install and rebuild", "Install software and recreate containers.", true},
	{"servers.export", "Servers", "Export servers", "Download a complete copy including configuration and files.", true},
	{"servers.copy", "Servers", "Copy servers", "Duplicate servers. Also requires create servers and read files.", true},
	{"console.read", "Console & logs", "Read console", "Stream live console output, which may contain sensitive data.", true},
	{"console.write", "Console & logs", "Send commands", "Run arbitrary Minecraft console commands.", true},
	{"logs.read", "Console & logs", "Read saved logs", "Browse saved server logs.", true},
	{"files.read", "Files", "Read and download files", "Read all server files, including secrets and player data.", true},
	{"files.write", "Files", "Write files", "Upload, edit, extract, rename and download files onto the server; may install executable code.", true},
	{"files.delete", "Files", "Delete files", "Remove server files and folders.", true},
	{"players.read", "Players", "View players", "List players, operators and whitelist entries.", true},
	{"players.manage", "Players", "Manage players", "Kick, ban, give items and change player state. OP and custom commands need separate permissions.", true},
	{"ops.manage", "Players", "Manage operators", "Grant or remove Minecraft operator privileges.", true},
	{"whitelist.manage", "Players", "Manage whitelist", "Add and remove whitelist entries.", true},
	{"mods.read", "Mods & worlds", "View mods and plugins", "Browse installed mods, plugins and modpacks.", true},
	{"mods.manage", "Mods & worlds", "Manage mods and plugins", "Install, update or remove executable mods, plugins and modpacks.", true},
	{"worlds.read", "Mods & worlds", "View worlds", "Browse worlds, regions and chunks.", true},
	{"worlds.manage", "Mods & worlds", "Manage worlds", "Create, import, activate, delete worlds and reset chunks.", true},
	{"worlds.export", "Mods & worlds", "Export worlds", "Download world archives.", true},
	{"backups.read", "Backups & schedules", "View backups", "List backups and watch backup progress.", true},
	{"backups.create", "Backups & schedules", "Create and upload backups", "Create snapshots and upload backup archives.", true},
	{"backups.restore", "Backups & schedules", "Restore backups", "Replace server data from a backup, including executable files.", true},
	{"backups.delete", "Backups & schedules", "Delete backups", "Permanently remove backups.", true},
	{"backups.download", "Backups & schedules", "Download backups", "Download complete backups, including configuration and secrets.", true},
	{"schedules.read", "Backups & schedules", "View schedules", "Read scheduled tasks and their commands.", true},
	{"schedules.manage", "Backups & schedules", "Manage schedules", "Create and manage persistent automation. Requires power controls, send commands and create backups.", true},
	{"performance.read", "Monitoring & DNS", "View performance", "Read resource usage and performance history.", true},
	{"performance.manage", "Monitoring & DNS", "Manage profiling", "Change collection settings and capture spark reports.", true},
	{"dns.read", "Monitoring & DNS", "View server DNS", "Read published join addresses.", true},
	{"dns.manage", "Monitoring & DNS", "Publish server DNS", "Publish or remove DNS records using configured providers.", true},
	{"system.read", "Panel administration", "View host status", "Read host capacity and container runtime status.", false},
	{"settings.manage", "Panel administration", "Manage shared settings", "Read and change shared configuration, credentials, DNS providers and notifications.", false},
	{"updates.manage", "Panel administration", "Manage update checks", "Configure and run update checks across all servers.", false},
	{"panel.update", "Panel administration", "Update the panel", "Check and install MCM application updates.", false},
	{"users.manage", "Panel administration", "Manage users and permissions", "Full account administration, including creating administrators and resetting passwords.", false},
}

type RolePreset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func RolePresets() []RolePreset {
	return []RolePreset{
		{"administrator", "Administrator", "Full access to every server and all panel administration.", allPermissions()},
		{"operator", "Operator", "Operate assigned servers, manage content and create backups. No panel administration or server deletion.", []string{"servers.view", "servers.edit", "servers.power", "servers.rebuild", "console.read", "console.write", "logs.read", "files.read", "files.write", "files.delete", "players.read", "players.manage", "ops.manage", "whitelist.manage", "mods.read", "mods.manage", "worlds.read", "worlds.manage", "worlds.export", "backups.read", "backups.create", "backups.download", "schedules.read", "schedules.manage", "performance.read", "performance.manage", "dns.read"}},
		{"viewer", "Viewer", "Read server status, console, logs, players, content lists and monitoring. Cannot change server data.", []string{"servers.view", "console.read", "logs.read", "players.read", "mods.read", "worlds.read", "backups.read", "schedules.read", "performance.read", "dns.read"}},
		{"custom", "Custom", "Choose individual permissions for this account.", []string{}},
	}
}

func allPermissions() []string {
	out := make([]string, 0, len(PermissionCatalog))
	for _, p := range PermissionCatalog {
		out = append(out, p.ID)
	}
	return out
}

type Access struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	AllServers  bool     `json:"all_servers"`
	ServerIDs   []string `json:"server_ids"`
	Disabled    bool     `json:"disabled"`
}

func (a Access) EffectivePermissions() []string {
	for _, role := range RolePresets() {
		if role.ID == a.Role && role.ID != "custom" {
			return role.Permissions
		}
	}
	if a.Role == "custom" {
		return slices.Clone(a.Permissions)
	}
	return []string{}
}

func (a Access) Has(permission string) bool {
	return !a.Disabled && slices.Contains(a.EffectivePermissions(), permission)
}

func (a Access) CanAccessServer(id string) bool {
	return a.Has("servers.view") && (a.Role == "administrator" || a.AllServers || slices.Contains(a.ServerIDs, id))
}

func (a Access) Can(permission, serverID string) bool {
	return a.Has(permission) && (serverID == "" || a.CanAccessServer(serverID))
}

// Normalize validates user-supplied access before any account write.
func (a *Access) Normalize() error {
	if !slices.Contains([]string{"administrator", "operator", "viewer", "custom"}, a.Role) {
		return errors.New("unknown role")
	}
	known := allPermissions()
	for _, p := range a.Permissions {
		if !slices.Contains(known, p) {
			return fmt.Errorf("unknown permission: %s", p)
		}
	}
	if a.Role != "custom" {
		a.Permissions = []string{}
	}
	if a.Permissions == nil {
		a.Permissions = []string{}
	}
	slices.Sort(a.Permissions)
	a.Permissions = slices.Compact(a.Permissions)
	if a.Role == "administrator" {
		a.AllServers = true
	}
	if a.AllServers {
		a.ServerIDs = []string{}
	}
	if a.ServerIDs == nil {
		a.ServerIDs = []string{}
	}
	if len(a.ServerIDs) > 1000 {
		return errors.New("too many server assignments")
	}
	for _, id := range a.ServerIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("server IDs cannot be empty")
		}
	}
	slices.Sort(a.ServerIDs)
	a.ServerIDs = slices.Compact(a.ServerIDs)
	if !a.Has("servers.view") && !a.Disabled {
		for _, p := range PermissionCatalog {
			if (p.ServerScoped || p.ID == "servers.create") && a.Has(p.ID) {
				return errors.New("server permissions also require servers.view")
			}
		}
	}
	if a.Has("servers.create") && !a.AllServers {
		return errors.New("creating servers requires access to all servers")
	}
	if a.Has("servers.copy") && (!a.Has("servers.create") || !a.Has("files.read")) {
		return errors.New("copying servers also requires servers.create and files.read")
	}
	if a.Has("schedules.manage") {
		for _, p := range []string{"servers.power", "console.write", "backups.create"} {
			if !a.Has(p) {
				return fmt.Errorf("managing schedules also requires %s", p)
			}
		}
	}
	return nil
}
