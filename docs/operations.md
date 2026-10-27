# Install and operate MCM

## Install on Linux

Run as your normal user on a Linux host with systemd, Bash, Git, curl, and
`flock` (from `util-linux`). Install either Podman or Docker. The selected
engine builds the complete app, including the web interface, so you do not
need Go, Node.js, pnpm, or a Compose provider on the host.

Podman runs rootless. The user needs subordinate UID/GID ranges and a usable
container engine. Distribution packages normally configure these for regular
users. Resource limits need cgroup v2 and delegated controllers. Confirm
`podman info` works as your user before installing.

```sh
git clone https://github.com/ChokunPlayZ/Minecraft-Container-Manager.git
cd Minecraft-Container-Manager
./mcmctl install --runtime podman
```

For Docker, use `./mcmctl install --runtime docker` as a user who can access
the daemon. Do not run the installer with `sudo`.

Installation builds the app before replacing the running binary, enables
Podman's user socket when selected, creates a persistent random session secret,
downloads the initial Minecraft runtime image if missing, and starts the user
service. It waits for the database, runtime API, and runtime image to be ready.

Open `http://YOUR_SERVER_IP:8080`. Onboarding creates the first administrator.
The host firewall must permit the panel port and any published Minecraft
ports. Change the panel port during installation if necessary:

```sh
./mcmctl install --runtime podman --port 9090
```

### Existing installations

Re-running `install` preserves existing settings and session secrets. It also
repairs the earlier user-service setup that failed with
`Failed to load environment files`: configuration is created before starting
the service. The installed binary is `~/.local/bin/mcm`; the earlier
`/usr/local/bin/mcm` is no longer used by the managed service.

For existing data, specify its actual directory:

```sh
./mcmctl install --runtime podman --data-dir /absolute/path/to/existing/data
```

If a Compose panel currently uses that data, stop its panel container first:

```sh
podman stop mcm
```

Then install with `--data-dir` pointing at its `/data` mount's host directory.
Keep the same container engine and user account so MCM can still find existing
Minecraft containers. Stop any other panel process using the database. The
installer does not import the Compose `.env`; copy any additional settings you
need into the managed configuration. Users and servers are retained in the
existing database; a new session secret requires signing in again.

### Automatic startup

The installer enables `mcm.service` and attempts to enable user lingering.
Lingering lets the service run after logout and start at boot. If your host
requires administrator authorization, finish with:

```sh
sudo loginctl enable-linger "$USER"
```

Use `./mcmctl disable` to turn off automatic startup while keeping the panel
running, or `./mcmctl enable` to turn it back on. Use `stop` to stop it now.

## Manage the panel

The commands work through `./mcmctl` in the checkout and through the installed
`~/.local/bin/mcmctl` from any directory. If `~/.local/bin` is on your shell's
`PATH`, the installed command is simply `mcmctl`.

```sh
./mcmctl status
./mcmctl logs
./mcmctl restart
./mcmctl stop
./mcmctl start
```

`logs` follows the service journal; press Ctrl+C to return to your shell.
Stopping the panel leaves Minecraft server containers running. Stop those
servers through the panel before host maintenance that requires them offline.

## Settings and file locations

| What | Default location |
| --- | --- |
| Persistent settings and session secret | `~/.config/mcm/mcm.env` |
| Source-checkout location for updates | `~/.config/mcm/install.conf` |
| Panel database and server files | `~/.local/share/mcm` |
| Installed binary and management command | `~/.local/bin/mcm`, `~/.local/bin/mcmctl` |
| User service | `~/.config/systemd/user/mcm.service` |
| Panel update backups | `~/.config/mcm/backups` |

`XDG_CONFIG_HOME` and `XDG_DATA_HOME` are honored during installation. Set
`MCM_BIN_DIR` before running the manager if you use another directory for
installed commands. Continue using the same values for later management.

Edit application settings and restart:

```sh
./mcmctl config
./mcmctl restart
```

The editor comes from `EDITOR`, falling back to `vi`. Settings use one
`KEY=value` assignment per line. Use literal absolute file paths; systemd does
not expand `$HOME`, `${PWD}`, or shell commands in this file. Example:

```ini
MCM_ADDR=:8080
MCM_PORT_RANGE=25565-25665
MCM_CONTAINER_RUNTIME=podman
MCM_CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock
MCM_DATA_DIR=/home/chokun/.local/share/mcm
```

The installer writes the socket and data paths for your actual account. Keep
the generated `MCM_SESSION_SECRET` in the file. Available application settings
are listed in the [configuration reference](../README.md#configuration-reference).

Changing engines is a migration: stop servers first, change both
`MCM_CONTAINER_RUNTIME` and `MCM_CONTAINER_HOST`, then re-run `install` with
the selected runtime. Existing containers do not move between engines; keep
the data path and let MCM recreate them in the new engine.

## Update

```sh
./mcmctl update
```

The command fast-forwards the current branch of the original source checkout,
builds the new binary and frontend, stops the panel briefly, snapshots the
installed version, settings, and SQLite database, then starts the update.
A failed build leaves the current panel running. A failed startup or readiness
check restores the saved version, settings, and database automatically and
reports that the update failed. Minecraft server containers stay running.

Keep the source checkout at its recorded path. The command refuses to pull
over local edits. Commit or stash those edits before updating. To deliberately
build the current local checkout without fetching or changing Git files:

```sh
./mcmctl update --local
```

Updates preserve the data directory, administrator accounts, server records,
and existing configuration. If the service was intentionally stopped, `update`
starts it after the build.

## Back up and roll back

```sh
./mcmctl backup
./mcmctl rollback
```

`backup` briefly stops the panel to copy its SQLite database (including any
remaining WAL files), settings, service unit, and installed commands. It
returns the panel to its previous running or stopped state. Every backup
prints its directory; the most recent is the rollback target.

`rollback` restores that target and starts the panel. The source checkout
stays at its current Git revision. A later update builds that checkout again.
Do not use rollback after intentionally changing databases or moving data
without first checking the saved paths.

These panel snapshots **do not include worlds, mods, or server files**. Use
MCM's Backups tab and S3-compatible storage for those. Database rollback also
undoes panel changes made after that snapshot, such as users or server records.
Keep copies of important backups on another machine.

## Uninstall

```sh
./mcmctl uninstall
```

This stops and disables the panel service and removes its unit and installed
commands. It retains settings, backups, server files, and Minecraft containers.
Stop Minecraft servers first if you also want them offline. Podman/Docker and
their other workloads are left in place. Reinstall using the same data path
to resume management.

## Troubleshoot

```sh
./mcmctl status
./mcmctl logs
```

For a finite log excerpt:

```sh
journalctl --user -u mcm.service -n 50 --no-pager
```

| Symptom | Action |
| --- | --- |
| `Failed to load environment files` | Re-run `./mcmctl install --runtime podman` from the updated checkout. It creates the required settings before starting. |
| `203/EXEC` or missing executable | Re-run `install`; it builds and installs the complete binary and rewrites the user unit. |
| Podman socket missing or unavailable | Check `systemctl --user status podman.socket --no-pager -l` and `podman info` as the service user. |
| Build failed | Resolve the reported image-download or build error and retry; an update has not stopped the installed version yet. |
| Update refused because of local changes | Commit/stash them, or intentionally use `update --local`. |
| Panel cannot become ready | Check the printed journal, configured port, data permissions, container API, and runtime image. HTTPS installations need a certificate trusted by the host and valid for the configured listen host. |
| Service stops after logout | Enable lingering with `sudo loginctl enable-linger "$USER"`. |
| `unknown mount option` or a socket path ending in `}` | That comes from the earlier Compose setup. The managed installer does not require Compose. If retaining Compose, use the updated `podman-compose.yml` with flat defaults and plain socket paths. |

For macOS or a containerized panel, use the alternative
[Compose setup](../README.md#alternative-docker-compose). The Linux service
manager deliberately requires Linux with systemd.
