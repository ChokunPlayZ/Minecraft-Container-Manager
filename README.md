# MCM - Minecraft Container Manager

MCM is a modern, high-performance, self-hosted Minecraft server management panel. Built with a lightweight **Go** backend and a responsive **React / Vite** frontend, MCM orchestrates Minecraft servers as sibling Docker containers on the host using lightweight, security-hardened **Eclipse Temurin Alpine OpenJDK/JRE** images.

Each Minecraft server container runs with MCM's custom native lifecycle entrypoint script (`DefaultEntryScript`), providing interactive console FIFO pipes, graceful shutdown traps (`SIGTERM`/`SIGINT`), headless Java configuration, and automated installer execution for Forge and NeoForge. MCM supports both direct host-port publication (for zero-proxy overhead, raw performance, and UDP/TCP support) and private container networking (`mcm-network`) with optional host ports for seamless multi-server proxy setups (e.g., Velocity or BungeeCord).

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
- **Live Container Resource Metrics**: Real-time stats polled directly from the Docker daemon:
  - **CPU Utilization %**: Normalized across available host cores.
  - **Memory Usage & Limits**: Real-time memory consumption, limits, and usage percentage.
  - **Network I/O**: Live incoming (`rx`) and outgoing (`tx`) network throughput.
  - **Block / Disk I/O**: Real-time disk read and write bytes.
- **Server Uptime Tracking**: Tracks server start timestamps (`started_at`) and displays live, human-readable uptime (e.g., `3d 4h 12m`) on dashboard cards and server headers.
- **ServerStatsGrid**: Clean, visual metrics dashboard available directly on the server overview and management pages.

### 🌐 Proxy Networks & Multi-Server Architecture
- **Dedicated Container Network (`mcm-network`)**: Sibling containers join an isolated Docker bridge network where each container is addressable by its internal hostname (`mcm-<server-id>`).
- **Proxy Config Editor**: Built-in visual editor and configuration generator for Velocity (`velocity.toml`) and BungeeCord / Waterfall (`config.yml`).
- **Optional Host Ports**: Run backend servers entirely private on the internal Docker network with zero exposed host ports, routing all player traffic securely through your proxy.

### 🔄 Container Lifecycle, Rebuild Warnings & Server Cloning
- **Lifecycle Controls**: Start, Graceful Stop (`SIGTERM` with `stop`/`end`/`shutdown` commands piped to console), Restart, and Force-Kill (`SIGKILL`).
- **Rebuild Warning System**: Automatically detects when server configuration changes (Java runtime, memory limits, CPU quotas, port bindings) require container recreation and displays an actionable rebuild banner.
- **1-Click Container Rebuild**: Recreate sibling containers on demand without data loss to apply updated environment variables or fresh base images.
- **Instant Server Duplication / Cloning**: 1-click server cloning (`POST /api/servers/:id/copy`) that duplicates server data, configurations, mods, and plugins while automatically allocating fresh host ports.

### ⚡ Live Console & Command Line
- **Real-Time Streaming**: Live server logs streamed directly to the browser using Server-Sent Events (SSE).
- **Last-Event-ID Resumption**: Seamlessly reconnects to log streams after network hiccups without duplicate messages or log gaps.
- **Interactive Command Input**: Send console commands directly to the running server via native FIFO stdin pipe (`/tmp/console.in`) with RCON fallback.
- **Smart Auto-Scroll**: Sticky bottom-scrolling that pauses automatically when scrolling up to inspect past logs.

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

### 🌐 Cloudflare SRV DNS Routing & Port Management
- **RFC 2782 DNS SRV Management**: Automatically registers, updates, and removes `_minecraft._tcp` SRV records via Cloudflare API v4.
- **Custom Subdomains & Apex Routing**: Route players through custom subdomains (e.g., `survival.example.com`) or apex root domains (`@` -> `example.com`) without exposing non-standard port numbers.
- **One-Click Direct Join Badge**: Displays active join addresses on the server overview with instant clipboard copying.
- **Port Pool Management**: Configure and validate host port allocation ranges (`MCM_PORT_RANGE`), monitor available vs. allocated ports, and prevent port collision conflicts.
- **Automatic Lifecycle Cleanup**: Removing a server automatically deregisters its DNS record on Cloudflare.

### 📦 S3-Compatible Backups & Scheduling
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
- **Native TLS Support**: Automatic HTTPS termination with configurable HTTP-to-HTTPS redirect.

---

## Quickstart (Docker Compose)

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
   Open `http://localhost:8080` in your browser and complete the initial onboarding setup.

### Data Persistence & Host Permissions

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft server containers on the host. Minecraft servers bind host ports within `MCM_PORT_RANGE` (default `25565-25665`).

- `MCM_DATA_DIR_HOST`: Host directory (default `${PWD}/data` next to `docker-compose.yml`) mounted into the panel container at `/data` for the SQLite database and server data. This is also the host-side path MCM binds into each Minecraft server container.
- `PUID` / `PGID`: Set to match your host user (e.g. `1000:1000` or `501:20` on macOS) so all files written to the data volume remain owned by your user. The image entrypoint adjusts data-directory permissions before the panel starts.

### Port Exposure & Multi-Server Networking

- **Direct Connections**: Each server container can publish its game port directly to the host network within `MCM_PORT_RANGE` (default `25565-25665`).
- **Extra Ports**: Additional ports (TCP/UDP) can be allocated per server from the Server Settings tab (e.g., Bedrock/Geyser UDP ports, Voice Chat UDP, or Dynmap/BlueMap WebUI ports).
- **Internal Proxy Networking**: When running behind a reverse proxy (e.g. Velocity or BungeeCord), backend servers can disable host port binding. Sibling containers on the `mcm-network` bridge communicate directly using the `mcm-<server-id>` hostname, eliminating unnecessary port exposure on the host firewall.

---

## Bare Metal / LXC Deployment

For hosts running MCM directly outside of Docker:

```sh
# Install the mcm binary and the systemd unit
sudo install -m 0755 ./mcm /usr/local/bin/mcm
sudo install -d /etc/mcm /var/lib/mcm
sudo install -m 0640 deploy/mcm.service /etc/systemd/system/mcm.service

# Create runtime user and configure environment
sudo useradd --system --home /var/lib/mcm --shell /usr/sbin/nologin mcm
sudo install -m 0640 .env.example /etc/mcm/mcm.env
# Edit /etc/mcm/mcm.env: configure MCM_SESSION_SECRET, paths, and settings

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable --now mcm
```

The unit runs the binary as the `mcm` user with `WorkingDirectory=/var/lib/mcm` and reads configuration from `/etc/mcm/mcm.env`. Ensure `docker.service` is active and accessible via `/var/run/docker.sock`.

---

## Configuration Reference

### Environment Variables

| Variable | Default | Description |
| --- | --- | --- |
| `MCM_ADDR` | `:8080` | Listen address for the web and REST API server. |
| `MCM_PORT_RANGE` | `25565-25665` | Host port pool allocated to Minecraft servers. |
| `MCM_DATA_DIR_HOST` | `${PWD}/data` | Docker: host directory bind-mounted into the panel at `/data` and into each server container. |
| `MCM_DATA_DIR` | `/data` (Docker) / `./data` (bare metal) | Process-side data directory used for SQLite database and server data. |
| `MCM_DB_PATH` | `$MCM_DATA_DIR/mcm.db` | SQLite database file location. |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker daemon endpoint. |
| `PUID` / `PGID` | `1000` / `1000` | UID/GID the panel runs as and applies to the data directory. |
| `MCM_SESSION_SECRET` | *(required)* | Secret key used to sign session cookies. |
| `MCM_TLS` | `false` | Force secure cookies flag. Set to true when running under HTTPS. |
| `MCM_TLS_CERT` | *(empty)* | Path to TLS certificate (PEM). Setting this with `MCM_TLS_KEY` enables native HTTPS. |
| `MCM_TLS_KEY` | *(empty)* | Path to TLS private key (PEM). |
| `MCM_TLS_REDIRECT` | `true` | When TLS is enabled, redirects plain HTTP traffic to HTTPS (301). |
| `MCM_TLS_REDIRECT_ADDR` | Derived (`:80`) | Listen address for HTTP-to-HTTPS redirection. |
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
| `MCM_S3_ENDPOINT` | *(empty)* | S3-compatible object store endpoint (e.g. `http://minio:9000`). Empty disables backups. |
| `MCM_S3_ACCESS_KEY` | *(empty)* | Access key for S3 backup storage. |
| `MCM_S3_SECRET_KEY` | *(empty)* | Secret key for S3 backup storage. |
| `MCM_S3_BUCKET` | *(empty)* | S3 bucket used for server backup archives. |
| `MCM_S3_REGION` | `us-east-1` | S3 region for signing requests. |
| `TZ` | `UTC` | Container and server timezone. |

### Dynamic Panel Settings

Configurable via the panel Settings UI or `PUT /api/settings`:

| Setting Key | Default | Description |
| --- | --- | --- |
| `dns_publish` | `false` | Master toggle to enable Cloudflare SRV DNS management. |
| `dns_domain` | *(empty)* | Apex zone domain name (e.g. `example.com`). |
| `dns_zone` | *(empty)* | Cloudflare Zone ID (32-character hexadecimal string). |
| `dns_api_token` | *(empty)* | Cloudflare API token with `Zone.DNS:Edit` permissions. |
| `dns_host` | *(empty)* | Target A/AAAA host pointing to the panel host IP (falls back to `dns_domain`). |
| `dns_service` | `_minecraft` | Service name according to RFC 2782. |
| `dns_proto` | `_tcp` | Protocol name according to RFC 2782. |
| `dns_ttl` | `1` | DNS record TTL in seconds (`1` = Auto). |
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
| `POST` | `/api/servers/:id/console/command` | Send command to server stdin FIFO pipe (with RCON fallback). |
| `GET` | `/api/servers/:id/install` | Get installed version and available upgrade builds. |
| `POST` | `/api/servers/:id/install` | Trigger server version or build upgrade. |
| `GET` | `/api/jars/:kind/versions` | Fetch upstream Minecraft versions for a given server type. |
| `GET` | `/api/jars/:kind/versions/:v/builds` | Fetch builds for a specific version. |
| `GET` | `/api/system/java-versions` | List available Java runtime versions supported by the system. |

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
| `GET` | `/api/servers/:id/properties` | Fetch current parsed `server.properties`. |
| `PUT` | `/api/servers/:id/properties` | Update `server.properties` content. |

### File Manager
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/files` | List files and directories in server volume. |
| `GET` | `/api/servers/:id/files/download` | Download file from server. |
| `GET` | `/api/servers/:id/files/content` | Read text file content for inline editing. |
| `PUT` | `/api/servers/:id/files/content` | Save edited text file content. |
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

### Cloudflare SRV DNS
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/dns` | List all active DNS records and Cloudflare connection status. |
| `POST` | `/api/dns/test` | Test Cloudflare credentials and zone connectivity. |
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
| `GET` | `/api/docker/status` | Get Docker daemon connection and runtime image status. |
| `GET` | `/api/proxy` | Cached and rate-limited reverse proxy for catalog APIs. |
| `POST` | `/api/proxy` | POST requests through external catalog proxy. |
| `GET` | `/healthz` | Liveness health check probe (`200 OK`). |
| `GET` | `/readyz` | Readiness probe checking database and Docker connectivity. |

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
│   ├── dns/                # Cloudflare API v4 integration for RFC 2782 SRV records
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
├── deploy/                 # Systemd service unit for bare metal / LXC installs
├── Dockerfile              # Multi-stage production build (Node -> Go -> Final Alpine)
└── docker-compose.yml      # Docker Compose configuration
```

---

## Development

### Backend (Go)

Ensure Go 1.23+ is installed:

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
