---
version: 1
slug: "web-src-components-log-browser-tsx"
primary_target: "web/src/components/log-browser.tsx"
related_targets: ["web/src/routes/servers.$id.tsx"]
---

# Saved server logs

Mode: Operate. Local extension of the existing server workspace.

## Direction contract

Purpose: Read Minecraft server log files, including compressed rotations and logs inside tar archives, without manually extracting them.

World: Inherit DESIGN.md, shared controls, theme pairs, underlined workspace tabs, and the existing console surface. No new visual system or assets.

First viewport: A Logs tab beside Console opens a single panel. Select a saved log, select its member when it is a tar archive, then search and read numbered output. Mobile stacks the selectors and keeps long output inside the scrollable reader.

Interaction: Open latest.log first when present, then newest saved files. Refresh discovers new rotations and reloads content. Search filters the preview without changing original line numbers; pagination bounds rendered output and wrapping is optional.

States: Loading, no saved logs, empty archive, empty log, no matching lines, damaged archive, request error with refresh recovery, and explicitly truncated preview. Search covers the first 2 MiB. Files provides access to the original download.

Scope: Discover files under logs/ and crash-reports/. Read .log, .txt, gzip-compressed variants, .tar, .tar.gz, and .tgz. Archive members remain virtual and no extracted files are written.
