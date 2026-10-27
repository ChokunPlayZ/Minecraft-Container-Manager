# MCM - Minecraft Container Manager

MCM is a modern, high-performance, self-hosted Minecraft server management panel. Built with a lightweight **Go** backend and a responsive **React / Vite** frontend, MCM orchestrates Minecraft servers as sibling **Docker or Podman** containers on the host using lightweight, security-hardened **Eclipse Temurin Alpine OpenJDK/JRE** images.

Each Minecraft server container runs with MCM's custom native lifecycle entrypoint script (`DefaultEntryScript`), providing interactive console FIFO pipes, graceful shutdown traps (`SIGTERM`/`SIGINT`), headless Java configuration, and automated installer execution for Forge and NeoForge. MCM supports both direct host-port publication (for zero-proxy overhead, raw performance, and UDP/TCP support) and private container networking (`mcm-network`) with optional host ports for seamless multi-server proxy setups (e.g., Velocity or BungeeCord).

## Install, manage, and update (Linux)

Use the included `mcmctl` command on a Linux host with systemd. Run it as your
normal user. It builds the complete panel with Podman or Docker, creates its
configuration and session secret, installs a user service, and starts the app.
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
./mcmctl install --runtime podman
```

Open `http://YOUR_SERVER_IP:8080` and complete onboarding. Open port 8080 and
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
| Edit settings | `./mcmctl config`, then `./mcmctl restart` |
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
live in `~/.local/share/mcm`. The installer enables the user service and tries
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
- **1-Click Container Rebuild**: Recreate sibling containers on demand without data loss to apply updated configuration, memory limits, or base images. If online, the server is gracefully shut down first to cleanly save world data, the container is recreated, and it is automatically booted back up.
- **Instant Server Duplication / Cloning**: 1-click server cloning (`POST /api/servers/:id/copy`) that duplicates server data, configurations, mods, and plugins while automatically allocating fresh host ports.

### ⚡ Live Console & Command Line
- **Real-Time Streaming**: Live server logs streamed directly to the browser using Server-Sent Events (SSE).
- **Last-Event-ID Resumption**: Seamlessly reconnects to log streams after network hiccups without duplicate messages or log gaps.
- **Interactive Command Input**: Send console commands directly to the running server via native FIFO stdin pipe (`/tmp/console.in`) with RCON fallback.
- **Smart Auto-Scroll**: Sticky bottom-scrolling that pauses automatically when scrolling up to inspect past logs.
- **Saved Log Browser**: Open the server's Logs tab to read files from `logs/` and `crash-reports/`, including `.log.gz` rotations and logs inside `.tar`, `.tar.gz`, and `.tgz` archives, without extracting them. Search, line numbers, wrapping, and pagination help inspect saved output. Previews show the first 2 MiB; search applies to that preview.

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
- **Dedicated Backup Manager**: Open Backups to select a server, manage archives, search by name, filter by local or S3 storage, and save its automatic backup interval. The server's Backups tab remains available.
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
   Open `http://localhost:8080` in your browser and complete the initial onboarding setup.

### Data Persistence & Host Permissions

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft server containers on the host. Minecraft servers bind host ports within `MCM_PORT_RANGE` (default `25565-25665`).

- `MCM_DATA_DIR_HOST`: Host directory (default `${PWD}/data` next to `docker-compose.yml`) mounted into the panel container at `/data` for the SQLite database and server data. This is also the host-side path MCM binds into each Minecraft server container.
- `PUID` / `PGID`: Set to match your host user (e.g. `1000:1000` or `501:20` on macOS) so all files written to the data volume remain owned by your user. The image entrypoint adjusts data-directory permissions before the panel starts.

New Minecraft containers inherit the panel process's UID/GID. Their startup step prepares required tools and repairs ownership of `/data` as root, then drops privileges before running server installers or Java. This keeps automatically generated mod configs editable in the panel. Set `PUID` to a nonzero UID to use this behavior. Custom server images must provide `su-exec` or `gosu`, or an Alpine/Debian package manager to install one.

After upgrading from a version that ran Minecraft as root, use **Rebuild container** for each existing server. Rebuilding preserves its data, replaces the old container, and applies the ownership repair at its next start. Restarting the old container alone does not update its startup settings. This runs Minecraft as a non-root user; running the Docker daemon itself in rootless mode is a separate host configuration.

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
| `MCM_DB_PATH` | `$MCM_DATA_DIR/mcm.db` | SQLite database file location. |
| `DOCKER_HOST` | Runtime-specific socket | API endpoint for Docker or Podman. Docker defaults to `unix:///var/run/docker.sock`; Podman defaults are described above. |
| `MCM_CONTAINER_RUNTIME` | `docker` | Container engine: `docker` or `podman`. |
| `MCM_CONTAINER_HOST` | *(empty)* | API endpoint override for either engine; takes priority over `DOCKER_HOST`. |
| `CONTAINER_HOST` | *(empty)* | Podman endpoint fallback when `MCM_CONTAINER_HOST` and `DOCKER_HOST` are unset. |
| `MCM_PODMAN_SOCKET` | `/run/podman/podman.sock` | Absolute host socket path mounted by `podman-compose.yml`, without `unix://`; set explicitly for rootless or VM deployments. |
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
| `GET` | `/api/servers/:id/logs` | List saved server logs; pass `?path=logs/archive.tar.gz` to list log members in a tar archive. |
| `GET` | `/api/servers/:id/logs/content` | Preview a plain or gzip-compressed log using `?path=...`; add `&entry=...` for a tar archive member. Returns content, truncation state, and preview limit. |
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
