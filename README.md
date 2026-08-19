# MCM - Minecraft Container Manager

MCM is a self-hosted Minecraft hosting panel. It runs as a small Go service that
talks to the host Docker daemon through a mounted socket and launches sibling
containers for each Minecraft server, using the bundled `mcm-server:latest`
image.

## Key features

- Auto jar install: Paper, Fabric, and vanilla server jars are downloaded on
  first boot by `docker/mcm-server`, so a fresh server starts without manual
  setup steps.
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

2. Build the Minecraft server image that the panel launches:

   ```sh
   docker build -t mcm-server:latest docker/mcm-server
   ```

   This image is used by the panel to run sibling Minecraft server containers.
   It is intentionally built separately from the panel image.

3. Build and start the panel:

   ```sh
   docker compose up -d --build
   ```

4. Open `http://localhost:8080` and complete onboarding.

The panel container mounts `/var/run/docker.sock` so it can manage Minecraft
server containers on the host. Minecraft servers bind the host port range
configured by `MCM_PORT_RANGE` (default `25565-25665`).

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
so the socket is available. Build the `mcm-server:latest` image with the
`docker build -t mcm-server:latest docker/mcm-server` command above.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `MCM_ADDR` | `:8080` | Listen address for the web/API server. |
| `MCM_PORT_RANGE` | `25565-25665` | Host port range allocated to Minecraft servers. |
| `MCM_DATA_DIR` | `/data` | Root directory for persistent data. |
| `MCM_DB_PATH` | `/data/mcm.db` | SQLite database file. |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Docker daemon endpoint. |
| `MCM_SESSION_SECRET` | *(required)* | Secret used to sign session cookies. |
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
| `DELETE` | `/api/servers/:id` | Remove a server. |
| `POST` | `/api/servers/:id/backup` | Trigger a backup. |
| `GET` | `/api/servers/:id/backups` | List backups for a server. |
| `POST` | `/api/servers/:id/restore/:backupId` | Restore a backup. |
| `DELETE` | `/api/backups/:backupId` | Delete a backup. |

## Project structure

```text
.
|-- cmd/mcm/            # Go entrypoint
|-- internal/           # Go backend (web assets live in internal/web/dist)
|-- migrations/         # Database migrations
|-- web/                # Frontend (TanStack/Vite)
|-- docker/mcm-server/  # Custom Minecraft server image (entrypoint + Dockerfile)
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

## mcm-server image

`docker/mcm-server` provides the sibling Minecraft runtime. It is an Eclipse
Temurin 21 JRE image with `curl` and `jq`. On first boot the entrypoint
downloads a server jar (Paper by default, or Fabric/vanilla), writes
`eula.txt`, and execs Java with the configured RAM. Server files persist in the
`/data` volume.

Environment variables for the server container:

| Variable | Default | Description |
| --- | --- | --- |
| `SERVER_TYPE` | `paper` | `paper`, `fabric`, or `vanilla`. |
| `VERSION` | `latest` | Minecraft version to resolve. |
| `BUILD` | *(empty)* | Specific Paper build or Fabric loader version. |
| `RAM_MB` | `2048` | Max heap in MB (min heap is 512M). |
| `EULA` | `TRUE` | Set `EULA=TRUE` to accept the Minecraft EULA. |

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
