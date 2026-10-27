---
version: 1
slug: "web-src-components-schedules-panel-tsx"
primary_target: "web/src/components/schedules-panel.tsx"
related_targets: ["web/src/routes/servers.$id.tsx"]
---

# Server schedules

Mode: Operate. Local extension of the existing server workspace.

## Direction contract

Purpose: Schedule Minecraft console commands and start, stop, or restart actions using common presets or five-field cron expressions.

World: Inherit DESIGN.md, shared inputs, selects, switches, buttons, theme pairs, and the underlined workspace tabs. Preserve existing tokens and component patterns.

First viewport: A Schedules tab beside Backups shows task names, actions, frequency, timezone, next run, last result, and controls. Add schedule expands a focused inline form. On mobile, fields stack and task controls wrap within their row.

Interaction: Choose an action and a preset such as midnight, or enter custom cron. The panel's existing TZ environment setting is the initial timezone (UTC fallback); each task saves its own IANA timezone with an optional override. Enable/disable is reversible. Editing uses the same form, preserves server validation errors, and deleting requires the shared confirmation dialog.

States: Loading, empty list, read error with Retry, saving, paused, never run, running, succeeded, failed, and skipped during downtime. Poll execution results every 15 seconds. The panel must stay online; missed tasks do not replay. Console commands require a running server. Restart follows the existing lifecycle behavior, including starting a stopped server.

Scope: Persistent per-server recurring tasks. No new visual tokens, typography, or assets.
