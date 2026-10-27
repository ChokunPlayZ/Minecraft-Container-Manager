---
version: 1
slug: "web-src-components-worlds-panel-tsx"
primary_target: "web/src/components/worlds-panel.tsx"
related_targets: ["web/src/api/worlds.ts", "web/src/routes/servers.$id.tsx"]
---

# Server worlds

Mode: Operate. Local extension of the existing server workspace.

## Direction contract

Purpose: Import and export Java world saves, choose the active world, configure generation before terrain exists, and reset selected generated chunks.

World: Inherit DESIGN.md's Operations atlas, semantic light/dark pairs, system typography, flat bordered panels, shared controls, confirmation dialogs, and underlined workspace tabs. No new global tokens, fonts, imagery, or component system.

First viewport: The Worlds tab places Import ZIP and Create world above a world library. At the large breakpoint, a 16rem library sits left of the selected world's details and actions; narrower layouts stack library, details, and chunk management. Forms expand inline above the library. Names and action groups wrap within their panels; wide chunk maps scroll inside their own container.

Interaction: Select a library item to inspect its seed, version or generator, disk usage, modification time, and dimensions. The active state remains distinct from the selected item. Make active requires confirmation and names the next-start behavior. Creation and generation settings use a seed string, generator, and structures switch; generated seeds remain read-only. Import accepts a named ZIP and states the implemented size and archive-content limits. Deleting an inactive world requires confirmation and reports its recovery path.

Chunk management: Choose a dimension and paginated region, then inspect its 32 × 32 grid. Generated, ungenerated, and selected cells have a written legend. Select generated cells directly or replace the selection with an ordered numeric coordinate range inside the current region. Counts, region bounds, and a visible coordinate disclosure identify the selection. Generated cells expose coordinates and selection state; arrow keys move focus and Space selects. Reset requires confirmation naming the dimension, region, selection, consequences, and recovery copies, then reports the reset count and recovery path.

States: Loading skeletons, empty library, unsupported server type, unreadable metadata, no generated regions, invalid or empty coordinate ranges, request errors, operation progress, and success notices are explicit. Read failures offer refresh or Retry. Mutation availability follows the server and API editability state; the running-server explanation preserves inspection. Active worlds cannot be deleted. Mock worlds are read-only.

## Evidence and boundaries

Checked PRODUCT.md, DESIGN.md, the panel, API module, server-tab integration, shared Button/Input/Card implementations, runtime theme declarations, and `.impeccable/review/worlds/desktop.png` plus `mobile.png`. The dark demo captures show the split desktop layout, stacked mobile layout, contained map overflow, and eight selected chunks with matching coordinate details. Seed display preserves the API string, including long signed values.

The finish-review handoff records one precision fix followed by a ship disposition and an empty detector result for the changed component. The captures cover the populated, running, read-only state; form, mutation, confirmation, error, and light-theme behavior are supported here by source inspection rather than new visual verification. No inherited design drift was established within this bounded comparison. Shared system documents were preserved.
