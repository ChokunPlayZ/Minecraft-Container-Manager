# MCM - Minecraft Container Manager

MCM is a modern, high-performance, self-hosted Minecraft server management panel. Built with a lightweight **Go** backend and a responsive **React / Vite** frontend, MCM orchestrates Minecraft servers as sibling **Docker or Podman** containers on the host using lightweight, security-hardened **Eclipse Temurin Alpine OpenJDK/JRE** images.

Each Minecraft server container runs with MCM's custom native lifecycle entrypoint script (`DefaultEntryScript`), providing interactive console FIFO pipes, graceful shutdown traps (`SIGTERM`/`SIGINT`), headless Java configuration, and automated installer execution for Forge and NeoForge. MCM supports both direct host-port publication (for zero-proxy overhead, raw performance, and UDP/TCP support) and private container networking (`mcm-network`) with optional host ports for seamless multi-server proxy setups (e.g., Velocity or BungeeCord).

## Install, manage, and update (Linux)

Use the included `mcmctl` command on a Linux host with systemd. Run it as your
normal user. Its guided installer walks through configuration, builds the
complete panel with Podman or Docker, installs a user service, and starts the app.
Go, Node.js, and pnpm are supplied by the build image.

On Debian/Ubuntu, install the host dependencies:

```sh
sudo apt update
sudo apt install -y podman uidmap slirp4netns fuse-overlayfs git curl util-linux
```

On Fedora:

```sh
sudo dnf install -y podman git curl util-linux
```

Then install MCM:

```sh
git clone https://github.com/ChokunPlayZ/Minecraft-Container-Manager.git
cd Minecraft-Container-Manager
./mcmctl install
```

The terminal wizard walks through the engine and API socket, storage, panel
address, Minecraft ports, timezone, passkeys, HTTPS, default server limits,
optional S3/CurseForge credentials, and login protection. Press Enter to keep
each default. Review all settings and revisit any step before saving; type
`:cancel` at any prompt to exit without saving. Existing settings and session
secrets are kept, and credential input is hidden in the terminal.

Flags such as `--runtime podman`, `--data-dir /absolute/path`, and `--port 9090`
set the wizard's defaults. Use `--non-interactive` for unattended installation;
without a terminal the installer also uses existing settings, flags, and defaults.
Use `--interactive` to explicitly request the wizard with redirected input.

Open `https://YOUR_SERVER_IP:8080` (or your configured address) and complete
onboarding. MCM generates a self-signed certificate by default; trust its public certificate or accept the
browser warning on first use. Open port 8080 and
any published Minecraft ports in the host firewall if you connect remotely.
For a host already running Docker, use `./mcmctl install --runtime docker` with
an account that can access its Docker daemon.

Already have this checkout or a failed `mcm.service` from the earlier setup?
Run `./mcmctl install --runtime podman` from the updated project directory.
It creates the previously missing `~/.config/mcm/mcm.env` and replaces the user
service with the managed installation. Existing settings and session secrets
are preserved. If your server data is elsewhere, include its actual path:

```sh
./mcmctl install --runtime podman --data-dir /absolute/path/to/existing/data
```

Keep using the same engine for existing Minecraft containers. If moving from a
Compose panel, stop that panel first and pass its existing host data directory
with `--data-dir`; run only one panel against that directory.

### Everyday commands

From the checkout, use:

| Task | Command |
| --- | --- |
| Check status | `./mcmctl status` |
| Follow logs | `./mcmctl logs` |
| Start / stop / restart | `./mcmctl start` / `./mcmctl stop` / `./mcmctl restart` |
| Edit settings step by step | `./mcmctl config` (offers a restart after saving) |
| Open settings in a text editor | `./mcmctl config --editor`, then `./mcmctl restart` |
| Download and install updates | `./mcmctl update` |
| Build local changes | `./mcmctl update --local` |
| Back up panel settings and database | `./mcmctl backup` |
| Restore the last panel backup and version | `./mcmctl rollback` |
| Enable / disable automatic startup | `./mcmctl enable` / `./mcmctl disable` |
| Uninstall while keeping data | `./mcmctl uninstall` |

The command is also installed at `~/.local/bin/mcmctl` for use outside the
checkout. Updates build before stopping the panel, save the installed version
and panel database, and restore them if the new version cannot become ready.
Minecraft containers continue running during a panel update.

Settings live in `~/.config/mcm/mcm.env`; the default database and server files
live in `~/.local/share/mcm`. Guided changes save a copy of the previous settings
under `~/.config/mcm/backups`. The installer enables the user service and tries
to enable lingering so it starts at boot and continues after logout. If your
host requires administrator authorization, it prints the one command to finish
that step. Keep the source checkout: `update` uses its current Git branch.

See [installation and operations](docs/operations.md) for prerequisites,
configuration, updates, backups, rollback, and troubleshooting. The Compose
instructions below are alternatives for people who want to run the panel in
a container.

---

## Key Features

### 🎮 Comprehensive Server Platform Coverage (23 Flavors)
- **Extensive Flavor Support**: Out-of-the-box support for 23 server platforms:
  - **Bukkit / Paper Ecosystem**: Paper, Purpur, Folia, Pufferfish, Leaf, Spigot, and Vanilla.
  - **Mod Loaders**: Fabric, Quilt, Forge, NeoForge, and SpongeVanilla/SpongeForge.
  - **Hybrid Mod + Plugin Engines**: Youer, Mohist (Legacy), Ketting, and Crucible.
  - **High-Performance Proxies**: Velocity, Waterfall, and BungeeCord.
  - **Limbo & Queue Servers**: Limbo and NanoLimbo.
  - **Bridges & Custom**: GeyserMC Standalone and Custom Server JARs.
- **Dynamic Java Runtime Selector**: Automatically select and run Java 8, 11, 17, or 21 (`eclipse-temurin:<version>-jre-alpine`) based on Minecraft version requirements, with built-in recommendations and automatic runtime image pulling.
- **In-Place Version Switching**: Upgrade or switch server types and builds with built-in version metadata and confirmation safeguards.

### 📦 Modpack Management Engine
- **Modrinth & CurseForge Integration**: Browse, search, and 1-click install popular modpacks directly from Modrinth and CurseForge catalogs.
- **Deep Modpack Inspection**: Automatically detects Minecraft version, mod loader, loader build, and recommended Java runtime before installation.
- **Asynchronous Task Progress Tracking**: Live task progress monitoring (SSE stream and polling) for modpack downloads, extraction, and archiving.
- **Client-Only Mod Sanitization**: Automatically scans and strips client-only mods (e.g. shaders, GUI optimizations, client audio mods) from server-side installations.
- **Stale Binary Cleanup**: Automatically purges obsolete loaders, launch scripts, and stale jar files when switching modpack engines or loaders.
- **User-Required File Notifications**: Flags mods or assets that require manual download due to licensing constraints and guides the administrator through the setup.
- **Version Locking & Updates**: Lock modpacks to specific versions to prevent configuration drift, with support for seamless upgrades.

### 🛠️ SMP Version Update Helper & Mod Ecosystem Hub
- **Version Update Helper**: Dedicated assistant to guide administrators through upgrading Minecraft versions, server loaders, and modpacks with compatibility checks and pre-update backup prompts.
- **Automated Mod Updates**: Scans installed mods against Modrinth and CurseForge, checks for available updates with request coalescing, and offers 1-click batch updating.
- **Mod Dependency Resolution**: Discovers missing required and optional dependencies before installation and installs them in one click via `ModDependenciesDialog`.
- **Mod Jar Picker**: Select exact builds or releases when upgrading or installing specific mods.
- **Multi-Provider Catalog**: Browse and search tens of thousands of mods and plugins across 4 major repositories:
  - **Modrinth**: Mods, plugins, and datapacks with loader detection, server-side-only filtering (`onlyServerSide`), and infinite scroll.
  - **PaperMC Hangar**: Curated Paper, Purpur, and Folia plugins.
  - **SpigotMC (via Spiget)**: Over 100,000 Bukkit and Spigot plugins.
  - **CurseForge**: Expansive catalog of mods and server addons.
- **Mod Management**: Enable/disable mods with `.disabled` toggling, view installed metadata, and delete mods.

### 📊 Real-Time Metrics & Container Monitoring
- **Dashboard Host Resources**: Live CPU, memory, and network usage summed across active managed server containers, with CPU and memory compared against the Docker or Podman host's capacity. The resource viewer refreshes every five seconds and shows available, used, and total space on the filesystem containing `MCM_DATA_DIR`. Available disk space excludes filesystem reservations and remains visible when the runtime is offline. Container usage excludes other host processes; it is not a sum of configured limits.
- **Live Container Resource Metrics**: Real-time stats polled directly from the Docker daemon:
  - **CPU Utilization %**: Normalized across available host cores.
  - **Memory Usage & Limits**: Real-time memory consumption, limits, and usage percentage.
  - **Network I/O**: Live incoming (`rx`) and outgoing (`tx`) network throughput.
  - **Block / Disk I/O**: Real-time disk read and write bytes.
- **Server Uptime Tracking**: Tracks server start timestamps (`started_at`) and displays live, human-readable uptime (e.g., `3d 4h 12m`) on dashboard cards and server headers.
- **ServerStatsGrid**: Clean, visual metrics dashboard available directly on the server overview and management pages.
- **Performance History**: The server's Performance tab charts CPU, memory, connected players, TPS, and median tick time. Background samples are stored in SQLite every 30 seconds and retained for 7 days, with 1-hour, 6-hour, 24-hour, and 7-day views. CPU and memory are always collected quietly. Player and tick metrics are **off by default**, including for existing servers. Choose **RCON polling** or **Console commands** per server to opt in. Console mode prints `list` and `spark tps` output every 30 seconds; RCON mode reads replies directly and never falls back to the console. RCON must be enabled with a password and reachable through a shared container network or an existing RCON TCP port binding; MCM does not expose ports or enable RCON automatically. Collection errors appear beside the selector. Longer windows average resource/tick readings and show peak player counts. Missing readings appear as gaps, and turning collection off preserves earlier history.
- **Spark Reports**: Detects enabled spark plugins/mods from their metadata or filenames, and bundled spark on Paper 1.21.1+. Capture a 60-second profile and open its returned link, or open a profiler that is already running. Manual report capture remains available when automatic collection is off, writes to the console, and uploads the report to spark's viewer when requested. Capture progress and the latest result survive navigation. Automatic TPS and median tick duration require spark and an enabled collection mode.

### 🌐 Proxy Networks & Multi-Server Architecture
- **Dedicated Container Network (`mcm-network`)**: Sibling containers join an isolated Docker bridge network where each container is addressable by its internal hostname (`mcm-<server-id>`).
- **Proxy Config Editor**: Built-in visual editor and configuration generator for Velocity (`velocity.toml`) and BungeeCord / Waterfall (`config.yml`).
- **Optional Host Ports**: Run backend servers entirely private on the internal Docker network with zero exposed host ports, routing all player traffic securely through your proxy.

### 🔄 Container Lifecycle, Rebuild Warnings & Server Cloning
- **Lifecycle Controls**: Start, Graceful Stop (`SIGTERM` with `stop`/`end`/`shutdown` commands piped to console), Restart, and Force-Kill (`SIGKILL`).
- **Dashboard Controls**: Start or stop a server from its fleet row. The action menu includes Restart, server-specific Backups, Copy server, and Force kill with confirmation.
- **Rebuild Warning System**: Automatically detects when server configuration changes (Java runtime, memory limits, CPU quotas, port bindings) require container recreation and displays an actionable rebuild banner.
- **1-Click Container Rebuild**: Recreate sibling containers on demand without data loss to apply updated configuration, memory limits, or base images. If online, the server is gracefully shut down first to cleanly save world data, the container is recreated, and it is automatically booted back up.
- **Instant Server Duplication / Cloning**: 1-click server cloning (`POST /api/servers/:id/copy`) that duplicates server data, configurations, mods, and plugins while automatically allocating fresh host ports.

### ⚡ Live Console & Command Line
- **Real-Time Streaming**: Live server logs streamed directly to the browser using Server-Sent Events (SSE).
- **Last-Event-ID Resumption**: Seamlessly reconnects to log streams after network hiccups without duplicate messages or log gaps.
- **Interactive Command Input**: Send console commands directly to the running server via native FIFO stdin pipe (`/tmp/console.in`) with RCON fallback.
- **Smart Auto-Scroll**: Sticky bottom-scrolling that pauses automatically when scrolling up to inspect past logs.
- **Saved Log Browser**: Open the server's Logs tab to read files from `logs/` and `crash-reports/`, including `.log.gz` rotations and logs inside `.tar`, `.tar.gz`, and `.tgz` archives, without extracting them. Search, line numbers, wrapping, and pagination help inspect saved output. Previews show the first 2 MiB; search applies to that preview.

### World Management

- Open a Java game server’s **Worlds** tab to browse its saved worlds, dimensions, saved seed, Minecraft version and disk usage. Proxies and standalone Geyser servers do not store Java worlds.
- Create worlds with numeric or text seeds, default/superflat/large-biomes/amplified generation and optional structures. Leave the seed blank for a random seed. Make a world active, then start the server to generate it. Generation settings stay editable until `level.dat` exists; existing terrain retains its saved seed.
- **Import ZIP** accepts a Java world with `level.dat`, either at the archive root or inside a folder. Matching Bukkit `<world>_nether` and `<world>_the_end` folders are imported and exported with the main world. Imports use a new folder name and never overwrite existing folders. Limits: 2 GiB compressed, 8 GiB unpacked, 100,000 entries. Unsafe paths, symbolic links, duplicate files and invalid metadata are rejected before the imported world becomes available.
- **Make active** updates `level-name` and generation settings in `server.properties`, preserving unrelated properties. Switching takes effect on the next start. World imports, exports, switches, seed edits, deletions and chunk resets require a stopped server and a confirmed inactive container; startup waits for world operations on that server.
- **Manage chunks** inspects paged region files by dimension, including modded dimension folders. The 32 × 32 map selects individual generated chunks or all generated chunks in the selected region. Coordinate-range inputs support precise selection on touch screens, with a visible list of all selected chunk coordinates. Arrow keys move between cells; Space selects. Resetting removes the selected terrain, entity and point-of-interest entries so Minecraft regenerates the chunks when visited. Other chunk payloads are unchanged. Resetting makes sectors reusable and does not immediately shrink region files.
- Chunk resets first copy every affected region and external chunk payload to `.mcm-world-backups/<id>/`, with a `reset.json` manifest. To recover, stop the server and use **Files** to copy the backed-up paths over their original paths. World deletion moves the inactive world and its dimension folders to `.mcm-world-trash/<id>/`; restore the folders to the server root using **Files**. These recovery folders remain until manually removed and count toward disk usage. The active world cannot be deleted.

### NBT Browser & Editor

- Open **Players → Player data** to find saved player files by cached name, UUID or world, including offline players. Quick fields edit health, food, saturation, XP, game mode, dimension and position when those tags exist; inventory, ender chest and abilities open in the full tag browser. Names come from the server's local `usercache.json`, whitelist and operator list.
- In **Files**, open `.dat`, `.dat_old`, `.nbt`, `.schematic`, `.schem` or `.litematic` files directly. **Open as NBT** works with other filenames too. Browse and search every Java Edition NBT tag type, edit named values and numeric arrays, add compound children or homogeneous list items, and remove tags. This also covers `level.dat`, structures, saved world data and mod NBT files.
- GZIP, zlib and uncompressed documents retain their compression and tag types. Signed 64-bit integers keep their full precision. Limits: 16 MiB on disk and after decompression, 64 nesting levels, 100,000 tags and 1,000,000 collection elements. Bedrock little-endian data and region containers (`.mca`/`.mcr`) are not standalone Java NBT documents.
- Browsing is available while running. Saving requires a stopped server, an inactive container and no active background task. Each save checks the opened file's content revision, writes an exact copy to `<filename>.mcm-nbt-<id>.bak`, then replaces the original atomically. Conflicts retain local edits and require reload before another save. Backups remain beside the file until removed; use **Download previous version** or Files to retrieve them. To restore, stop the server and upload the backup under the original filename.

### Scheduled Commands & Server Actions

- Open a server's **Schedules** tab to schedule a console command, start, stop, or graceful restart.
- Choose midnight, hourly, every 30 minutes, weekly, or monthly presets, or enter a standard five-field cron expression (`minute hour day month weekday`). For example, `0 0 * * *` runs at midnight and `*/15 * * * *` runs every 15 minutes. Names such as `midnight`, `daily`, and `@hourly` are also accepted by the API.
- New tasks default to the existing `TZ` environment setting (`UTC` when unset or invalid). Each task saves its timezone and allows an override. Midnight follows the selected zone, including daylight saving changes.
- Edit, pause/resume, and delete tasks; inspect the next run, last result, and error details. Console commands require a running server. Restart follows the manual restart behavior and also starts a stopped server.
- Schedules persist in SQLite and run while the panel is online, within the five-second polling cadence. Missed runs are skipped after panel downtime. A task cannot overlap its own previous execution within the panel process, and pausing or deleting a task leaves an already running execution to finish. Failed runs are recorded and wait for the next scheduled occurrence.

### Automatic update checks

Enable **Settings → Updates → Enable nightly update checks** to check server software, plugins, and mods at **00:00 in the panel timezone** (`TZ`, UTC by default). This is off by default. **Check now** starts an immediate fleet scan after opting in; scans never install updates or restart servers.

Saved results survive panel restarts. Servers with available updates display an **Update Available** badge on the dashboard and server page. The server page shows the software version/build and plugin/mod update count, with actions to review them. Opening the installed plugins/mods list reads saved results; use its re-check action for an explicit refresh. Changing software settings or installed artifacts invalidates corresponding results.

Each fleet scan starts with a fresh shared response cache. Repeated provider requests, including failures, are reused throughout that scan. Modrinth hash results are reused even when servers have overlapping but different mod lists; game version and loader filters stay separate. Software catalog responses are shared too. Failed checks retain previously known updates and show a warning.

Software checks compare published stable versions and builds for the installed version when a build is pinned. Platforms without a supported live catalog (including custom jars) show an explanation. Unrecognized plugin/mod jars may not be identifiable. Nightly CurseForge checks require `CURSEFORGE_API_KEY` on the panel; a browser-only key is available only for manual checks. The panel must be running at midnight; missed scans are not replayed on startup.

Authenticated endpoints: `GET /api/updates` reads saved status and results, `PUT /api/updates/settings` saves `{ "enabled": true }`, and `POST /api/updates/check` starts a scan and returns `202`. Manual and scheduled fleet scans cannot overlap. `GET /api/servers/:id/mods/updates?cached=true` reads saved/in-memory results without contacting catalogs. Mutating endpoints use existing CSRF protection.

### Discord notifications

Open **Settings → Notifications** to connect a Discord channel:

1. In Discord, open the channel's **Settings → Integrations → Webhooks**, create a webhook, and copy its URL.
2. Paste the URL into MCM and use **Send test message** to check delivery. You can test before saving or enabling notifications.
3. Enable notifications, choose the events to send, and save.

Event switches apply to all servers: server starts, stops, crashes, server operation errors, backup completion, and backup failure. Manual actions, scheduled server actions, and automatic backups use the same notifications. A restart sends a stop followed by a start when those events are selected. Start notifications mean the container has started; Minecraft may still be loading.

MCM checks container status every five seconds while the panel process is running, including when no browser is open. An unexpected container exit sends one crash alert with its exit code; deliberate stops and force stops use the stop event. Failed runtime inspections do not count as crashes. Events during panel downtime and exits between polling checks cannot be replayed.

Notifications are disabled by default. Settings persist in the panel database and take effect without a restart. The webhook URL contains a secret: it is stored in the database, hidden from settings responses, and omitted from delivery errors. Leave its field blank to keep the saved URL, enter a replacement, or choose **Remove saved webhook** and save to disconnect. Treat database backups as containing this credential.

Delivery runs through a bounded background queue with HTTP timeouts and bounded retries for Discord rate limits. Discord delivery failures are logged and do not fail server or backup operations. The queue is held in memory, so pending events do not survive a panel restart and may be dropped if the queue fills. Messages disable mentions. Standard HTTPS Discord webhook URLs and optional `thread_id` queries are supported; arbitrary hosts and redirects are rejected. See the [Discord webhook API](https://docs.discord.com/developers/resources/webhook) for channel and thread requirements.

The authenticated API exposes `GET`/`PUT /api/notifications/discord` and `POST /api/notifications/discord/test`. Save `enabled`, an array of `events` (`server_started`, `server_stopped`, `server_crashed`, `server_error`, `backup_completed`, `backup_failed`), and optionally `webhook_url`. Omitting the URL preserves it; an empty URL clears it when disabled. Reads return `enabled`, `configured`, and `events`. The test endpoint accepts `{}` for the saved webhook or `{ "webhook_url": "…" }` for an unsaved URL. Mutating requests require the existing session and CSRF protection.

### 👥 Player & Permissions Management
- **Live Player Roster**: Real-time list of online players with 3D player head avatars (powered by Minotar).
- **Player Moderation**: Quick actions to Kick, Ban, Pardon, OP, De-OP, or Whitelist players.
- **Whitelist & OP Panels**: Searchable and manageable whitelist and operator rosters with customizable OP permission levels (1–4).
- **Interactive "Give Item" Dialog**: Integrated Minecraft item catalog search, count selection, enchantment options, and custom NBT editing.

### ⚙️ Server Properties & Visual MOTD Designer
- **Interactive Property Editor**: Configure key gameplay settings (difficulty, gamemode, PvP, view distance, flight, spawn protection, hardcore, simulation distance, etc.) with intuitive UI toggles.
- **Raw Configuration Mode**: Direct editing of the `server.properties` text file when raw precision is needed.
- **Visual MOTD Designer**: Design server MOTDs with color pickers, Minecraft formatting codes (`§a`, `§l`, etc.), and a real-time server list ping preview.

### 📁 Full-Featured File Manager
- **Server File Explorer**: Navigate the server data volume directly from the browser.
- **File Transfers**: Upload single or multiple files simultaneously with progress tracking, or download files directly.
- **Inline Text Editor**: Edit configuration files (`yml`, `json`, `properties`, `toml`, `txt`) directly in the browser.
- **Archive Operations**: Create zip archives and extract zip files in place.
- **Remote Download**: Fetch remote files or datapacks directly from an external URL.

### 🌐 SRV DNS Routing & Port Management
- **RFC 2782 DNS SRV Management**: Automatically registers, updates, and removes `_minecraft._tcp` SRV records via Cloudflare, Porkbun, DigitalOcean, or Amazon Route 53.
- **Custom Subdomains & Apex Routing**: Route players through custom subdomains (e.g., `survival.example.com`) or apex root domains (`@` -> `example.com`) without exposing non-standard port numbers.
- **One-Click Direct Join Badge**: Displays active join addresses on the server overview with instant clipboard copying.
- **Port Pool Management**: Configure and validate host port allocation ranges (`MCM_PORT_RANGE`), monitor available vs. allocated ports, and prevent port collision conflicts.
- **Automatic Lifecycle Cleanup**: Removing a server automatically deregisters its DNS record with the original provider. Custom SRV names persist across stop/start cycles, record removal, and server renames.

### 📦 S3-Compatible Backups & Scheduling
- **Dedicated Backup Manager**: Open Backups to select a server, manage archives, search by name, filter by local or S3 storage, and save its automatic backup interval. The server's Backups tab remains available.
- **Separate Backup Media**: Store local archives and temporary backup files on another disk or mounted share with `MCM_BACKUP_DIR`. Existing local archives remain accessible.
- **Object Store Integration**: Archive server data directories to compressed `.tar.gz` archives stored in any S3-compatible object store (AWS S3, MinIO, Cloudflare R2, DigitalOcean Spaces).
- **Automated Scheduling & Retention**: Configure automated backup intervals per server with automatic pruning of older snapshots.
- **Instant Restore**: One-click in-place snapshot restoration.
- **Live Backup Progress**: Real-time SSE progress events and progress bars during backup creation.
- **Backup Downloads & Uploads**: Download backup snapshots directly from storage or upload external archives for restoration.

### 🔒 Enterprise Security & Authentication
- **Multi-Factor Authentication (MFA)**:
  - **TOTP Two-Factor Authentication**: Authenticator app enrollment with QR code generation.
  - **WebAuthn / Passkeys**: Hardware security keys (YubiKey), Apple Touch ID / Face ID, and Windows Hello.
- **Multi-User Administration**: Manage panel user accounts, user creation, password updates, and user deletion.
- **Hardening**: Built-in double-submit CSRF cookie protection, brute-force login lockout (5 failed attempts / 15 min), and sliding-window rate limiting.
- **Native TLS Support**: HTTPS on the configured panel port, with a persistent, automatically renewed self-signed certificate when no certificate is supplied.

---

## Alternative: Docker Compose

1. **Copy the environment template and configure a session secret**:

   ```sh
   cp .env.example .env
   # Edit MCM_SESSION_SECRET in .env
   ```

2. **Ensure the Minecraft server runtime base image is available**:

   ```sh
   docker pull eclipse-temurin:21-jre-alpine
   ```

   MCM automatically pulls `eclipse-temurin:<version>-jre-alpine` (for Java 8, 11, 17, or 21) on demand when servers using those versions are created. The default runtime image is configurable via `MCM_SERVER_IMAGE`.

3. **Build and start the panel**:

   ```sh
   docker compose up -d --build
   ```

4. **Access the panel**:
   Open `https://localhost:8080` in your browser and complete the initial onboarding setup. Trust the generated public certificate or accept the self-signed certificate warning on first use.

### Data Persistence & Host Permissions

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft server containers on the host. Minecraft servers bind host ports within `MCM_PORT_RANGE` (default `25565-25665`).

- `MCM_DATA_DIR_HOST`: Host directory (default `${PWD}/data` next to `docker-compose.yml`) mounted into the panel container at `/data` for the SQLite database and server data. This is also the host-side path MCM binds into each Minecraft server container.
- `PUID` / `PGID`: Set to match your host user (e.g. `1000:1000` or `501:20` on macOS) so all files written to the data volume remain owned by your user. The image entrypoint adjusts data-directory permissions before the panel starts.

New Minecraft containers inherit the panel process's UID/GID. Their startup step prepares required tools and repairs ownership of `/data` as root, then drops privileges before running server installers or Java. This keeps automatically generated mod configs editable in the panel. Set `PUID` to a nonzero UID to use this behavior. Custom server images must provide `su-exec` or `gosu`, or an Alpine/Debian package manager to install one.

After upgrading from a version that ran Minecraft as root, use **Rebuild container** for each existing server. Rebuilding preserves its data, replaces the old container, and applies the ownership repair at its next start. Restarting the old container alone does not update its startup settings. This runs Minecraft as a non-root user; running the Docker daemon itself in rootless mode is a separate host configuration.

### Separate backup media

For a native Linux installation, use
`./mcmctl install --backup-dir /mnt/backup-drive/mcm`, or set
`MCM_BACKUP_DIR` in the managed configuration and restart. See
[separate backup media](docs/operations.md#separate-backup-media) for migration
and mount setup. The **Local disk** backup target uses the selected directory.

For Docker or Podman Compose, create a dedicated directory on the mounted
media, set its absolute host path in `.env`, and add the backup override:

```ini
MCM_BACKUP_DIR_HOST=/mnt/backup-drive/mcm
```

```sh
docker compose -f docker-compose.yml -f compose.backups.yml up -d --build
# Or, with Podman:
podman compose -f podman-compose.yml -f compose.backups.yml up -d --build
```

The override mounts that directory at `/backups` and sets `MCM_BACKUP_DIR`
for the panel. Keep using both Compose files for subsequent deployments.
The host directory must already exist and the media must be mounted before
starting the panel. Container startup applies `PUID`/`PGID` ownership to the
backup directory as well as the data directory. Server containers only need
the server data mount.

Local backups, uploads, restores, and retention use the configured backup
directory. Temporary archives for local and S3 operations stay in its `.tmp`
subdirectory, keeping large staging files off the system temporary disk.
Existing backups under `MCM_DATA_DIR/backups` can still be downloaded,
restored, or pruned. To relocate them, stop the panel and copy their server-ID
directories to the new backup root without changing filenames. The panel
database remains in its original location.

### Port Exposure & Multi-Server Networking

- **Direct Connections**: Each server container can publish its game port directly to the host network within `MCM_PORT_RANGE` (default `25565-25665`).
- **Extra Ports**: Additional ports (TCP/UDP) can be allocated per server from the Server Settings tab (e.g., Bedrock/Geyser UDP ports, Voice Chat UDP, or Dynmap/BlueMap WebUI ports).
- **Internal Proxy Networking**: When running behind a reverse proxy (e.g. Velocity or BungeeCord), backend servers can disable host port binding. Sibling containers on the `mcm-network` bridge communicate directly using the `mcm-<server-id>` hostname, eliminating unnecessary port exposure on the host firewall.

---

## Alternative: Podman Compose and direct use

Select Podman with `MCM_CONTAINER_RUNTIME=podman`. MCM uses Podman's
[Docker-compatible API](https://docs.podman.io/en/latest/markdown/podman-system-service.1.html)
for server creation, lifecycle controls, networking, console commands, logs, and
resource metrics. Docker remains the default runtime.

### Rootless Linux (containerized panel)

Enable the API socket as the user that will own the panel and server data:

```sh
systemctl --user enable --now podman.socket
cp .env.example .env
# Set MCM_SESSION_SECRET in .env.
export MCM_PODMAN_SOCKET="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/podman/podman.sock"
podman compose -f podman-compose.yml up -d --build
```

The standalone `podman-compose.yml` selects Podman regardless of the runtime
setting in `.env`. It mounts the API socket, uses a shared SELinux label for
server data, and disables the panel's SELinux process label as required for
socket access. It runs the panel as UID/GID 0 **inside the rootless container**,
which maps to the invoking host user and keeps the socket and data accessible.
Minecraft containers preserve that ownership mapping. Run the panel and its
server containers through the same user's Podman service.

`MCM_PODMAN_SOCKET` is an absolute filesystem path, such as
`/run/user/1000/podman/podman.sock`, **without** `unix://`. The prefix belongs only
in API endpoint variables such as `MCM_CONTAINER_HOST`. The Compose file requires
`MCM_DATA_DIR_HOST` to be set to the absolute host data directory, and defaults the
socket path to `/run/podman/podman.sock` if `MCM_PODMAN_SOCKET` is unset. Its variable
expressions are kept flat for compatibility with older `podman-compose` providers.
The copied `.env` template sets `MCM_DATA_DIR_HOST` to `${PWD}/data`; retain your
existing value if you already keep server data elsewhere.

If `podman-compose` reports `unknown mount option /run/podman/podman.sock`, update
`podman-compose.yml` to this version, set `MCM_PODMAN_SOCKET` as shown above, ensure
`MCM_DATA_DIR_HOST` is configured in `.env`, and retry.
The previous file used nested default expressions that older providers left
partially expanded inside the socket mount.

`podman compose` requires an installed Compose provider, such as `podman-compose`
or Docker Compose. For a rootful deployment, enable `podman.socket` with
`sudo systemctl enable --now podman.socket` and set
`MCM_PODMAN_SOCKET=/run/podman/podman.sock`; UID/GID 0 then means host root.
Rootless resource limits require cgroup v2 with CPU and memory controllers
delegated to the user service.

### Running the panel directly

With the rootless socket enabled, launch the built binary as the same user:

```sh
MCM_CONTAINER_RUNTIME=podman ./mcm
```

The default endpoint is `unix://$XDG_RUNTIME_DIR/podman/podman.sock` (falling back
to `/run/user/<uid>/podman/podman.sock`) for non-root processes and
`unix:///run/podman/podman.sock` for root. MCM preserves file ownership with
Podman's `keep-id` mapping when the API reports rootless mode. Installers and
Java then run as the panel's UID/GID.

To override the endpoint, set `MCM_CONTAINER_HOST`. The resolution order is
`MCM_CONTAINER_HOST`, `DOCKER_HOST`, then (in Podman mode) `CONTAINER_HOST`, followed
by the runtime's default socket. Clear an existing `DOCKER_HOST` when switching
to Podman's default. Absolute socket paths such as
`/run/user/1000/podman/podman.sock` are automatically prefixed with `unix://`.
Other endpoints must use a transport supported by the Docker client, such as
`unix://` or `tcp://`; use a locally forwarded socket for an SSH connection.

For a Linux user service, use the [managed installation](#install-manage-and-update-linux)
above. It builds and installs the binary, creates the required environment file,
and enables the socket and service automatically. Do not also start a Compose
panel against the same data directory.

### macOS and remote hosts

Start your Podman machine and point a directly running panel at its forwarded
socket:

```sh
podman machine start
export MCM_CONTAINER_RUNTIME=podman
export MCM_CONTAINER_HOST="unix://$(podman machine inspect --format '{{.ConnectionInfo.PodmanSocket.Path}}')"
./mcm
```

`MCM_DATA_DIR_HOST` must refer to the same data directory on the machine running
Podman. For a VM, share that directory with write access; for a remote host,
provide shared storage. Containerized panels must mount the **VM-side** Podman
socket, not the macOS forwarding socket.

Podman image references are normalized to Docker Hub (`docker.io/library/` for
Eclipse Temurin), so automatic pulls do not depend on short-name registry search
configuration. Explicit registries, digests, and `localhost/` images are preserved.

Switching engines does not move existing containers. Stop servers before switching,
keep the same server data path, and let MCM recreate containers in the selected
engine. Start only one panel against a given database/data directory.

---

## Bare metal and LXC

Use `./mcmctl install --runtime podman` or `./mcmctl install --runtime docker`
on Linux with systemd. The panel runs directly as your user; Minecraft servers
run through the selected container engine. Container engines inside LXC require
host support for the necessary namespaces, mounts, and cgroups.

If LXC prevents Podman or Docker from running, resolve that at the LXC host
before installing MCM. Check the chosen engine's `info` command as the same
user who will run the panel. See [operations](docs/operations.md) for the complete
service lifecycle and recovery commands.

---

## Configuration Reference

### Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `MCM_ADDR` | `:8080` | Listen address for the web and REST API server. |
| `MCM_PORT_RANGE` | `25565-25665` | Host port pool allocated to Minecraft servers. |
| `MCM_DATA_DIR_HOST` | `${PWD}/data` | Docker: host directory bind-mounted into the panel at `/data` and into each server container. |
| `MCM_DATA_DIR` | `/data` (Docker) / `./data` (bare metal) | Process-side data directory used for SQLite database and server data. |
| `MCM_BACKUP_DIR` | `$MCM_DATA_DIR/backups` | Process-side directory for local backup archives and temporary backup/restore files. May be on separate media; `compose.backups.yml` sets `/backups`. |
| `MCM_BACKUP_DIR_HOST` | *(empty)* | Existing absolute host backup directory mounted at `/backups` when using `compose.backups.yml`. |
| `MCM_DB_PATH` | `$MCM_DATA_DIR/mcm.db` | SQLite database file location. |
| `DOCKER_HOST` | Runtime-specific socket | API endpoint for Docker or Podman. Docker defaults to `unix:///var/run/docker.sock`; Podman defaults are described above. |
| `MCM_CONTAINER_RUNTIME` | `docker` | Container engine: `docker` or `podman`. |
| `MCM_CONTAINER_HOST` | *(empty)* | API endpoint override for either engine; takes priority over `DOCKER_HOST`. |
| `CONTAINER_HOST` | *(empty)* | Podman endpoint fallback when `MCM_CONTAINER_HOST` and `DOCKER_HOST` are unset. |
| `MCM_PODMAN_SOCKET` | `/run/podman/podman.sock` | Absolute host socket path mounted by `podman-compose.yml`, without `unix://`; set explicitly for rootless or VM deployments. |
| `PUID` / `PGID` | `1000` / `1000` | UID/GID the panel runs as and applies to the data directory. |
| `MCM_SESSION_SECRET` | *(required)* | Secret key used to sign session cookies. |
| `MCM_HTTP_ONLY` | `false` | Serve plain HTTP instead of native HTTPS, for local development or a TLS-terminating reverse proxy. Cannot be combined with supplied certificate paths. |
| `MCM_TLS` | `false` | Force Secure cookies when using HTTP behind an HTTPS reverse proxy. Native HTTPS always enables Secure cookies. |
| `MCM_TLS_CERT` | *(empty)* | Path to TLS certificate (PEM). Without a supplied pair, MCM generates and reuses `$MCM_DATA_DIR/tls/cert.pem` and `key.pem`. |
| `MCM_TLS_KEY` | *(empty)* | Path to TLS private key (PEM). |
| `MCM_TLS_REDIRECT` | `false` | Redirect plain HTTP to HTTPS on the same `MCM_ADDR` port (301). Only applies to native HTTPS. |
| `MCM_LOGIN_MAX_ATTEMPTS` | `5` | Maximum failed login attempts allowed before lockout. |
| `MCM_LOGIN_LOCKOUT` | `15m` | Lockout duration after exceeding failed login attempts. |
| `MCM_RATE_LIMIT_MAX` | `100` | Maximum state-changing requests permitted per client per window. |
| `MCM_RATE_LIMIT_WINDOW` | `1m` | Sliding window duration for rate limiting. |
| `MCM_DEFAULT_CPU_LIMIT` | `0` | Default CPU cores limit for new servers (0 = unlimited). |
| `MCM_DEFAULT_MEMORY_MB` | `0` | Default memory limit in MB for new servers (0 = configured server RAM + 2 GiB). |
| `MCM_SERVER_IMAGE` | `eclipse-temurin:21-jre-alpine` | Base Docker image used for server containers (Java 8, 11, 17, 21 pulled dynamically). |
| `MCM_WEB_AUTHN_RPID` | `localhost` | WebAuthn Relying Party ID (effective domain). |
| `MCM_WEB_AUTHN_RP_ORIGIN` | Derived | Allowed WebAuthn origins (comma-separated, e.g. `https://mc.example.com`). |
| `MCM_WEB_AUTHN_RP_NAME` | `Minecraft Container Manager` | Display name presented during Passkey registration. |
| `MCM_S3_ENDPOINT` | *(empty)* | S3-compatible object store endpoint (e.g. `http://minio:9000`). Empty disables S3 storage; local backups remain available. |
| `MCM_S3_ACCESS_KEY` | *(empty)* | Access key for S3 backup storage. |
| `MCM_S3_SECRET_KEY` | *(empty)* | Secret key for S3 backup storage. |
| `MCM_S3_BUCKET` | *(empty)* | S3 bucket used for server backup archives. |
| `MCM_S3_REGION` | `us-east-1` | S3 region for signing requests. |
| `TZ` | `UTC` | Container and server timezone. |

### HTTPS certificates

MCM serves HTTPS on `MCM_ADDR` by default, including existing installations after
an upgrade. For the first upgrade from an HTTP-default version, pull this checkout
and run `./mcmctl update --local` using the updated script; the old installed
script only checks HTTP readiness. When `MCM_TLS_CERT` and `MCM_TLS_KEY` are empty, it stores a self-signed
ECDSA certificate and private key under `$MCM_DATA_DIR/tls`. The private key is
readable only by the panel's user. The certificate covers localhost, the host's
name and LAN addresses, the configured listen host, and the configured WebAuthn
RP ID/origin hosts. It is reused across restarts and renewed at startup when fewer
than 30 days remain or required hostnames change. Certificates last one year.

Trust `tls/cert.pem` on your client devices to remove the browser warning; keep
`tls/key.pem` private. For a public certificate, set both PEM paths. In Compose,
mount those files into the panel and use their container-side paths. Invalid or
incomplete supplied certificates cause startup to fail.

`MCM_ADDR` controls the only panel listener. Set `MCM_TLS_REDIRECT=true` to redirect
plain HTTP to HTTPS on that same port. For example, `http://mc.example.com:8080/login`
redirects to `https://mc.example.com:8080/login`, preserving the path and query.
The legacy `MCM_TLS_REDIRECT_ADDR` setting is ignored; MCM never opens a separate
redirect port. With redirects disabled, plain HTTP receives an HTTPS-required
error. For plain HTTP service, set
`MCM_HTTP_ONLY=true`; behind an HTTPS reverse proxy, also set `MCM_TLS=true` and
configure `MCM_WEB_AUTHN_RPID` and `MCM_WEB_AUTHN_RP_ORIGIN` for the public URL.

### Dynamic Panel Settings

Configurable via the panel Settings UI or `PUT /api/settings`:

| Setting Key | Default | Description |
| --- | --- | --- |
| `dns_provider` | `cloudflare` | `cloudflare`, `porkbun`, `digitalocean`, or `route53`. Credentials are retained separately for each provider. |
| `dns_publish` | `false` | Master toggle to enable SRV DNS management. |
| `dns_domain` | *(empty)* | Apex zone domain name (e.g. `example.com`). |
| `dns_zone` | *(empty)* | Cloudflare Zone ID (32-character hexadecimal string). |
| `dns_api_token` | *(empty)* | Cloudflare API token with `Zone:Read` and `DNS:Edit` permissions. |
| `dns_porkbun_api_token` | *(empty)* | Porkbun API key. Enable API access for the domain. |
| `dns_porkbun_api_secret` | *(empty)* | Porkbun secret API key. |
| `dns_porkbun_zone` | `dns_domain` | Registered zone domain, e.g. `example.com`; can differ from a base domain such as `games.example.com`. |
| `dns_digitalocean_api_token` | *(empty)* | DigitalOcean token with domain read/create/update/delete permissions. |
| `dns_digitalocean_zone` | `dns_domain` | Zone domain in DigitalOcean Networking. |
| `dns_route53_api_token` | *(empty)* | AWS access key ID. |
| `dns_route53_api_secret` | *(empty)* | AWS secret access key. |
| `dns_route53_session_token` | *(empty)* | Optional STS session token for temporary AWS credentials. |
| `dns_route53_zone` | *(empty)* | Public hosted zone ID. IAM permissions: `route53:GetHostedZone`, `route53:ListResourceRecordSets`, and `route53:ChangeResourceRecordSets`. |
| `dns_host` | *(empty)* | Target A/AAAA host pointing to the panel host IP (falls back to `dns_domain`). |
| `dns_service` | `_minecraft` | Service name according to RFC 2782. |
| `dns_proto` | `_tcp` | Protocol name according to RFC 2782. |
| `dns_ttl` | `120` | DNS record TTL in seconds (`1` = Auto on Cloudflare). Clamped to 600 on Porkbun and 30 on DigitalOcean. |
| `port_range_start` | `25565` | Start of the port allocation pool range. |
| `port_range_end` | `25665` | End of the port allocation pool range. |

---

## API Reference

MCM exposes a clean JSON REST API along with Server-Sent Events (SSE) for console and task progress streaming.

### Authentication, Onboarding & MFA
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/auth/csrf` | Fetch CSRF protection token. |
| `GET` | `/api/onboarding/status` | Check if initial setup/onboarding has completed. |
| `POST` | `/api/onboarding` | Create initial administrator account. |
| `POST` | `/api/auth/login` | Authenticate with email/password (and TOTP code if enabled). |
| `POST` | `/api/auth/logout` | Invalidate current session. |
| `GET` | `/api/auth/me` | Fetch authenticated user profile. |
| `GET` | `/api/auth/totp` | Get TOTP two-factor enrollment status. |
| `POST` | `/api/auth/totp/enroll` | Begin TOTP enrollment and generate secret/URI. |
| `POST` | `/api/auth/totp/enroll/confirm` | Confirm TOTP code and enable two-factor auth. |
| `POST` | `/api/auth/totp/disable` | Disable TOTP two-factor authentication. |
| `POST` | `/api/passkey/register/begin` | Begin WebAuthn passkey registration ceremony. |
| `POST` | `/api/passkey/register/finish` | Finalize WebAuthn passkey credential storage. |
| `GET` | `/api/passkey` | List registered passkeys for current user. |
| `DELETE` | `/api/passkey` | Delete a registered passkey. |
| `POST` | `/api/passkey/login/begin` | Begin WebAuthn passkey authentication ceremony. |
| `POST` | `/api/passkey/login/finish` | Finalize WebAuthn passkey login. |

### Server Lifecycle & Management
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers` | List all managed Minecraft servers with status and uptime. |
| `POST` | `/api/servers` | Create a new server (flavor, version, build, RAM, CPU, ports, java_version). |
| `GET` | `/api/servers/:id` | Get server configuration, container state, and rebuild warning status. |
| `PATCH` | `/api/servers/:id` | Update server configuration (RAM, CPU, ports, java_version). |
| `DELETE` | `/api/servers/:id` | Delete server and remove container/data volume. |
| `POST` | `/api/servers/:id/start` | Start server container. |
| `POST` | `/api/servers/:id/stop` | Gracefully stop server container. |
| `POST` | `/api/servers/:id/restart` | Restart server container. |
| `POST` | `/api/servers/:id/kill` | Force-kill unresponsive server process (`SIGKILL`). |
| `POST` | `/api/servers/:id/recreate` | Recreate container from runtime image and sync configuration changes. |
| `POST` | `/api/servers/:id/copy` | Duplicate/clone server data, configuration, and plugins to a new server. |
| `GET` | `/api/servers/:id/export` | Download full server data archive (`.tar.gz`). |
| `GET` | `/api/servers/:id/status` | Lightweight status polling endpoint. |
| `GET` | `/api/servers/:id/stats` | Real-time container resource metrics (CPU %, memory, net I/O, disk I/O). |
| `GET` | `/api/servers/:id/console` | Stream real-time console logs via Server-Sent Events (SSE) with `Last-Event-ID`. |
| `GET` | `/api/servers/:id/logs` | List saved server logs; pass `?path=logs/archive.tar.gz` to list log members in a tar archive. |
| `GET` | `/api/servers/:id/logs/content` | Preview a plain or gzip-compressed log using `?path=...`; add `&entry=...` for a tar archive member. Returns content, truncation state, and preview limit. |
| `POST` | `/api/servers/:id/console/command` | Send command to server stdin FIFO pipe (with RCON fallback). |
| `GET` | `/api/servers/:id/install` | Get installed version and available upgrade builds. |
| `POST` | `/api/servers/:id/install` | Trigger server version or build upgrade. |
| `GET` | `/api/jars/:kind/versions` | Fetch upstream Minecraft versions for a given server type. |
| `GET` | `/api/jars/:kind/versions/:v/builds` | Fetch builds for a specific version. |
| `GET` | `/api/system/java-versions` | List available Java runtime versions supported by the system. |
| `GET` | `/api/system/capacity` | Get runtime CPU/memory capacity, aggregate measured server usage, and data-directory filesystem space. Requires authentication. Runtime, usage, and disk failures are reported independently so available readings remain visible. |

### Modpack Management
| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/modpack/inspect` | Inspect an external modpack to detect loader, version, and requirements. |
| `GET` | `/api/servers/:id/modpack` | Get currently installed modpack metadata and version lock state. |
| `POST` | `/api/servers/:id/modpack/inspect` | Inspect modpack compatibility against a specific server. |
| `POST` | `/api/servers/:id/modpack/install` | Trigger asynchronous modpack installation or version change. |
| `DELETE` | `/api/servers/:id/modpack` | Uninstall modpack configuration from server. |

### Mod Updates & Version Upgrade Helper
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/mods/updates` | Fetch cached or detected updates for installed mods. |
| `POST` | `/api/servers/:id/mods/updates/check` | Check for updates across Modrinth and CurseForge catalogs. |
| `GET` | `/api/servers/:id/mods/upgrade-check` | Check mod compatibility for upgrading the server's Minecraft version. |
| `POST` | `/api/servers/:id/mods/upgrade-apply` | Apply version upgrade across mods and server platform. |
| `GET` | `/api/servers/:id/mods/:name/versions` | Fetch all available versions and jar files for a specific mod. |
| `POST` | `/api/servers/:id/mods/:name/update` | Update a specific mod jar to a target version. |
| `POST` | `/api/servers/:id/mods/sanitize-client-only` | Scan and remove client-only mods from the server. |

### Background Tasks & Progress Tracking
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/tasks/progress` | Poll active background task progress (modpack install, zip, etc.). |
| `GET` | `/api/servers/:id/tasks/events` | Stream live task progress events via Server-Sent Events (SSE). |

### Scheduled Tasks
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/schedules` | List recurring tasks, next run, and last result. |
| `POST` | `/api/servers/:id/schedules` | Create a task with `name`, `action` (`command`, `start`, `stop`, `restart`), `command` (for command tasks), `cron`, optional `timezone` (defaults to `TZ`), and optional `enabled` (defaults to true). |
| `PUT` | `/api/servers/:id/schedules/:scheduleId` | Replace task configuration; include `enabled`. |
| `PATCH` | `/api/servers/:id/schedules/:scheduleId` | Pause or resume with `{"enabled": false}` or `{"enabled": true}`. |
| `DELETE` | `/api/servers/:id/schedules/:scheduleId` | Delete a task and cancel its future runs. |

### Players, Whitelist & Operators
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/players` | List online players with player head avatars. |
| `POST` | `/api/servers/:id/players/:name/command` | Run player command (kick, ban, pardon, op, give item). |
| `GET` | `/api/servers/:id/ops` | List operator accounts and permission levels. |
| `POST` | `/api/servers/:id/ops` | Add an operator with permission level (1–4). |
| `DELETE` | `/api/servers/:id/ops/:name` | Remove operator status. |
| `GET` | `/api/servers/:id/whitelist` | List whitelisted players. |
| `POST` | `/api/servers/:id/whitelist` | Add player to whitelist. |
| `DELETE` | `/api/servers/:id/whitelist/:name` | Remove player from whitelist. |

### Mods & Plugins
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/mods` | List installed mods or plugins with enablement status. |
| `POST` | `/api/servers/:id/mods` | Upload mod/plugin jar file. |
| `POST` | `/api/servers/:id/mods/download` | Download mod/plugin directly from catalog URL. |
| `PATCH` | `/api/servers/:id/mods/:name` | Toggle mod/plugin enabled or disabled (`.disabled`). |
| `DELETE` | `/api/servers/:id/mods/:name` | Delete mod/plugin jar file. |

### Server Properties
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/worlds` | List worlds, active world and whether world writes are available. |
| `POST` | `/api/servers/:id/worlds` | Create a world with `name`, string `seed`, `level_type` and `generate_structures`. |
| `POST` | `/api/servers/:id/worlds/import` | Import a ZIP using multipart `name` and `file` fields. |
| `PUT` | `/api/servers/:id/worlds/:world` | Update an ungenerated world’s seed and generation settings. |
| `POST` | `/api/servers/:id/worlds/:world/activate` | Select the world for the next server start. |
| `GET` | `/api/servers/:id/worlds/:world/export` | Download a world and matching Bukkit dimensions as ZIP. |
| `DELETE` | `/api/servers/:id/worlds/:world` | Move an inactive world to recovery storage; returns `recovery_path`. |
| `GET` | `/api/servers/:id/worlds/:world/regions?dimension=world&offset=0` | List up to 100 region headers for a dimension ID returned by the world list. |
| `GET` | `/api/servers/:id/worlds/:world/chunks?dimension=world&x=0&z=0` | List generated chunks and saved timestamps in a region. |
| `POST` | `/api/servers/:id/worlds/:world/chunks/reset` | Reset `chunks` (unique local indices `z * 32 + x`, 0–1023) in `dimension`, `region_x`, `region_z`; returns count and `recovery_path`. |
| `GET` | `/api/servers/:id/properties` | Fetch current parsed `server.properties`. |
| `PUT` | `/api/servers/:id/properties` | Update `server.properties` content. |

### File Manager
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/files` | List files and directories in server volume. |
| `GET` | `/api/servers/:id/files/download` | Download file from server. |
| `GET` | `/api/servers/:id/files/content` | Read text file content and its revision for inline editing. |
| `PUT` | `/api/servers/:id/files/content` | Save edited text; pass `expected_revision` from the read response to check for changes. An empty revision requires a new file. A `409 file_changed` response includes the current file in `current` for comparison. |
| `POST` | `/api/servers/:id/files/upload` | Upload one or more files. |
| `POST` | `/api/servers/:id/files/archive` | Create a zip archive of selected files/folders. |
| `POST` | `/api/servers/:id/files/unzip` | Extract a zip archive in place. |
| `POST` | `/api/servers/:id/files/from_url` | Download a remote file directly to the server data directory. |
| `DELETE` | `/api/servers/:id/files` | Delete file or directory. |
| `POST` | `/api/servers/:id/files/mkdir` | Create a new directory. |
| `POST` | `/api/servers/:id/files/rename` | Rename a file or directory. |

### Backups
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/backups` | List backups for a server. |
| `POST` | `/api/servers/:id/backup` | Trigger an immediate manual backup snapshot. |
| `GET` | `/api/servers/:id/backups/progress` | Poll active backup operation progress. |
| `GET` | `/api/servers/:id/backups/events` | Stream live backup progress events via SSE. |
| `POST` | `/api/servers/:id/backups/upload` | Upload an existing backup archive (.tar.gz). |
| `POST` | `/api/servers/:id/restore/:backupId` | Restore a backup snapshot in place. |
| `GET` | `/api/backups/:backupId/download` | Download backup archive file from storage. |
| `DELETE` | `/api/backups/:backupId` | Delete a backup archive from storage. |

### SRV DNS

Choose the provider under **Settings → DNS Providers** and use **Test Connection** before enabling publishing. Provider API references: [Cloudflare](https://developers.cloudflare.com/api/resources/dns/subresources/records/), [Porkbun](https://porkbun.com/api/json/v3/documentation), [DigitalOcean](https://docs.digitalocean.com/reference/api/reference/domain-records/), [Route 53](https://docs.aws.amazon.com/Route53/latest/APIReference/API_ChangeResourceRecordSets.html). Route 53 supports simple SRV records in public hosted zones on the global AWS endpoint.

`POST /api/dns/test` accepts optional `provider`, `api_token`, `api_secret`, `session_token`, `zone`, and `domain` fields. Missing credentials use that provider's saved values. `GET /api/servers/{id}/dns` includes the saved `subdomain` even when no record is published. Use `@` for the apex domain. Removing the published record preserves this address preference; deleting the server removes it.

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/dns` | List all active DNS records and DNS provider connection status. |
| `POST` | `/api/dns/test` | Test saved or supplied DNS provider credentials and zone connectivity (also works while publishing is disabled). |
| `GET` | `/api/servers/:id/dns` | Get DNS record status and join address for a server. |
| `POST` | `/api/servers/:id/dns` | Publish or update an SRV record for a server. |
| `DELETE` | `/api/servers/:id/dns` | Remove an SRV record for a server. |

### User Management, Ports, Settings & Health
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/users` | List all registered user accounts. |
| `POST` | `/api/users` | Create a new user account. |
| `PATCH` | `/api/users/:id` | Update user email or password. |
| `DELETE` | `/api/users/:id` | Delete user account and associated passkeys/sessions. |
| `GET` | `/api/ports/available` | Check available host ports in allocation range. |
| `GET` | `/api/settings` | Fetch global panel settings (DNS, port range). |
| `PUT` | `/api/settings` | Update global panel settings. |
| `GET` | `/api/runtime/status` | Get selected engine (`runtime`), API reachability, and runtime image status. |
| `GET` | `/api/docker/status` | Compatibility alias for `/api/runtime/status`. |
| `GET` | `/api/proxy` | Cached and rate-limited reverse proxy for catalog APIs. |
| `POST` | `/api/proxy` | POST requests through external catalog proxy. |
| `GET` | `/healthz` | Liveness health check probe (`200 OK`). |
| `GET` | `/readyz` | Readiness probe checking database and container runtime connectivity (legacy check keys: `docker` and `docker_image`). |

---

## Project Structure

```text
.
├── cmd/mcm/                # Main Go application entrypoint
├── internal/
│   ├── api/                # HTTP routes, middlewares, SSE console/task streams, and handlers
│   ├── auth/               # Password hashing, sessions, TOTP, and WebAuthn passkeys
│   ├── backups/            # S3 client, archive engine, and background backup scheduler
│   ├── config/             # Environment variable parsing and defaults
│   ├── db/                 # SQLite connection and migration runner
│   ├── dns/                # Cloudflare, Porkbun, DigitalOcean, and Route 53 SRV records
│   ├── docker/             # Docker engine client, container provisioning, and resource metrics
│   ├── jars/               # Version and build metadata resolution for 22 server platforms
│   ├── ports/              # Host port allocation pool manager and validator
│   ├── proxy/              # Cached, rate-limited reverse proxy for catalog APIs
│   ├── rcon/               # Minecraft RCON client implementation
│   ├── servers/            # Server entity store, modpacks, mod updates, tasks, and state transitions
│   └── web/                # Embedded production frontend assets
├── migrations/             # SQL schema migrations (SQLite)
├── web/                    # React / Vite frontend application
│   ├── src/
│   │   ├── api/            # Typed API client, rate-limited fetchers, and upstream catalog adapters
│   │   ├── components/     # UI components (Console, Modpacks, Mod Updates, Stats Grid, Proxy Editor, etc.)
│   │   ├── routes/         # TanStack Router page routes (Dashboard, Server Detail, Settings, Users)
│   │   └── styles/         # CSS design system and theme tokens
│   └── tests/              # Vitest test suite for web components and utilities
├── deploy/                 # Service examples and installer regression tests
├── mcmctl                  # Linux install, service management, update, and recovery
├── Dockerfile              # Multi-stage production build (Node -> Go -> Final Alpine)
└── docker-compose.yml      # Docker Compose configuration
```

---

## Development

### Backend (Go)

Ensure Go 1.26+ is installed (see `go.mod`):

```sh
# Build binary
go build ./...

# Run backend tests
go test ./...
```

### Frontend (React / Vite)

```sh
cd web

# Install dependencies
npm install

# Run dev server with hot reload
npm run dev

# Run Vitest test suite
npm run test

# Typecheck and production build
npm run typecheck
npm run build
```

During the multi-stage Docker build, `web/dist` is compiled and embedded directly into `internal/web/dist`, producing a single self-contained binary.

---

## License & Credits

- Java runtimes provided by [Eclipse Temurin](https://adoptium.net/) Alpine OpenJDK images.
- Mod & plugin catalog integrations powered by [Modrinth](https://modrinth.com/), [PaperMC Hangar](https://hangar.papermc.io/), [SpigotMC / Spiget](https://spiget.org/), and [CurseForge](https://curseforge.com/).
- Player head avatars provided by [Minotar](https://minotar.net/).
