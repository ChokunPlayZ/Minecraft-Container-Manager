---
version: 1
slug: "web-src-routes-backups-tsx"
primary_target: "web/src/routes/backups.tsx"
related_targets: ["web/src/components/backups-panel.tsx","web/src/components/backup-retention-settings.tsx","web/src/components/app-shell.tsx","web/src/styles/index.css"]
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

## Retention and recoverable trash extension

This local Operate extension adds Saved backups, Trash, and Retention views inside the shared archive panel. Retain the existing server context, flat bordered card, ruled rows, compact controls, semantic theme pairs, written statuses, and shared Button, Input, Label, Skeleton, and confirmation-dialog components. The selected view uses the existing secondary button treatment; protection uses the existing primary color and permanent deletion uses the destructive color. No new durable tokens, typography, assets, or visual identity are introduced.

Saved backups retain creation, upload, search, storage filtering, download, and restore. Completed snapshots can be protected from retention and deletion. Moving an eligible snapshot to trash requires confirmation; protected snapshots cannot be selected for that action. Bulk selection offers the same actions as individual rows and reports completed work if an operation stops partway through. Trash shows its stored size, expiry rule, and each archive's trash and expiry times. Recover returns a snapshot to the saved list and protects completed snapshots without changing the live server world. Individual permanent deletion, selected permanent deletion, and Empty trash require a confirmation that states the irreversible storage removal.

Retention groups three labeled integer fields: Backups to keep, Maximum age (days), and Keep trash (days). Their help text explains zero values and existing-trash behavior. Limits apply across local and S3 storage; protected snapshots are additional to the count limit, and the newest successful snapshot remains available. Saving a policy does not run cleanup. The saved-policy preview separates moves to trash from permanent deletions, names the affected archives and reasons, and distinguishes stored trash bytes from bytes freed by deletion. Unsaved changes disable preview refresh and cleanup until saved or discarded. Run cleanup now confirms the preview's consequences and refreshes results after success or partial failure. Loading, retry, invalid input, no cleanup candidates, saving, read-only permissions, operation errors, and completion notices remain explicit.

The three retention fields share a row from the existing medium breakpoint and stack on narrow screens. View controls, row actions, bulk actions, and preview headings wrap within the panel; archive names and expiry metadata wrap within their rows. The preview list has its own bounded scrolling area. These are local arrangements built from the incumbent spacing and responsive utilities.

## Retention evidence and boundaries

Source inspection covered the shared archive and retention components, API and policy implementation, migration, interaction tests, README semantics, and the existing product and design documents. The six dark demo captures in `.impeccable/review/` (`saved-desktop.png`, `saved-mobile.png`, `trash-desktop.png`, `trash-mobile.png`, `retention-desktop.png`, and `retention-mobile.png`) show the populated views, protected state, stored trash, expiry guidance, cleanup preview, desktop field row, and stacked mobile layout. These captures verify the inherited shell and component language at both sizes. Light-theme, confirmation, validation, error, permission, and mutation behavior are supported by source inspection and tests rather than additional screenshots.

The finish reviewer disposition is SHIP with no material findings after reviewing all six captures and the implementation. The implementation handoff reports passing full Go tests, backup race checks, Go vet, and targeted backend/database checks covering the migration and worker. The full frontend suite passed 453 tests, followed by all nine retention interaction tests after adding the bulk-selection, protected-snapshot, and partial-failure check. Type checking and the production build passed; lint reports only pre-existing warnings. This record changes only the owning surface brief. PRODUCT.md, DESIGN.md, `.impeccable/design.json`, and unrelated surface guidance remain unchanged; no pre-existing design drift is repaired.
