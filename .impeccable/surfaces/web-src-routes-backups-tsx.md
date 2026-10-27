---
version: 1
slug: "web-src-routes-backups-tsx"
primary_target: "web/src/routes/backups.tsx"
related_targets: ["web/src/components/backups-panel.tsx","web/src/components/app-shell.tsx","web/src/styles/index.css"]
---

# Dedicated backup manager

Mode: Operate. Extend the inherited Operations atlas system in DESIGN.md; retain its tokens, flat ruled panels, native system typography, compact controls, paired themes, and bounded confirmations.

## Purpose and direction

Give administrators a dedicated Backups destination for selecting a server, managing its archives, and saving its automatic backup schedule. The server workspace retains its Backups tab and shares the archive panel. Place server identity and written status before the backup tasks so every operation has a visible target.

The page begins with its title and Refresh servers action, followed by a labeled server selector and Manage server link. The selected server's archive panel contains count and stored-size summary, optional name and local/S3 storage fields, Create backup and Upload actions, then searchable archives with a storage filter and result count. Automatic backups follow in a separate card with an enable switch, interval in minutes, and Save schedule. Scheduled backups use the panel's default storage; the archive storage selector governs manual creation and upload.

## Interaction and states

An optional `server` query selects the requested server, including links from dashboard rows. Without one, the first available server is selected. An unavailable requested server produces a warning and requires another selection. Server loading uses skeletons; failures offer retry; an empty fleet links to Servers.

Archive creation, upload, download, restore, and deletion use existing backup APIs. Restore and deletion require explicit destructive confirmations. Busy operations disable conflicting archive controls and expose progress; upload shows transfer percentage, operations show completion or failure, and deletion identifies the active row. Only completed archives can be restored or downloaded. Loading, retry, no archives, and no filter matches have explicit feedback; filter-empty results provide Reset filters. Archive names wrap and metadata retains creation time, size, storage, and written status.

The schedule accepts whole-minute intervals of at least five minutes when enabled. Save changes only backup enablement and interval, with validation, saving, error, and saved states. Disabled schedules keep the interval field unavailable.

## Responsive behavior

The selector and status/actions wrap within the shared shell. Below (640px), archive fields stack and the storage filter wraps beneath search; from (640px), the create form has two columns with actions beneath; from (1280px), name, storage, and actions share a row. Archive identity and actions wrap as space requires. Schedule fields and save controls wrap, and shared dialogs remain bounded to the viewport. Light and dark themes retain the same order and geometry.

Finish: ship, with no material findings in `.impeccable/review/dashboard-backup-update/finish-review.md`. No new visual world or raster asset was introduced.
