# MCM - Minecraft Container Manager

MCM is a self-hosted Minecraft hosting panel. It runs as a small Go service that
talks to the host Docker daemon through a mounted socket and launches sibling
containers for each Minecraft server, using the community
[`itzg/minecraft-server`](https://github.com/itzg/docker-minecraft-server)
image. MCM is a control panel: each server container owns and publishes its own
game port directly, and the panel manages the servers themselves.

## Key features

- Auto jar install: Paper, Fabric, and other server jars are resolved and
  downloaded on first boot by the `itzg/minecraft-server` image, so a fresh
  server starts without manual setup steps.
- One-touch management: create, start, stop, restart, and remove servers from a
  single web UI backed by a JSON API.
- Paper / Fabric support: pick a server type plus version (and build) when
  creating a server.
- S3-compatible backups: export and restore worlds/infrastructure to any
  S3-compatible object store.
- Idle spin-down: automatically stop servers that have been idle for a
  configured period to save resources.
- Cloudflare SRV: optional SRV record registration for domain/port routing.
- TOTP / passkey: multi-factor authentication for panel accounts.
- Onboarding: guided first-run setup and account creation.

> Note: backup scheduling, idle spin-down, Cloudflare SRV, and TOTP/passkey are
> currently stubbed in the backend and wired into configuration only. See
> "Stubbed features" below.

## Quickstart (Docker Compose)

1. Copy the environment template and set a strong session secret:

   ```sh
   cp .env.example .env
   # edit MCM_SESSION_SECRET in .env
   ```

2. Ensure the Minecraft server runtime image the panel launches is available:

   ```sh
   docker pull itzg/minecraft-server
   ```

   The `itzg/minecraft-server` image runs each server container. It is pulled
   automatically on first use; the image name is configurable via
   `MCM_SERVER_IMAGE`.

3. Build and start the panel:

   ```sh
   docker compose up -d --build
   ```

4. Open `http://localhost:8080` and complete onboarding.

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft
server containers on the host. Minecraft servers bind the host port range
configured by `MCM_PORT_RANGE` (default `25565-25665`).

Persistence and permissions are wired through the compose file: `MCM_DATA_DIR_HOST`
is a host directory (default `${PWD}/data`, next to `docker-compose.yml`) mounted
into the container at `/data` and used for the SQLite database and server data.
It is also the host-side path MCM binds into each Minecraft server container.
Set `PUID`/`PGID` to match your host user so the bind-mounted files stay owned
by you, or specify a `user:` on the service to run the panel as a fixed
UID/GID. The image entrypoint fixes up data-directory ownership before the
panel starts.

### Server port exposure (Docker / firewall)

MCM is a control panel, so each server container owns and publishes its own
game port. The panel allocates a `host_port` from the `MCM_PORT_RANGE` pool
(default `25565-25665`) and the server container binds it directly to the host.
The server container publishes its own game port (`25565/tcp`), and additional
ports can be opened per-server from the settings UI (e.g. a WebUI over TCP or a
Bedrock/Geyser adapter over UDP). The panel itself does not proxy player traffic.

- **Host firewall:** open the `host_port` range/individual ports inbound so
  players can reach the server containers directly.
- **Port pool:** the `host_port` values come from the `MCM_PORT_RANGE`
  allocation pool (default `25565-25665`). Only expose the ports actually
  allocated to servers to avoid publishing unused ranges.

## Bare metal / LXC

For hosts that cannot or should not run the panel itself in Docker:

```sh
# Install the mcm binary and the systemd unit.
sudo install -m 0755 ./mcm /usr/local/bin/mcm
sudo install -d /etc/mcm /var/lib/mcm
sudo install -m 0640 deploy/mcm.service /etc/systemd/system/mcm.service

# Create the runtime user and env file.
sudo useradd --system --home /var/lib/mcm --shell /usr/sbin/nologin mcm
sudo install -m 0640 .env.example /etc/mcm/mcm.env
# edit /etc/mcm/mcm.env: set MCM_SESSION_SECRET, adjust paths, etc.

sudo systemctl daemon-reload
sudo systemctl enable --now mcm
```

The unit runs the binary as the `mcm` user with `WorkingDirectory=/var/lib/mcm`
and reads configuration from `/etc/mcm/mcm.env`. It requires `docker.service`
so the socket is available. Ensure the `itzg/minecraft-server` runtime image is
pulled on the host before starting servers.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `MCM_ADDR` | `:8080` | Listen address for the web/API server. |
| `MCM_PORT_RANGE` | `25565-25665` | Host port range allocated to Minecraft servers. |
| `MCM_DATA_DIR_HOST` | `${PWD}/data` | Docker: host directory bind-mounted into the panel at `/data`; also the host-side path MCM binds into each server container. Set an absolute host path to relocate data. |
| `MCM_DATA_DIR` | `/data` (Docker) / `./data` (bare metal) | Process-side data directory the panel reads and writes itself. In Docker the compose file pins this to `/data`. |
| `MCM_DB_PATH` | `$MCM_DATA_DIR/mcm.db` | SQLite database file (container path for Docker, process path for bare metal). |
| `PUID` / `PGID` | `1000` / `1000` | Docker only: UID/GID the panel runs as and that own the data directory. |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker daemon endpoint. |
| `MCM_SESSION_SECRET` | *(required)* | Secret used to sign session cookies. |
| `MCM_TLS_CERT` | *(empty)* | Path to the TLS certificate (PEM). Setting this (with `MCM_TLS_KEY`) enables HTTPS. |
| `MCM_TLS_KEY` | *(empty)* | Path to the TLS private key (PEM). |
| `MCM_TLS_REDIRECT` | `true` | When TLS is enabled, redirect plain HTTP to HTTPS with a 301. |
| `MCM_TLS_REDIRECT_ADDR` | Derived (`:80`) | Listen address for the HTTP-to-HTTPS redirect listener. |
| `MCM_LOGIN_MAX_ATTEMPTS` | `5` | Failed login attempts allowed per window before lockout. |
| `MCM_LOGIN_LOCKOUT` | `15m` | Lockout duration after too many failed login attempts. |
| `MCM_RATE_LIMIT_MAX` | `100` | Maximum state-changing requests per client per window. |
| `MCM_RATE_LIMIT_WINDOW` | `1m` | Sliding window for the general rate limiter. |
| `MCM_DEFAULT_CPU_LIMIT` | `0` | Default CPU cores limit for new servers (0 = unlimited). |
| `MCM_DEFAULT_MEMORY_MB` | `0` | Default memory limit (MB) for new servers (0 = configured server RAM + 2 GiB). |
| `MCM_SERVER_IMAGE` | `itzg/minecraft-server` | Docker image used to run Minecraft server containers. |
| `MCM_S3_ENDPOINT` | *(empty)* | S3-compatible object store endpoint (e.g. `http://minio:9000`). Empty disables backups. |
| `MCM_S3_ACCESS_KEY` | *(empty)* | Access key for the S3 store. |
| `MCM_S3_SECRET_KEY` | *(empty)* | Secret key for the S3 store. |
| `MCM_S3_BUCKET` | *(empty)* | Bucket used to store world backup archives. |
| `MCM_S3_REGION` | `us-east-1` | Region reported during S3 signing. |
| `TZ` | `UTC` | Container/system timezone. |

The idle timeout and Cloudflare credentials for the remaining stubbed features
are intended to be added to this table as those features are implemented.

## API summary

MCM exposes a REST API under `/`. Endpoints are covered in detail by the
backend, but the intended surface is:

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/auth/register` | Create a panel account. |
| `POST` | `/api/auth/login` | Log in and start a session. |
| `GET` | `/api/servers` | List managed Minecraft servers. |
| `POST` | `/api/servers` | Create a server (type, version, build, RAM, ports). |
| `GET` | `/api/servers/:id` | Server detail and status. |
| `POST` | `/api/servers/:id/start` | Start a server. |
| `POST` | `/api/servers/:id/stop` | Stop a server. |
| `POST` | `/api/servers/:id/restart` | Restart a server. |
| `POST` | `/api/servers/:id/recreate` | Detach and rebuild a server's container on next start (used for runtime migrations). |
| `DELETE` | `/api/servers/:id` | Remove a server. |
| `POST` | `/api/servers/:id/backup` | Trigger a backup. |
| `GET` | `/api/servers/:id/backups` | List backups for a server. |
| `POST` | `/api/servers/:id/restore/:backupId` | Restore a backup. |
| `DELETE` | `/api/backups/:backupId` | Delete a backup. |
| `GET` | `/healthz` | Liveness probe, always returns `200`. |
| `GET` | `/readyz` | Readiness probe, checks DB and Docker reachability. |

## Project structure

```text
.
|-- cmd/mcm/            # Go entrypoint
|-- internal/           # Go backend (web assets live in internal/web/dist)
|-- migrations/         # Database migrations
|-- web/                # Frontend (TanStack/Vite)
|-- deploy/             # systemd unit for bare-metal/LXC
|-- Dockerfile          # Multi-stage panel image (web -> go -> runtime)
`-- docker-compose.yml  # Panel orchestration
```

## Development

Backend:

```sh
go build ./...
go test ./...
```

Frontend:

```sh
cd web
pnpm install
pnpm dev     # local dev server
pnpm build   # production build into web/dist
```

The panel Docker image copies the frontend build into `internal/web/dist`
during the multi-stage build, replacing the committed placeholder
`internal/web/dist/index.html`.

## Backups

World backups archive each server's data directory to a tar.gz and upload it to
any S3-compatible object store (MinIO, AWS S3, DigitalOcean Spaces, etc) using
path-style requests signed with AWS Signature Version 4. Retained backups are
limited per server via the `backup_retention` setting (default 10). Scheduling
is configured per server: enable/disable automatic backups and set the interval
in minutes from the server settings.

## Stubbed features

The following are not yet implemented end-to-end and are reserved in the data
model and configuration only: idle spin-down, Cloudflare SRV registration, and
TOTP/passkey authentication. Onboarding, server lifecycle
(create/start/stop/restart/remove), and S3-compatible backups are functional.
