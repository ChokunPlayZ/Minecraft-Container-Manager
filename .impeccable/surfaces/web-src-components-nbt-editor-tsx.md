---
version: 1
slug: "web-src-components-nbt-editor-tsx"
primary_target: "web/src/components/nbt-editor.tsx"
related_targets: ["web/src/components/player-data-panel.tsx", "web/src/components/file-manager.tsx", "web/src/components/players-panel.tsx", "web/src/api/nbt.ts", "web/src/lib/nbt.ts", "web/src/routes/servers.$id.tsx"]
---

# NBT browser and player data

Mode: Operate. Local extension of the existing server workspace.

## Direction contract

Purpose: Browse and edit standalone Java Edition NBT files, with a direct workflow for saved player data, including offline players.

World: Inherit DESIGN.md's Operations atlas, paired light/dark tokens, system typography, shared Button/Input/Select/Textarea controls, flat bordered panels and confirmation dialogs. Monospace identifies file paths, tag names and raw values. This feature adds no global tokens or visual system changes.

Entry points: Players > Player data discovers saved player files and searches local cached names, UUIDs and worlds. Selecting a file replaces the library with its editor. Files opens recognized NBT filenames directly and provides Open as NBT for other files. Back to browser returns to the originating library or folder.

Composition: File identity and Reload/Save NBT actions lead the editor. Compression and editability remain explicit. Player quick edit exposes existing health, food, saturation, XP, game mode, dimension and position tags, with shortcuts to inventory, ender chest and abilities. The tag browser and selected tag form share a divided panel: side by side at the large breakpoint, stacked on narrower screens. Selecting a tag on narrow screens scrolls its detail form into view. Quick fields use two columns on narrow screens and four at the extra-large breakpoint. Long paths wrap and the tag list scrolls within its own container.

Interaction: Search names, paths and values, expand compounds and lists, select a tag to edit its name or value, and add or remove child tags and list items. Lists retain a common element type; arrays accept signed integers separated by spaces or commas. Numeric strings preserve full 64-bit values across JSON. Duplicate sibling names and invalid numeric values block saving. Editing level.dat can also expose its embedded player compound when present.

Save behavior: Browsing works while running. Saving requires the API's editable state plus a stopped/error server state; the backend also verifies the container is inactive and background tasks are finished. Saves preserve tag types and compression, check the opened content revision, keep an exact backup beside the original, and replace the file atomically. A success notice links to the previous version. Conflicts retain local edits and require a confirmed reload; editor exit, sub-tab changes, server-tab changes, route navigation and page unload guard unsaved changes.

States: Loading, empty results, no saved player files, retryable discovery/read errors, invalid NBT, read-only files, validation errors, unsaved edits, save errors, conflicts and successful saves have explicit text. A failed discovery does not claim the player library is empty. Demo data is labeled and read-only. Standalone Java NBT is the supported format; Bedrock data and region containers are outside this editor.

## Evidence and boundaries

Compared PRODUCT.md, DESIGN.md, runtime theme declarations, shared controls, the NBT components and their integrations. The implementation follows the incumbent system; shared design documents and the sidecar remain unchanged.

Inspected `.impeccable/review/nbt/desktop.png` and `mobile.png`, showing populated dark-theme demo data with Inventory selected, desktop tree/details columns and mobile stacking. No horizontal page overflow was observed. These captures cover the read-only state; editing, validation, conflict, failure and unsaved-change behavior are supported by source review and automated tests. Light-theme behavior inherits semantic tokens and was not separately captured.

The finish reviewer returned ship after duplicate-name validation and discovery-error fixes. The mechanical detector returned no findings. Codec, store and authenticated API checks cover tag/compression preservation, exact long values, malformed input limits, backups, stale writes, server state and unsafe paths; UI regressions cover editing and both entry points. Pre-existing DESIGN.md/sidecar freshness drift is not canonized or repaired by this extension.
