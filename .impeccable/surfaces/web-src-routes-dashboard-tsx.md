---
version: 1
slug: "web-src-routes-dashboard-tsx"
primary_target: "web/src/routes/dashboard.tsx"
related_targets: ["web/src/routes","web/src/components","web/src/styles/index.css"]
---

# Whole application redesign

Mode: Operate. Scope: all routes, panels, actions, catalogs, and dialogs under web/src.

## Direction contract

THESIS: An operations atlas: stable wayfinding and aligned, task-focused workspaces replace isolated cards under a sparse top bar.

OWN-WORLD: Mineral white and cool graphite surfaces, forest-teal actions, a permanent navigation rail, precise rules, system UI typography, quiet 12px panels, and consistent 8px controls. Light and dark themes serve administrators using the panel in varied ambient light.

STORY: See the fleet state, find a server, enter its workspace, operate safely, and understand the result. Dense settings use grouped sections and explicit labels.

FIRST VIEWPORT: A 224px navigation rail at left, a slim workspace bar, page title and create action, fleet summary as a single horizontal strip, then a searchable collection of server rows. Detail pages put power controls beside the server title, compact resource readings below, then underlined tabs and a large console. On mobile, navigation becomes a labeled menu and actions wrap. Signature interaction: search and state filters narrow the fleet immediately, with an explicit result count and reset path. Motion is limited to 160ms state feedback and a short dialog reveal, with reduced-motion support.

FORM: Grounded candidate 6, operations runbook / network atlas, seed ad72187a. Alternatives considered: hosting instruments, IDE workspaces, server-rack labels, Minecraft debug data, network topology, operations runbooks, game launchers. Catalog challengers are declined for the admin task: streaming wall hides state; annual plates favor presentation; airport signage overweights navigation; botanical comparison adds irrelevant stages; emission rails replace familiar controls. Retained disciplines: visible selection, strict alignment, unmistakable wayfinding, comparable readings, and state beyond color. No catalog art ships.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

Confirmed: the user delegated full design judgment and chose direct implementation.

## Dashboard and backup extension

The inherited Operate direction remains in force. Shared navigation now uses compact icon-and-label links for Servers, Backups, Settings, and Users in both rail and mobile menus. Backups opens the dedicated manager; the server workspace retains its Backups tab.

Each fleet row exposes a state-aware Start or Stop button, Manage, and a named More actions menu containing Restart, Backups, Copy server, and Force kill. Backups carries the row's server selection to the manager. Power actions show pending feedback and inline errors; unavailable operations are disabled. Force kill names the server and requires a destructive confirmation describing possible loss of unsaved world data.

At (1200px) and above, rows and the heading share a fixed (232px) actions column. From (768px) through (1199px), identity, status, memory, and host port remain aligned while actions move below a divider across the full row; the Actions heading is hidden. Below (768px), identity and status remain visible, secondary columns and the heading disappear, and actions occupy their own line.

Finish: ship, with no material findings in `.impeccable/review/dashboard-backup-update/finish-review.md`. The extension reuses existing tokens, controls, theme pairs, and confirmation behavior.

## Host capacity extension

A single Host capacity panel follows the fleet summary and precedes the server collection. CPU cores and total memory come from the selected Docker or Podman daemon, including remote hosts and runtime VMs. Two divided columns stack below (640px); labels precede tabular totals, configured limits, and accessible horizontal meters. Limits include all servers, even stopped ones; memory uses the explicit cap or the Java heap plus the runtime's default (2 GiB) overhead. Uncapped CPU counts are written separately. Overcapacity uses a written excess amount and the destructive token, while unknown host totals show Not reported and omit the meter.

Host readings load independently of the fleet, refresh every (30 seconds), prevent overlapping polls, and stop polling on unmount. A labeled refresh action supports retry. Loading uses skeletons and failures replace the readings with an explicit unavailable state. Demo totals remain confined to the labeled preview.

Finish: ship. Desktop and mobile previews in both themes show aligned content and no horizontal clipping. Capacity-specific UI tests cover totals, default overhead, uncapped CPU, overcapacity, missing metrics, loading, failure recovery, polling, and cleanup. Backend tests cover Docker and Podman host responses, authentication, and runtime failures. The existing Operate direction and theme tokens remain in force.
