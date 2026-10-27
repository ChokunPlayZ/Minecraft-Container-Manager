---
name: Minecraft Container Manager
description: Shared interface tokens and patterns for the MCM administration panel.
colors:
  background: "#f5f7f7"
  foreground: "#1c2b2e"
  card: "#ffffff"
  card-foreground: "#1c2b2e"
  popover: "#ffffff"
  popover-foreground: "#1c2b2e"
  primary: "#236d61"
  primary-foreground: "#ffffff"
  secondary: "#edf2f1"
  secondary-foreground: "#344846"
  muted: "#edf1f1"
  muted-foreground: "#607172"
  accent: "#e7efed"
  accent-foreground: "#1e5d53"
  destructive: "#b63842"
  destructive-foreground: "#ffffff"
  border: "#dce4e3"
  input: "#cdd8d6"
  ring: "#347e71"
  sidebar: "#edf2f0"
  console: "#152124"
  dark-background: "#11191b"
  dark-foreground: "#e7eeec"
  dark-card: "#182225"
  dark-card-foreground: "#e7eeec"
  dark-popover: "#202d30"
  dark-popover-foreground: "#e7eeec"
  dark-primary: "#93cbb9"
  dark-primary-foreground: "#142c25"
  dark-secondary: "#243235"
  dark-secondary-foreground: "#d2dfdb"
  dark-muted: "#233034"
  dark-muted-foreground: "#9bafae"
  dark-accent: "#293d38"
  dark-accent-foreground: "#c3e4d8"
  dark-destructive: "#ed9197"
  dark-destructive-foreground: "#3b1418"
  dark-border: "#2c3a3d"
  dark-input: "#3c4c4f"
  dark-ring: "#93cbb9"
  dark-sidebar: "#152022"
  dark-console: "#0d1517"
  console-foreground: "#dde8e4"
  running: "oklch(43.2% 0.095 166.913)"
  running-dark: "oklch(84.5% 0.143 164.978)"
  running-tint-source: "oklch(69.6% 0.17 162.48)"
  pending: "oklch(47.3% 0.137 46.201)"
  pending-dark: "oklch(87.9% 0.169 91.605)"
  pending-tint-source: "oklch(76.9% 0.188 70.08)"
  installing: "oklch(44.3% 0.11 240.79)"
  installing-dark: "oklch(82.8% 0.111 230.318)"
  installing-tint-source: "oklch(68.5% 0.169 237.323)"
typography:
  page-title:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "1.75rem"
    fontWeight: 650
    lineHeight: 1.25
    letterSpacing: "-0.03em"
  large-title:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "1.875rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.025em"
  dialog-title:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.555556
  section-title:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  body:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  description:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.6
  control:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "0.875rem"
    fontWeight: 500
    lineHeight: 1.428571
  label:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.333333
  metric:
    fontFamily: "-apple-system, BlinkMacSystemFont, \"Segoe UI\", sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.03em"
  console:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, \"Liberation Mono\", \"Courier New\", monospace"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 2
rounded:
  sm: "6px"
  md: "8px"
  lg: "12px"
  dialog: "16px"
  full: "calc(infinity * 1px)"
spacing:
  "1": "4px"
  "1.5": "6px"
  "2": "8px"
  "2.5": "10px"
  "3": "12px"
  "3.5": "14px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
  "7": "28px"
  "8": "32px"
  "10": "40px"
  "12": "48px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-destructive:
    backgroundColor: "{colors.destructive}"
    textColor: "{colors.destructive-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-outline:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-secondary:
    backgroundColor: "{colors.secondary}"
    textColor: "{colors.secondary-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.muted-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-link:
    backgroundColor: "transparent"
    textColor: "{colors.primary}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  input:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "4px 12px"
    height: "40px"
    width: "100%"
  badge:
    backgroundColor: "color-mix(in oklab, #236d61 10%, transparent)"
    textColor: "{colors.primary}"
    typography: "{typography.label}"
    rounded: "{rounded.full}"
    padding: "4px 10px"
  card:
    backgroundColor: "{colors.card}"
    textColor: "{colors.card-foreground}"
    rounded: "{rounded.lg}"
  navigation-active:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.lg}"
    padding: "12px"
  button-primary-dark:
    backgroundColor: "{colors.dark-primary}"
    textColor: "{colors.dark-primary-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-destructive-dark:
    backgroundColor: "{colors.dark-destructive}"
    textColor: "{colors.dark-destructive-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-outline-dark:
    backgroundColor: "{colors.dark-card}"
    textColor: "{colors.dark-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-secondary-dark:
    backgroundColor: "{colors.dark-secondary}"
    textColor: "{colors.dark-secondary-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-ghost-dark:
    backgroundColor: "transparent"
    textColor: "{colors.dark-muted-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  button-link-dark:
    backgroundColor: "transparent"
    textColor: "{colors.dark-primary}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "40px"
  input-dark:
    backgroundColor: "{colors.dark-card}"
    textColor: "{colors.dark-foreground}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "4px 12px"
    height: "40px"
    width: "100%"
  badge-dark:
    backgroundColor: "color-mix(in oklab, #93cbb9 10%, transparent)"
    textColor: "{colors.dark-primary}"
    typography: "{typography.label}"
    rounded: "{rounded.full}"
    padding: "4px 10px"
  card-dark:
    backgroundColor: "{colors.dark-card}"
    textColor: "{colors.dark-card-foreground}"
    rounded: "{rounded.lg}"
  navigation-active-dark:
    backgroundColor: "{colors.dark-card}"
    textColor: "{colors.dark-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.lg}"
    padding: "12px"
---

# Design System: Minecraft Container Manager

## Overview

**Creative North Star: "Operations atlas"**

The operations atlas gives MCM a stable frame: mineral surfaces, a forest-teal action color, clear rules, and aligned operational data. Light and dark themes share the same hierarchy and component geometry. Typography uses the native system UI stack so labels, settings, and long-running workspaces remain familiar.

Surfaces are flat and restrained. Borders group related work; spacing separates tasks; selected controls and navigation identify the current context. The interface uses text and icons alongside state color. Copy names the operation, its input, or its result, without slogans, filler, or unsupported safety claims.

**Key Characteristics:**

- Mineral white and cool graphite surfaces with paired theme tokens.
- A stable navigation frame and fluid, aligned workspaces.
- Compact controls, quiet panels, and readable operational data.
- Direct labels, visible state, keyboard focus, and bounded dialogs.

This record describes the final implementation in `web/src/styles/index.css`, the shared UI primitives, and the application shell. Theme-prefixed tokens in the frontmatter record the `.dark` overrides; the runtime keeps the same semantic CSS property names. The surface brief owns the dashboard composition and its mode.

## Colors

Cool, low-chroma surfaces carry the workspace; forest teal marks primary actions and selection, while small semantic hues distinguish server states.

### Primary

- **Forest teal / pale forest:** `primary` and `dark-primary` identify primary actions, focus-associated emphasis, and selected controls. Always pair them with their respective `primary-foreground` values.
- **Quiet forest:** `accent` and `accent-foreground` supply hover and filter-selection surfaces. The ring tokens define visible keyboard focus.
- Solid selected controls and active DNS badges use the primary pair. The final review measured this pair at (6.12:1) in light and (8.11:1) in dark. The active DNS badges keep that pair on hover.

### Secondary

- **Error rose:** the destructive pair marks destructive buttons and errors; low-opacity destructive surfaces support alerts.
- **Running green:** emerald foregrounds use `running` in light and `running-dark` in dark over a (10%) tint from `running-tint-source`.
- **Pending amber:** starting and stopping states use the pending foregrounds over a (10%) tint from `pending-tint-source`.
- **Installation sky:** installing and building states use the installing foregrounds over a (10%) tint from `installing-tint-source`.
- The state colors retain Tailwind's installed OKLCH values in the frontmatter. They are functional state colors, not additional brand accents.

### Neutral

- **Mineral ground / deep graphite:** background tokens sit behind cards and the workspace.
- **Panel white / cool graphite:** card and card-foreground tokens define the normal working surface.
- **Popover layer:** popover tokens match light cards and lift to a lighter graphite in dark mode.
- **Secondary and muted surfaces:** secondary supports subdued controls; muted supports table headings, read-only areas, and hover feedback. Their foreground tokens keep secondary text distinct from disabled controls.
- **Rules and field edges:** border is the quiet divider; input is the stronger field stroke. Sidebar is a separate rail surface.
- **Console ground:** console remains dark in both themes; console-foreground supplies its pale output text.

**The Paired Theme Rule.** Use semantic foreground and background pairs together. The dark theme changes their values, not their role.

**The State Plus Text Rule.** A status color accompanies a written state; pending server states also carry a spinner.

## Typography

**Interface Font:** the native system stack recorded in the frontmatter. No web font is loaded.

**Data Font:** the monospace stack recorded for console text, matching the installed utility framework.

The hierarchy is compact, with sentence-case labels, modest heading weights, and tighter tracking on titles. MCM's product identity uses the same UI family; there is no separate decorative display face.

### Hierarchy

- **Page title:** the page-title token; at narrow widths it becomes (1.5rem). The desktop value is (1.75rem), weight (650), with tight tracking.
- **Large title:** server and authentication titles use the large-title role (1.875rem, weight 600). Server titles begin at (1.5rem) and grow at the small breakpoint. The authentication identity panel has a local product-name treatment (2.25rem, weight 500, line-height 1.2); it is not a reusable page heading.
- **Dialog title:** commonly (1.125rem, weight 600); some catalog workflow headings use (1.25rem) or bold emphasis.
- **Section title:** the section-title token provides a compact internal heading.
- **Body and descriptions:** the body token is the base (14px) text; page descriptions use the description token and a maximum measure of (70ch). Empty-state explanations use a maximum of (45ch).
- **Controls and labels:** controls use (14px, weight 500); small buttons and badges use (12px). Supporting metadata sometimes uses (11px), with muted foreground rather than low-opacity text as the default treatment.
- **Metrics:** values use tabular numerals and the metric token, reducing to (1.25rem) on narrow screens.
- **Console:** output uses (12px) mono text with a (24px) line-height. Command inputs and file editors use the same mono family.

**The Comparable Data Rule.** Use tabular numerals for resource readings and counts, and monospace for commands, paths, versions, and machine identifiers.

## Layout

The shared shell fixes a rail at the left on wide screens and centers fluid content in the remaining space. The rail is (224px); the header is (64px) high. The header, main content, and footer share a maximum width of (1480px). Main horizontal padding is (20px), increasing to (32px) at the small breakpoint and (40px) at the large breakpoint. Main vertical padding grows from (32px) to (40px) at the large breakpoint.

The spacing foundation is (4px), with frequently reused half steps (6px, 10px, 14px). Form groups normally use (16px), panel sections (20px or 24px), and larger task separations (24px or 28px). Page heading groups have a (20px) gap and a (28px) bottom margin, reducing the margin to (24px) on narrow screens.

Responsive rules follow the shipped utility breakpoints:

- **Small (640px / 40rem):** panel padding grows from (20px) to (24px); multi-column form groups can open; search fields can become bounded rather than full width. Authentication form width remains capped at (384px).
- **Medium (768px / 48rem):** fleet rows use aligned identity, state, memory, port, and action columns. Below this width, rows use identity/state columns with actions on a separate line; secondary columns and the table-like heading are hidden. Four resource cells become a two-by-two grid with matching dividers. Workspace tabs continue to scroll horizontally.
- **Large (1024px / 64rem):** the fixed rail appears and content is offset by its width. Below it, a header menu button reveals labeled navigation in flow; selecting a destination closes the menu. The authentication layout becomes a split panel at this breakpoint.
- **Extra large (1280px / 80rem):** dense settings and the file editor can use adjacent columns; below it, their sections stack.

File identity precedes size and actions. Below the small breakpoint, populated file rows place the name on its own line and wrap a second action row. The dependency dialog stacks its action group on narrow screens, allows labels to wrap, and reverses the action stack so the primary operation appears first. At the small breakpoint the group returns to wrapping horizontal actions. Preserve these rules for long names.

The common prompt/confirmation panel is capped at (448px). Other workflows use (512px), (576px), or (672px) caps according to content. Dialogs fill available width inside their viewport gutter. Dense settings use narrower local content limits where implemented rather than stretching every form to the shell width.

## Elevation & Depth

Depth comes primarily from adjacent surface tones, one-pixel rules, and the darker console inset. Ordinary cards, fleet rows, metrics, and the navigation frame remain flat. A selected rail item uses a one-pixel ring. Overlays use a structural shadow; the backdrop dims the workspace without blur.

### Shadow Vocabulary

- **Dialog:** a broad structural shadow (`0 24px 80px rgb(0 0 0 / 25%)`) shared by all dialog panels.
- **Menu:** the installed medium shadow (`0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)`). Submenus use the installed large shadow.
- **Switch thumb:** the installed small shadow is confined to the white moving thumb.
- **Focus:** an external ring, rather than a raised surface, marks keyboard position. The base outline is (2px) with (3px) offset. Buttons use a (2px) ring with (2px) surface-colored offset; fields use a (20%) ring tint and a primary border.

**The Flat Workspace Rule.** Ordinary panels and rows have no ambient shadow. Reserve elevation for overlays, menus, focus, and the switch thumb.

## Shapes

Controls have gently curved corners using the medium radius; ordinary panels use the large radius. Small menu items use the small radius. Dialog panels override their local radius to the shared dialog value. Badges, state dots, avatar initials, progress tracks, and switches use fully rounded ends. Thin borders and dividers establish alignment without decorative frames.

Icons are SVG line icons, normally (16px) in controls, (18px) in rail navigation, and (32px) in empty states. The MCM mark is a simple outlined box. No shipping raster asset was added; existing remote player avatars remain user data and keep their existing behavior.

## Components

### Buttons

Compact, explicit actions with one shared geometry. The six shipped variants are primary, destructive, outline, secondary, ghost, and link. Primary and destructive variants use their paired semantic colors; outline uses a card fill and input stroke; secondary uses the secondary pair; ghost begins with muted text and gains an accent surface on hover; link uses primary text and an underline on hover.

Default buttons are (40px) high with (16px) horizontal padding. Small buttons are (36px) high with (12px) horizontal padding and (12px) text; large buttons are (44px) high with (24px) horizontal padding. Icon buttons are (40px) square. SVG children normally measure (16px). Buttons shift down (1px) while active, use (150ms) color/border/shadow feedback, and reduce disabled opacity to (45%) while blocking pointer interaction. Destructive intent belongs in the label and confirmation as well as its color.

### Inputs / Fields

Inputs and native selects are (40px) high, full width, with a card fill, input stroke, medium corners, and (12px) horizontal padding. Textareas begin at (100px) high. Focus changes the border to primary and adds a tinted ring. Disabled fields use muted fill and (60%) opacity. Native select affordances remain functional with a (16px) chevron. Labels are explicitly associated with their fields. Below (768px), non-checkbox/radio inputs, selects, and textareas use (16px) text to keep mobile entry readable.

The switch is (44px by 24px) with a (20px) white thumb. It uses primary when checked and input when unchecked, moves the thumb (20px), exposes its checked state, and supports Space and Enter. Its feedback lasts (200ms).

### Chips / State Badges

Badges use small text, fully rounded ends, and (4px by 10px) padding. The default badge uses a (10%) primary tint with primary text. Secondary, destructive, and outline variants retain the same geometry. Server status badges use the semantic foreground/tint combinations described above, a written state, and either a dot or a pending spinner. Filters expose selection with pressed or selected state. Solid selected workflow labels and active DNS badges use primary with primary-foreground.

### Cards / Containers

Cards use a one-pixel border, card fill, card foreground, large corners, and no ambient shadow. Headers use (20px) padding, growing to (24px) at the small breakpoint, with (8px) between title and description. Content and footers align to those edges; footers wrap actions. Inset settings sections use muted or lightly tinted backgrounds rather than a separate visual system.

### Navigation

Rail and mobile navigation links combine an icon and a (14px) label for Servers, Backups, Settings, and Users, without supporting descriptions. The active item uses a card surface, foreground text, a one-pixel border ring, a primary icon, and a trailing chevron. Inactive links use muted text and an accent hover surface. The current page is exposed to assistive technology. A skip link becomes visible on focus.

Workspace tabs use a transparent surface and a two-pixel bottom rule for selection. They are separated by (6px), have (13px by 15px) padding on wide screens, and use (12px) padding on narrow screens. They retain horizontal scrolling instead of compressing labels.

### Dialogs and Menus

All dialogs share the DialogBackdrop behavior: the body stops scrolling, focus moves inside the top dialog, Tab remains within it, Escape invokes its dismissal callback, and closing returns focus to the trigger when it still exists. Nested menus keep their own keyboard behavior. Dialog panels receive a dialog role, modal state, and an accessible heading association.

The backdrop uses a dark translucent fill (`rgb(9 20 21 / 62%)`) with (24px) gutters; below (768px), gutters become (12px). Panels scroll internally and remain capped at the viewport height minus both gutters. A reveal moves the panel upward from (6px) over (160ms, ease-out). Reduced motion removes this reveal and reduces other transition/animation durations to near zero. Outside-pointer dismissal is implemented by individual workflow callers; do not assume every dialog has the same outside-click policy.

Menus use a popover surface, one-pixel border, large corners, (6px) padding, a minimum width of (160px), and elevated positioning. Menu items have small corners, (8px) padding, and accent fill/text on keyboard focus.

### Operational Collections and Feedback

Fleet search matches name, type, version, and host port immediately. State filters expose their selected state, a result count announces changes, and the empty result provides Reset filters. Loading uses skeletons; errors state the failure and offer the implemented recovery action. Mock previews carry the Demo data label.

Resource cells share dividers and tabular values. Console output uses the console token pair, (16px) padding, a (384px) normal height, a (320px) minimum height, and a (420px) height from the small breakpoint. Progress bars expose progress semantics, use labeled percentages when determinate, and animate value changes over (300ms, ease-out); indeterminate states use a pulse. Keep real state and progress supplied by the existing implementation.

## Do's and Don'ts

### Do:

- **Do** use paired theme tokens for text, controls, and selected states.
- **Do** keep names, file identity, and the operation label readable when actions wrap.
- **Do** preserve status text, explicit field labels, keyboard focus, and accessible names for icon actions.
- **Do** use direct operational copy such as Create server, Reset filters, Try again, and Cancel.
- **Do** group related settings and keep long content scrollable within the viewport.

### Don't:

- **Don't** add slogans, decorative introductory labels, filler, or unsupported safety claims.
- **Don't** encode a state or destructive action through color alone.
- **Don't** let a mobile action cluster consume the width needed to identify its target.
- **Don't** substitute low-contrast white-on-amber or white-on-emerald labels for the shared selected-state pair.
- **Don't** add decorative raster imagery or ambient panel shadows to this system.

Not canonized: no unresolved defect was identified by the final ship verdict. The earlier clipped dependency actions, unreadable mobile file identity, and low-contrast solid labels were corrected and are not retained as system patterns. Sidecar tonal ramps are synthesized swatch previews, not additional runtime colors.
