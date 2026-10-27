---
version: 1
slug: "web-src-components-discord-notification-settings-tsx"
primary_target: "web/src/components/discord-notification-settings.tsx"
related_targets: ["web/src/routes/settings.tsx"]
---

# Discord notifications

Mode: Operate. Extend Settings for administrators configuring one Discord channel for all managed servers. Inherit the existing design system and shared form controls. No new visual system, dependencies, or image assets.

## Direction contract

THESIS: Connect a private webhook, choose server and backup events, and verify delivery with a test message.

STORY: Open Notifications, enter or replace the webhook, choose six event types, test, and save. The saved URL stays hidden. Loading, retry, delivery errors, save results, disabled notifications, and pending webhook removal are explicit. Configuration applies to all servers; restarts produce stop/start events.

FIRST VIEWPORT: The existing Settings heading and tab strip lead to one form with an enable switch, labeled webhook field and setup guidance, and event rows. Save and test actions follow the event choices. Narrow layouts wrap descriptions and actions while retaining aligned switches and the existing horizontal tab navigation.

FORM: Existing-surface extension. Use the incumbent Card, Input, Button, Label, and Switch controls and semantic theme tokens. Signature interaction: test the saved or unsaved webhook independently of automatic notifications.

## Verification

The form behavior is covered by six frontend tests. Desktop and mobile preview captures are reviewed against the existing Settings system; demonstration delivery explicitly states that no Discord message was sent. Preserve DESIGN.md and its sidecar. The existing stale-sidecar warning is outside this feature's scope.

Finish review: ship. Both captures are complete; the form matches the incumbent typography, semantic theme tokens, flat panel, shared controls, and responsive layout. No material fixes were identified. Documentation check required no system changes.
