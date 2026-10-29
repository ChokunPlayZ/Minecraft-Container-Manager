// Demo data mirrors the server catalog. The live API is authoritative.
import type { PermissionCatalog } from './types';
export const demoPermissionCatalog: PermissionCatalog = {
  "permissions": [
    {
      "id": "servers.view",
      "group": "Servers",
      "label": "View servers",
      "description": "List accessible servers and their status.",
      "server_scoped": true
    },
    {
      "id": "servers.create",
      "group": "Servers",
      "label": "Create servers",
      "description": "Create servers. Requires access to all servers.",
      "server_scoped": false
    },
    {
      "id": "servers.edit",
      "group": "Servers",
      "label": "Edit configuration",
      "description": "Change server settings, properties and runtime configuration.",
      "server_scoped": true
    },
    {
      "id": "servers.delete",
      "group": "Servers",
      "label": "Delete servers",
      "description": "Permanently remove servers and their data.",
      "server_scoped": true
    },
    {
      "id": "servers.power",
      "group": "Servers",
      "label": "Power controls",
      "description": "Start, stop, restart and force kill servers.",
      "server_scoped": true
    },
    {
      "id": "servers.rebuild",
      "group": "Servers",
      "label": "Install and rebuild",
      "description": "Install software and recreate containers.",
      "server_scoped": true
    },
    {
      "id": "servers.export",
      "group": "Servers",
      "label": "Export servers",
      "description": "Download a complete copy including configuration and files.",
      "server_scoped": true
    },
    {
      "id": "servers.copy",
      "group": "Servers",
      "label": "Copy servers",
      "description": "Duplicate servers. Also requires create servers and read files.",
      "server_scoped": true
    },
    {
      "id": "console.read",
      "group": "Console & logs",
      "label": "Read console",
      "description": "Stream live console output, which may contain sensitive data.",
      "server_scoped": true
    },
    {
      "id": "console.write",
      "group": "Console & logs",
      "label": "Send commands",
      "description": "Run arbitrary Minecraft console commands.",
      "server_scoped": true
    },
    {
      "id": "logs.read",
      "group": "Console & logs",
      "label": "Read saved logs",
      "description": "Browse saved server logs.",
      "server_scoped": true
    },
    {
      "id": "files.read",
      "group": "Files",
      "label": "Read and download files",
      "description": "Read all server files, including secrets and player data.",
      "server_scoped": true
    },
    {
      "id": "files.write",
      "group": "Files",
      "label": "Write files",
      "description": "Upload, edit, extract, rename and download files onto the server; may install executable code.",
      "server_scoped": true
    },
    {
      "id": "files.delete",
      "group": "Files",
      "label": "Delete files",
      "description": "Remove server files and folders.",
      "server_scoped": true
    },
    {
      "id": "players.read",
      "group": "Players",
      "label": "View players",
      "description": "List players, operators and whitelist entries.",
      "server_scoped": true
    },
    {
      "id": "players.manage",
      "group": "Players",
      "label": "Manage players",
      "description": "Kick, ban, give items and change player state. OP and custom commands need separate permissions.",
      "server_scoped": true
    },
    {
      "id": "ops.manage",
      "group": "Players",
      "label": "Manage operators",
      "description": "Grant or remove Minecraft operator privileges.",
      "server_scoped": true
    },
    {
      "id": "whitelist.manage",
      "group": "Players",
      "label": "Manage whitelist",
      "description": "Add and remove whitelist entries.",
      "server_scoped": true
    },
    {
      "id": "mods.read",
      "group": "Mods & worlds",
      "label": "View mods and plugins",
      "description": "Browse installed mods, plugins and modpacks.",
      "server_scoped": true
    },
    {
      "id": "mods.manage",
      "group": "Mods & worlds",
      "label": "Manage mods and plugins",
      "description": "Install, update or remove executable mods, plugins and modpacks.",
      "server_scoped": true
    },
    {
      "id": "worlds.read",
      "group": "Mods & worlds",
      "label": "View worlds",
      "description": "Browse worlds, regions and chunks.",
      "server_scoped": true
    },
    {
      "id": "worlds.manage",
      "group": "Mods & worlds",
      "label": "Manage worlds",
      "description": "Create, import, activate, delete worlds and reset chunks.",
      "server_scoped": true
    },
    {
      "id": "worlds.export",
      "group": "Mods & worlds",
      "label": "Export worlds",
      "description": "Download world archives.",
      "server_scoped": true
    },
    {
      "id": "backups.read",
      "group": "Backups & schedules",
      "label": "View backups",
      "description": "List backups and watch backup progress.",
      "server_scoped": true
    },
    {
      "id": "backups.create",
      "group": "Backups & schedules",
      "label": "Create and upload backups",
      "description": "Create snapshots and upload backup archives.",
      "server_scoped": true
    },
    {
      "id": "backups.restore",
      "group": "Backups & schedules",
      "label": "Restore backups",
      "description": "Replace server data from a backup, including executable files.",
      "server_scoped": true
    },
    {
      "id": "backups.delete",
      "group": "Backups & schedules",
      "label": "Delete backups",
      "description": "Permanently remove backups.",
      "server_scoped": true
    },
    {
      "id": "backups.download",
      "group": "Backups & schedules",
      "label": "Download backups",
      "description": "Download complete backups, including configuration and secrets.",
      "server_scoped": true
    },
    {
      "id": "schedules.read",
      "group": "Backups & schedules",
      "label": "View schedules",
      "description": "Read scheduled tasks and their commands.",
      "server_scoped": true
    },
    {
      "id": "schedules.manage",
      "group": "Backups & schedules",
      "label": "Manage schedules",
      "description": "Create and manage persistent automation. Requires power controls, send commands and create backups.",
      "server_scoped": true
    },
    {
      "id": "performance.read",
      "group": "Monitoring & DNS",
      "label": "View performance",
      "description": "Read resource usage and performance history.",
      "server_scoped": true
    },
    {
      "id": "performance.manage",
      "group": "Monitoring & DNS",
      "label": "Manage profiling",
      "description": "Change collection settings and capture spark reports.",
      "server_scoped": true
    },
    {
      "id": "dns.read",
      "group": "Monitoring & DNS",
      "label": "View server DNS",
      "description": "Read published join addresses.",
      "server_scoped": true
    },
    {
      "id": "dns.manage",
      "group": "Monitoring & DNS",
      "label": "Publish server DNS",
      "description": "Publish or remove DNS records using configured providers.",
      "server_scoped": true
    },
    {
      "id": "system.read",
      "group": "Panel administration",
      "label": "View host status",
      "description": "Read host capacity and container runtime status.",
      "server_scoped": false
    },
    {
      "id": "settings.manage",
      "group": "Panel administration",
      "label": "Manage shared settings",
      "description": "Read and change shared configuration, credentials, DNS providers and notifications.",
      "server_scoped": false
    },
    {
      "id": "updates.manage",
      "group": "Panel administration",
      "label": "Manage update checks",
      "description": "Configure and run update checks across all servers.",
      "server_scoped": false
    },
    {
      "id": "panel.update",
      "group": "Panel administration",
      "label": "Update the panel",
      "description": "Check and install MCM application updates.",
      "server_scoped": false
    },
    {
      "id": "users.manage",
      "group": "Panel administration",
      "label": "Manage users and permissions",
      "description": "Full account administration, including creating administrators and resetting passwords.",
      "server_scoped": false
    }
  ],
  "roles": [
    {
      "id": "administrator",
      "label": "Administrator",
      "description": "Full access to every server and all panel administration.",
      "permissions": [
        "servers.view",
        "servers.create",
        "servers.edit",
        "servers.delete",
        "servers.power",
        "servers.rebuild",
        "servers.export",
        "servers.copy",
        "console.read",
        "console.write",
        "logs.read",
        "files.read",
        "files.write",
        "files.delete",
        "players.read",
        "players.manage",
        "ops.manage",
        "whitelist.manage",
        "mods.read",
        "mods.manage",
        "worlds.read",
        "worlds.manage",
        "worlds.export",
        "backups.read",
        "backups.create",
        "backups.restore",
        "backups.delete",
        "backups.download",
        "schedules.read",
        "schedules.manage",
        "performance.read",
        "performance.manage",
        "dns.read",
        "dns.manage",
        "system.read",
        "settings.manage",
        "updates.manage",
        "panel.update",
        "users.manage"
      ]
    },
    {
      "id": "operator",
      "label": "Operator",
      "description": "Operate assigned servers, manage content and create backups. No panel administration or server deletion.",
      "permissions": [
        "servers.view",
        "servers.edit",
        "servers.power",
        "servers.rebuild",
        "console.read",
        "console.write",
        "logs.read",
        "files.read",
        "files.write",
        "files.delete",
        "players.read",
        "players.manage",
        "ops.manage",
        "whitelist.manage",
        "mods.read",
        "mods.manage",
        "worlds.read",
        "worlds.manage",
        "worlds.export",
        "backups.read",
        "backups.create",
        "backups.download",
        "schedules.read",
        "schedules.manage",
        "performance.read",
        "performance.manage",
        "dns.read"
      ]
    },
    {
      "id": "viewer",
      "label": "Viewer",
      "description": "Read server status, console, logs, players, content lists and monitoring. Cannot change server data.",
      "permissions": [
        "servers.view",
        "console.read",
        "logs.read",
        "players.read",
        "mods.read",
        "worlds.read",
        "backups.read",
        "schedules.read",
        "performance.read",
        "dns.read"
      ]
    },
    {
      "id": "custom",
      "label": "Custom",
      "description": "Choose individual permissions for this account.",
      "permissions": []
    }
  ],
  "servers": [
    {
      "id": "demo",
      "name": "Mega SMP Server"
    },
    {
      "id": "creative",
      "name": "Creative Realm"
    }
  ]
};
