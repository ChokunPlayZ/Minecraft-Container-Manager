---
version: 1
slug: "web-src-components-ui-select-tsx"
primary_target: "web/src/components/ui/select.tsx"
related_targets: ["web/src/components/ui/dialog-backdrop.tsx", "web/src/components/create-server-dialog.tsx", "web/src/components/backups-panel.tsx", "web/src/routes/backups.tsx", "web/src/routes/servers.$id.tsx", "web/src/styles/index.css"]
---

# Shared field dropdown

Mode: Operate. Inherit the Operations atlas system in DESIGN.md: paired mineral/forest tokens, native system typography, compact controls, and bounded overlays. This is a shared control refinement across existing workflows; no new visual world or assets.

## Purpose and behavior

Use the shared Radix Select for visible field dropdowns throughout the application. A labeled trigger displays the current option or placeholder and a muted chevron. Opening it presents a custom listbox with wrapped option labels, a selected checkmark, optional group labels, and scroll buttons for long lists. Keyboard navigation, typeahead, selection, and focus return follow the shared primitive. Existing action menus keep their established Radix menu behavior.

Declarative options and groups preserve their actual values and disabled state. Values are normalized to strings; numeric callers convert them explicitly. Empty options provide placeholders and are not selectable list items. Named fields retain form values and required state; asynchronously loaded choices remain selectable when available.

## Geometry and states

The default trigger uses the select component mapping in DESIGN.md: full width, control height, medium corners, card/foreground pair, and input stroke. Its selected text truncates within the available width. Hover strengthens the stroke; keyboard focus and open state share a primary border and tinted focus ring with (150ms) feedback. A disabled trigger uses muted fill, reduced opacity, and cannot open.

The listbox uses the select-menu token mapping, border, and medium menu shadow. Items are at least (36px) high, use the select-item-highlighted mapping when highlighted, and reserve trailing space for the selected checkmark. Disabled items reduce opacity and cannot be selected. Labels wrap rather than extending the menu beyond the viewport. Light and dark themes preserve the same geometry.

## Portal, focus, and responsive behavior

The listbox is portaled at layer (150), above dialog panels. It starts (4px) from the trigger, retains (12px) collision padding, is at least as wide as the trigger, and is bounded by the viewport width minus (24px). Its height is limited to (320px) or available space, whichever is smaller.

DialogBackdrop permits focus inside the portaled listbox and leaves its keys to Select. Escape dismisses the listbox first, returns focus to the trigger, and leaves the outer workflow open. The dialog resumes its existing focus and dismissal behavior after the listbox closes.

## Relevant routes and evidence

The shared control covers create-server choices on `/dashboard`; server configuration, properties, installation catalogs, players, logs, proxy configuration, and backups on `/servers/$id`; and server/storage selection on `/backups`. The CurseForge API key prompt on the server workspace and headerless administrator cards on `/users` retain top padding of (20px), growing to (24px) at the small breakpoint. The shell profile identity and mobile avatar lead directly to `/settings?tab=account`, with Log out available in expanded mobile navigation.

Finish: ship, with no material findings in `.impeccable/review/custom-dropdowns/finish-review.md`. Current captures and `browser-checks.json` in that directory cover grouped choices, ordinary fields, mobile bounds, modal focus/Escape behavior, profile navigation, and the two padding fixes. The existing demo preview's administrator fixture is illustrative.
