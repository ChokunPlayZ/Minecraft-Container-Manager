# Switch server runtime to itzg/docker-minecraft-server

Status: **Proposed** (2026-08-20)

This document plans replacing MCM's hand-rolled `mcm-server:latest` runtime
image with the community-maintained
[`itzg/docker-minecraft-server`](https://github.com/itzg/docker-minecraft-server)
image. The goal is to offload jar resolution, mod/plugin handling, EULA, and
server launch to a battle-tested image while keeping MCM's web UI, port
allocation, backups, spin-down, and DNS features intact.

## Why switch

The current architecture builds a custom sibling image
(`docker/mcm-server`) whose `entrypoint.sh` hand-rolls what itzg already does,
battle-tested:

- Resolving and downloading server jars (Paper, Fabric, vanilla, Forge,
  NeoForge, Spigot, and many more) with proper version/build logic.
- Accepting EULA and launching the server with correct JVM memory settings.
- Managing mods/plugins (auto-download by URL, `.disabled` toggling, first-run
  copy-in).
- Handling RCON and a large surface of server options.

Replacing it removes a meaningful amount of bespoke code and maintenance burden,
gives broader server-type coverage, and tracks upstream fixes automatically when
the image tag is updated.

## What itzg image is

The itzg image is a Docker image that turns environment variables into a
configured Minecraft server. Key variables for this switch:

| Current MCM concept | itzg variable | Notes |
|---|---|---|
| Server platform (`SERVER_TYPE`) | `TYPE` | `PAPER`, `FABRIC`, `VANILLA`, `FORGE`, `NEOFORGE`, `SPIGOT`, `FOLIA`, etc. |
| Version (`VERSION`) | `VERSION` | Same semantic; `LATEST` accepted. |
| Build / loader | `BUILD_NUMBER` (Paper), `FABRIC_LOADER`, `FORGE_VERSION` | Platform-specific; keep MCM's `BUILD` and map per type. |
| RAM (`RAM_MB`) | `MEMORY` | e.g. `2G` or `2048M`. itzg parses `M`/`G`. |
| EULA | `EULA=TRUE` | Required. |
| Server directory | `/data` | itzg default workdir is `/data`. |
| Mods/plugins dirs | `MODS` / `PLUGINS` + `/mods`, `/plugins` | Drop files into these folders; image copies and enables them. |
| RCON | `ENABLE_RCON`, `RCON_PASSWORD` | Optional, not currently used by MCM. |

Because MCM already binds a host data directory into each server container at
`/data`, the bind-mount model stays the same; we only change the image and the
environment we pass.

## Mapping MCM -> itzg

### 1. Runtime image

- **Remove** `docker/mcm-server/` (its `Dockerfile` and `entrypoint.sh`) as the
  sibling runtime.
- **Use** `itzg/minecraft-server` (multi-arch). Pin a versioned tag (e.g.
  `itzg/minecraft-server:latest` or a dated tag) and make it configurable via
  `MCM_SERVER_IMAGE` so operators control updates.

The panel's own image (`Dockerfile`, `docker/entrypoint.sh`) is unrelated and
stays unchanged.

### 2. Container creation (`internal/docker/docker.go`)

Change `CreateOpts` / `Create` to:

- Set `Image` to the itzg image name instead of `mcm-server:latest`.
- Replace the `SERVER_TYPE`/`RAM_MB` env values with itzg equivalents:
  - `TYPE` from the server type (uppercased, e.g. `PAPER`).
  - `MEMORY` from `RAMMB` (e.g. `2048M`).
  - `VERSION` stays as-is.
  - `BUILD` mapped per type (e.g. `BUILD_NUMBER` for Paper, `FABRIC_LOADER`
    for Fabric, `FORGE_VERSION`/`NEOFORGE_VERSION` for Forge/NeoForge).
  - `EULA=TRUE`.
- Keep the existing bind mount of `DataDir -> /data`, restart policy, resource
  limits, port bindings, and extra ports unchanged.
- Keep `ExposedPorts` including `25565/tcp` and extra ports.

The `imageName` constant, `RuntimeStatus.Image`, and the integration test need
updating to the new image name.

### 3. Jar resolution (`internal/jars`)

MCM resolves/validates jar metadata to power the create-server form and to
store canonical versions. Two options:

- **Keep validation (recommended for now):** Keep `internal/jars` for form
  validation and recording versions, and let itzg do the actual download. This
  preserves the good UX without duplicating download logic. The validation
  endpoints remain wired into the API.
- **Reduce later:** Once on itzg everywhere, the resolver's validation could
  be loosened (itzg accepts `LATEST`). This is a follow-up, not part of the
  first cut.

Decision: keep `internal/jars` validation for the first cut; it is orthogonal
to the container switch.

### 4. Mods / plugins (`internal/servers/mods.go`)

Currently MCM writes artifact `.jar` files directly into
`data/servers/<id>/mods|plugins` and toggles via `.disabled`. With itzg:

- **Reading (ListMods)** maps onto the same directories if we drop files into
  `/mods` and `/plugins`. Keep the existing listing logic; it reads the bound
  data dir directly.
- **Upload** writes into the same directory. No change needed.
- **Enable/disable (`.disabled`)** continues to work with itzg: itzg skips
  `.disabled` files on first run and on rebuilds.

So `mods.go` needs **no structural change**. Verify directory selection still
matches itzg's expectations (`/mods` vs `/plugins` by platform). MCM's
`modDirForType` matches Paper/Spigot -> `plugins`, Fabric/Forge/NeoForge ->
`mods`, which aligns with itzg.

One caveat: itzg manages its own first-run copy-in and will add the files to
the running server on the next start. MCM's upload happens at runtime; the
`.jar` files land in the same mounted dir and are available next container
start. Confirm there is no conflict (e.g. itzg moving files) — none expected.

### 5. Server properties / commands / players

These features edit `server.properties` and use RCON. They operate on the
server's bound data directory and the RCON connection, both independent of the
chosen image.

- `props.go`, `ops.go`, `players.go`, `commands.go` need **no change** for the
  image swap. They rely on the data dir and RCON port, which are unchanged.
- Optionally, RCON could be enabled via itzg env (`ENABLE_RCON=TRUE`,
  `RCON_PASSWORD=...`) if MCM ever grows a native RCON path. Not required now.

### 6. Server state / lifecycle

`Start`/`Stop`/`Restart`/`Status`/`Console` in `servers.go` and the Docker
Manager are image-agnostic (they use container IDs). **No change** beyond the
create-time env/image swap.

### 7. Ports and networking

`ports.Pool`, host port binding, extra ports, and the SRV DNS publisher are
unaffected. itzg still exposes `25565/tcp` by default and honors published
ports via the same Docker HostConfig MCM already builds. **No change.**

### 8. Backups / spin-down / DNS

All operate on the server data directory or the Docker lifecycle and are
image-agnostic. **No change.**

## Files touched

Primary:
- `internal/docker/docker.go` — image name + env mapping to itzg.
- `internal/docker/docker_integration_test.go` — new image name; verify env.
- `internal/docker/port_bindings_test.go` — unchanged, but confirm env assertions.
- `internal/api/docker.go` — error string referencing `mcm-server` image build.
- `README.md` — quickstart no longer builds the custom image; document `MCM_SERVER_IMAGE`.
- `docker-compose.yml` / `.env.example` — add `MCM_SERVER_IMAGE` (optional).

Removed:
- `docker/mcm-server/Dockerfile`
- `docker/mcm-server/entrypoint.sh`

No change (verify only):
- `internal/servers/*` (mods, files, props, commands, players, lifecycle)
- `internal/jars/*`
- `internal/backups/*`, `internal/spindown/*`, `internal/dns/*`
- `internal/ports/*`
- `web/*`

## Env mapping detail (for `docker.go`)

```text
TYPE          = strings.ToUpper(serverType)   // paper -> PAPER
VERSION       = version                        // unchanged
MEMORY        = fmt.Sprintf("%dM", ramMB)      // 2048 -> 2048M
EULA          = "TRUE"

// Type-specific build mapping
paper    -> BUILD_NUMBER = build
fabric   -> FABRIC_LOADER = build
forge    -> FORGE_VERSION = build
neoforge -> NEOFORGE_VERSION = build
spigot   -> (no build var; latest is used)
vanilla  -> (no build var)
```

The `BUILD` field in the DB is kept as-is; only the env var it maps to changes.

## Migration for existing servers

Existing servers already have a `container_id` and a populated data directory
(including a downloaded `server.jar` and `eula.txt`). On switch:

1. After deploying, existing containers are still the old image until
   restarted/recreated. Plan a rolling recreate.
2. `ensureContainer` only creates a container if `container_id` is empty, so
   existing records keep their old container. Provide a one-time reconcile
   (e.g. delete `container_id` for existing servers, or a script/API to
   recreate) so next `Start` builds an itzg-based container.
3. Because the data dir already has a `server.jar` and `eula.txt`, itzg sees an
   existing setup and should skip re-download. Verify itzg's behavior with a
   pre-populated data dir before mass-migrating; this is the main migration
   risk.
4. Mods/plugins already in `mods`/`plugins` persist (same bind mount) and are
   picked up by itzg.

Recommended migration step: on each existing server, stop it, clear
`container_id`, adjust nothing in the data dir, then `Start` to provision a new
itzg-based container. Validate one server end-to-end before rolling to the rest.

## Risks / open questions

- **Pre-populated data dir:** Confirm itzg uses an existing `server.jar` +
  `eula.txt` without re-downloading or erroring. This is the top migration
  risk. Mitigate with a small integration test.
- **Version/build pinning:** MCM stores exact versions/builds; itzg defaults to
  `LATEST`. The env mapping keeps exact values when present, so this is
  preserved.
- **Spigot:** itzg's Spigot build is `latest`-style; MCM's `SpigotBuilds`
  already returns `["latest"]`, so mapping is consistent.
- **Image tag policy:** pin `MCM_SERVER_IMAGE` to a specific tag in production;
  `latest` is convenient for dev but can break on upstream changes.
- **RCON**: if MCM later needs RCON-native commands, enable via itzg env. Not
  in this cut.
- **`docker/mcm-server` removal**: ensure nothing else references the custom
  image (README quickstart, integration test, RuntimeStatus). Grep for
  `mcm-server` and update all references.

## Proposed work order

1. Add `MCM_SERVER_IMAGE` config (default `itzg/minecraft-server`).
2. Update `internal/docker/docker.go`: image name, env mapping (TYPE, MEMORY,
   VERSION, EULA, per-type build var).
3. Update `RuntimeStatus` image reference and `internal/api/docker.go` error
   text.
4. Update integration test to the new image + assert itzg env.
5. Delete `docker/mcm-server/`.
6. Update README quickstart (remove custom image build step; document
   `MCM_SERVER_IMAGE`).
7. Add a reconcile path for existing servers (recreate container), plus an
   integration test confirming a pre-populated data dir boots cleanly.
8. Run `go test ./...` and the integration test; manually verify one server
   create/start/stop/mod/enable cycle through the web UI.

## Not in scope

- Changing the panel image, auth, backups, spin-down, DNS, or frontend.
- Implementing native RCON in MCM (future follow-up, enabled via itzg env if
  desired).
- Loosening `internal/jars` validation to rely purely on itzg (`LATEST`).
