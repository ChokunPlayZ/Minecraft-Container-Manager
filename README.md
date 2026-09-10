# MCM - Minecraft Container Manager

MCM is a modern, high-performance, self-hosted Minecraft server management panel. Built with a lightweight **Go** backend and a responsive **React / Vite** frontend, MCM orchestrates Minecraft servers as sibling Docker containers on the host using the community [`itzg/minecraft-server`](https://github.com/itzg/docker-minecraft-server) image.

Each Minecraft server owns and publishes its game port directly to the host network—delivering zero proxy overhead, native UDP/TCP support (e.g., Geyser/Bedrock), and maximum performance while giving server administrators full lifecycle control from a single web UI.

---

## Key Features

### 🎮 Comprehensive Server Flavor Support
- **Major Server Types**: Out-of-the-box support for **Paper**, **Fabric**, **Vanilla**, **Forge**, **NeoForge**, and **Spigot**.
- **Automated Jar & Version Management**: Resolves and installs Minecraft versions and specific builds automatically on container launch.
- **In-Place Version Switching**: Upgrade or switch server types and builds with built-in version metadata and confirmation safeguards.

### ⚡ Live Console & Command Line
- **Real-Time Streaming**: Live server logs streamed directly to the browser using Server-Sent Events (SSE).
- **Interactive Command Input**: Send console commands directly to the running server via stdin pipe or RCON fallback.
- **Smart Auto-Scroll**: Sticky bottom-scrolling that pauses automatically when inspecting previous logs.

### 🧩 Mod & Plugin Ecosystem Hub
- **Multi-Provider Catalog**: Browse and search tens of thousands of mods and plugins directly within the panel across 4 major repositories:
  - **Modrinth**: Mods, plugins, and datapacks with category filtering, loader detection, and infinite scroll.
  - **PaperMC Hangar**: Curated Paper, Purpur, and Folia plugins.
  - **SpigotMC (via Spiget)**: Over 100,000 Bukkit and Spigot plugins.
  - **CurseForge**: Expansive catalog of mods and server addons.
- **One-Click Installation**: Download compatible jar files directly to the server's `mods` or `plugins` folder.
- **Mod Management**: View installed items, detect installed catalog versions, toggle mods enabled/disabled, or delete files.

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
- **File Transfers**: Upload single or multiple files simultaneously, or download files directly to your machine.
- **Inline Text Editor**: Edit configuration files (`yml`, `json`, `properties`, `toml`, `txt`) directly in the browser.
- **Archive Operations**: Create zip archives and extract zip files in place.
- **Remote Download**: Fetch remote files or datapacks directly from an external URL.

### 🌐 Cloudflare SRV DNS Routing
- **RFC 2782 DNS SRV Management**: Automatically registers, updates, and removes `_minecraft._tcp` SRV records via Cloudflare API v4.
- **Custom Subdomains & Apex Routing**: Route players through custom subdomains (e.g., `survival.example.com`) or apex root domains (`@` -> `example.com`) without exposing non-standard port numbers.
- **One-Click Direct Join Badge**: Displays active join addresses on the server overview with instant clipboard copying.
- **Automatic Lifecycle Cleanup**: Removing a server automatically deregisters its DNS record on Cloudflare.

### 📦 S3-Compatible Backups & Scheduling
- **Object Store Integration**: Archive server data directories to compressed `.tar.gz` archives stored in any S3-compatible object store (AWS S3, MinIO, Cloudflare R2, DigitalOcean Spaces).
- **Automated Scheduling**: Configure background backup intervals per server.
- **Automated Retention**: Enforce retention limits to automatically prune older snapshots.
- **Instant Restore**: One-click in-place snapshot restoration.

### 🔒 Enterprise Security & Authentication
- **Multi-Factor Authentication (MFA)**:
  - **TOTP Two-Factor Authentication**: Authenticator app enrollment with QR code generation.
  - **WebAuthn / Passkeys**: Support for hardware security keys (YubiKey), Apple Touch ID / Face ID, and Windows Hello.
- **Multi-User Administration**: Manage panel user accounts, user creation, password updates, and user deletion.
- **Hardening**: Built-in CSRF protection, brute-force login lockout (5 failed attempts / 15 min), and sliding-window rate limiting.
- **Native TLS Support**: Automatic HTTPS termination with configurable HTTP-to-HTTPS redirect.

### 🔄 Container Lifecycle & Recovery
- **Lifecycle Controls**: Start, Graceful Stop, and Restart.
- **Force-Kill**: Terminate unresponsive or frozen server processes immediately via Docker `SIGKILL`.
- **Container Rebuild**: Recreate sibling containers on demand to apply updated environment variables, port mappings, or fresh base images.

---

## Quickstart (Docker Compose)

1. **Copy the environment template and configure a session secret**:

   ```sh
   cp .env.example .env
   # Edit MCM_SESSION_SECRET in .env
   ```

2. **Ensure the Minecraft server runtime image is available**:

   ```sh
   docker pull itzg/minecraft-server
   ```

   The `itzg/minecraft-server` image is pulled automatically on first use; the image name is configurable via `MCM_SERVER_IMAGE`.

3. **Build and start the panel**:

   ```sh
   docker compose up -d --build
   ```

4. **Access the panel**:
   Open `http://localhost:8080` in your browser and complete the initial onboarding setup.

### Data Persistence & Host Permissions

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft server containers on the host. Minecraft servers bind the host port range configured by `MCM_PORT_RANGE` (default `25565-25665`).

- `MCM_DATA_DIR_HOST`: Host directory (default `${PWD}/data` next to `docker-compose.yml`) mounted into the panel container at `/data` for the SQLite database and server data. This is also the host-side path MCM binds into each Minecraft server container.
- `PUID` / `PGID`: Set to match your host user (e.g. `1000:1000` or `501:20` on macOS) so all files written to the data volume remain owned by your user. The image entrypoint adjusts data-directory permissions before the panel starts.

### Port Exposure & Firewall

MCM is a control panel: each server container owns and publishes its own game port directly to the host network.

- **Host Firewall**: Open the configured `MCM_PORT_RANGE` (default `25565-25665`) on your firewall so players can connect directly.
- **Extra Ports**: Additional ports (TCP/UDP) can be allocated per-server from the Server Settings tab (e.g., Bedrock/Geyser UDP ports, Voice Chat UDP, or Dynmap/BlueMap WebUI ports).
- **Direct Connection**: Because MCM does not proxy game traffic, there is zero protocol lag or proxy overhead.

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
| `MCM_SERVER_IMAGE` | `itzg/minecraft-server` | Docker image used to run Minecraft server containers. |
| `MCM_WEB_AUTHN_RPID` | `localhost` | WebAuthn Relying Party ID (effective domain). |
| `MCM_WEB_AUTHN_RP_ORIGIN` | Derived | Allowed WebAuthn origins (comma-separated, e.g. `https://mc.example.com`). |
| `MCM_WEB_AUTHN_RP_NAME` | `Minecraft Container Manager` | Display name presented during Passkey registration. |
| `MCM_S3_ENDPOINT` | *(empty)* | S3-compatible object store endpoint (e.g. `http://minio:9000`). Empty disables backups. |
| `MCM_S3_ACCESS_KEY` | *(empty)* | Access key for S3 backup storage. |
| `MCM_S3_SECRET_KEY` | *(empty)* | Secret key for S3 backup storage. |
| `MCM_S3_BUCKET` | *(empty)* | S3 bucket used for server backup archives. |
| `MCM_S3_REGION` | `us-east-1` | S3 region for signing requests. |
| `TZ` | `UTC` | Container and server timezone. |

### Dynamic Panel Settings (Cloudflare SRV DNS)

Configurable via the panel Settings UI (`/settings?tab=dns`) or `PUT /api/settings`:

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

---

## API Reference

MCM exposes a clean JSON REST API along with Server-Sent Events (SSE) for console streaming.

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
| `GET` | `/api/servers` | List all managed Minecraft servers. |
| `POST` | `/api/servers` | Create a new server (flavor, version, build, RAM, CPU, ports). |
| `GET` | `/api/servers/:id` | Get server configuration and container status. |
| `PATCH` | `/api/servers/:id` | Update server configuration (RAM, CPU, ports). |
| `DELETE` | `/api/servers/:id` | Delete server and remove container/data. |
| `POST` | `/api/servers/:id/start` | Start server container. |
| `POST` | `/api/servers/:id/stop` | Gracefully stop server container. |
| `POST` | `/api/servers/:id/restart` | Restart server container. |
| `POST` | `/api/servers/:id/kill` | Force-kill unresponsive server (`SIGKILL`). |
| `POST` | `/api/servers/:id/recreate` | Recreate container from image and sync settings. |
| `GET` | `/api/servers/:id/status` | Lightweight polling status endpoint. |
| `GET` | `/api/servers/:id/console` | Stream real-time console logs via Server-Sent Events (SSE). |
| `POST` | `/api/servers/:id/console/command` | Send command to server stdin/RCON. |
| `GET` | `/api/servers/:id/install` | Get installed version and available upgrade builds. |
| `POST` | `/api/servers/:id/install` | Trigger server version or build upgrade. |
| `GET` | `/api/jars/:kind/versions` | Fetch upstream Minecraft versions for a given server type. |
| `GET` | `/api/jars/:kind/versions/:v/builds` | Fetch builds for a specific version. |

### Players, Whitelist & Operators
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/players` | List online players. |
| `POST` | `/api/servers/:id/players/:name/command` | Run player command (kick, ban, pardon, op, give item, etc.). |
| `GET` | `/api/servers/:id/ops` | List operator accounts. |
| `POST` | `/api/servers/:id/ops` | Add an operator with permission level. |
| `DELETE` | `/api/servers/:id/ops/:name` | Remove an operator. |
| `GET` | `/api/servers/:id/whitelist` | List whitelisted players. |
| `POST` | `/api/servers/:id/whitelist` | Add player to whitelist. |
| `DELETE` | `/api/servers/:id/whitelist/:name` | Remove player from whitelist. |

### Mods & Plugins
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/mods` | List installed mods or plugins. |
| `POST` | `/api/servers/:id/mods` | Upload mod/plugin jar file. |
| `POST` | `/api/servers/:id/mods/download` | Download mod/plugin directly from catalog URL. |
| `PATCH` | `/api/servers/:id/mods/:name` | Toggle mod/plugin enabled or disabled. |
| `DELETE` | `/api/servers/:id/mods/:name` | Delete mod/plugin jar. |

### Server Properties
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/properties` | Fetch current `server.properties`. |
| `PUT` | `/api/servers/:id/properties` | Update `server.properties` content. |

### File Manager
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/files` | List files and directories in server path. |
| `GET` | `/api/servers/:id/files/download` | Download a file. |
| `GET` | `/api/servers/:id/files/content` | Read text file content for editing. |
| `PUT` | `/api/servers/:id/files/content` | Save edited text file content. |
| `POST` | `/api/servers/:id/files/upload` | Upload one or more files. |
| `POST` | `/api/servers/:id/files/archive` | Create a zip archive of selected files/folders. |
| `POST` | `/api/servers/:id/files/unzip` | Extract a zip archive in place. |
| `POST` | `/api/servers/:id/files/from_url` | Download a file from a remote URL to server directory. |
| `DELETE` | `/api/servers/:id/files` | Delete file or directory. |
| `POST` | `/api/servers/:id/files/mkdir` | Create a new directory. |
| `POST` | `/api/servers/:id/files/rename` | Rename a file or directory. |

### Backups
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/servers/:id/backups` | List backups for a server. |
| `POST` | `/api/servers/:id/backup` | Trigger an immediate manual backup snapshot. |
| `POST` | `/api/servers/:id/restore/:backupId` | Restore a backup snapshot. |
| `DELETE` | `/api/backups/:backupId` | Delete a backup archive from S3. |

### Cloudflare SRV DNS
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/dns` | List all active DNS records and Cloudflare status. |
| `POST` | `/api/dns/test` | Test Cloudflare credentials and zone connectivity. |
| `GET` | `/api/servers/:id/dns` | Get DNS record status and join address for a server. |
| `POST` | `/api/servers/:id/dns` | Publish or update an SRV record for a server. |
| `DELETE` | `/api/servers/:id/dns` | Remove an SRV record for a server. |

### User Management & System Health
| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/users` | List all registered user accounts. |
| `POST` | `/api/users` | Create a new user account. |
| `PATCH` | `/api/users/:id` | Update user email or password. |
| `DELETE` | `/api/users/:id` | Delete user account and associated passkeys/sessions. |
| `GET` | `/api/ports/available` | Check available host ports in allocation range. |
| `GET` | `/api/settings` | Fetch global panel settings. |
| `PUT` | `/api/settings` | Update global panel settings. |
| `GET` | `/api/docker/status` | Get Docker daemon connection and version info. |
| `GET` | `/healthz` | Liveness health check probe (`200 OK`). |
| `GET` | `/readyz` | Readiness probe checking database and Docker connectivity. |

---

## Project Structure

```text
.
├── cmd/mcm/                # Main Go entrypoint
├── internal/
│   ├── api/                # HTTP routes, middlewares, SSE console, and handlers
│   ├── auth/               # Password hashing, sessions, TOTP, and WebAuthn passkeys
│   ├── backups/            # S3 client, archive engine, and background backup scheduler
│   ├── config/             # Environment variable parsing and defaults
│   ├── db/                 # SQLite connection and migrations runner
│   ├── dns/                # Cloudflare API v4 integration for RFC 2782 SRV records
│   ├── docker/             # Docker engine client and container provisioning
│   ├── jars/               # Paper, Fabric, Vanilla, Forge, NeoForge, Spigot version resolution
│   ├── ports/              # Host port allocation pool manager
│   ├── rcon/               # Minecraft RCON client implementation
│   ├── servers/            # Server entity store and state transitions
│   └── web/                # Embedded production frontend assets
├── migrations/             # SQL schema migrations (SQLite)
├── web/                    # React / Vite frontend application
│   ├── src/
│   │   ├── api/            # API client and TypeScript interfaces
│   │   ├── components/     # UI components (Console, File Manager, Mod Catalog, MOTD, etc.)
│   │   ├── routes/         # TanStack Router page routes
│   │   └── styles/         # CSS and theme tokens
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

- Server containers are powered by the incredible [`itzg/docker-minecraft-server`](https://github.com/itzg/docker-minecraft-server).
- Mod & plugin catalog integrations powered by [Modrinth](https://modrinth.com/), [PaperMC Hangar](https://hangar.papermc.io/), [SpigotMC / Spiget](https://spiget.org/), and [CurseForge](https://curseforge.com/).
- Player head avatars provided by [Minotar](https://minotar.net/).
