# Minecraft Container Manager

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Minecraft server administrators managing a self-hosted installation. The user confirmed the administrator scope and delegated workflow priorities.

## Product Purpose

Create, configure, monitor, and maintain Minecraft servers running in Docker containers from one management panel.

## Operating Context

The existing React/Vite application talks to a Go backend. Administrators move between the server fleet, a live console, player moderation, server files, mods and plugins, backups, server configuration, account security, DNS, host ports, and administrator accounts.

## Capabilities and Constraints

Preserve existing APIs, authentication, light/dark/system themes, destructive-action confirmations, installation progress, and all server management capabilities. Panel accounts are admins or members. Members manage only the servers an admin assigns to them; they cannot create, copy, or delete servers, change memory, CPU, or ports, publish DNS, or use panel settings and user management. The user's request covers every page, action, and modal. Do not fabricate operational statistics or add capabilities without implementation.

## Brand Commitments

Keep the MCM name and Minecraft Container Manager product identity. The user requested a complete UI revamp and supplied no binding palette or typography.

## Evidence on Hand

README.md; web/src/routes; web/src/components; web/tests. The application has an existing mock preview for UI verification. Mock content is illustrative and must not be presented as live infrastructure.

## Product Principles

- Make server state and the next useful action immediately visible.
- Keep routine actions easy to find and destructive actions deliberate.
- Apply the same interaction language across the full panel.
- Support narrow screens and keyboard operation.

## Open Decisions

The user delegated design judgment, requested coverage of everything, and chose direct implementation. The user requested direct functional copy with no slogans or filler.
