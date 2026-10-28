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

Open `https://YOUR_SERVER_IP:8080`. Onboarding creates the first administrator.
Without supplied certificates, MCM saves a self-signed certificate under
`MCM_DATA_DIR/tls`. Trust `cert.pem` on your client or accept the browser warning;
keep `key.pem` private. The installer verifies HTTPS readiness using that public
certificate. Existing installs also switch to HTTPS after updating. For a
TLS-terminating reverse proxy, set `MCM_HTTP_ONLY=true` and `MCM_TLS=true` in the
managed configuration. The optional port-80 redirect is disabled by default.
The host firewall must permit the panel port and any published Minecraft
ports. Change the panel port during installation if necessary:

```sh
./mcmctl install --runtime podman --port 9090
```

### Existing installations

For the first upgrade from a version that defaults to HTTP, run the updated
management script from the source checkout. The previously installed script
only probes HTTP and would reject the new HTTPS listener:

```sh
git pull --ff-only
./mcmctl update --local
```

Later updates can use the installed `mcmctl update` command as usual. Rollback
readiness checks support older HTTP binaries.

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
need into the managed configuration. Leave `MCM_DB_PATH` and
`MCM_DATA_DIR_HOST` empty for native installation so both the database and
server mounts use the host directory selected with `--data-dir`. An explicit
`MCM_DB_PATH` overrides that directory; keep it only if your existing database
is stored separately at that host path. Users and servers are retained in the
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
| Server backup archives and staging files | `MCM_BACKUP_DIR`, default `~/.local/share/mcm/backups` |
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
MCM_DB_PATH=
MCM_DATA_DIR_HOST=
```

The installer writes the socket and data paths for your actual account. Keep
the generated `MCM_SESSION_SECRET` in the file. Available application settings
are listed in the [configuration reference](../README.md#configuration-reference).

An empty `MCM_DB_PATH` uses `MCM_DATA_DIR/mcm.db`. The earlier `.env.example`
set it to `/data/mcm.db`, a path inside the panel container. Copying that value
into a native service overrides its host data directory and can cause
`open database: mkdir /data: permission denied`. Clear the override to use
the configured data directory. The manager checks data and database write
permissions before building or stopping the installed panel.

Changing engines is a migration: stop servers first, change both
`MCM_CONTAINER_RUNTIME` and `MCM_CONTAINER_HOST`, then re-run `install` with
the selected runtime. Existing containers do not move between engines; keep
the data path and let MCM recreate them in the new engine.

### Separate backup media

Local server backups can use a different disk or a mounted network share.
Mount it on the host and give the service user write access to a dedicated
directory, then install with:

```sh
./mcmctl install --runtime podman --backup-dir /mnt/backup-drive/mcm
```

For an existing installation, set `MCM_BACKUP_DIR=/mnt/backup-drive/mcm` with
`./mcmctl config` and restart. Use a literal absolute path. Leaving it empty
keeps the default `MCM_DATA_DIR/backups`. Reinstalling or updating preserves
the configured path; install and update check its write permissions before
building or stopping the panel.

The Backups page's **Local disk** target uses this directory for manual and
automatic backups and uploaded archives. Archives are stored as
`<backup-dir>/<server-id>/<backup-id>.tar.gz`. Temporary archives, including
S3 upload and restore staging, use `<backup-dir>/.tmp`; completed local
archives are moved into place without a second full copy. Database and
server files stay in `MCM_DATA_DIR`.
Choose a backup directory outside server data directories; paths inside the
server being archived are rejected, including paths through symlinks.

Existing archives in `MCM_DATA_DIR/backups` remain available for downloads,
restores, and retention cleanup. To move them, stop the panel and copy the
server-ID directories into the new backup directory, preserving filenames;
the database records need no changes. Keep the database backed up too: the
archive list and server associations are stored there. When changing from
one custom backup path to another, copy those archives to the new path as
well; automatic lookup only checks the current path and the original
`MCM_DATA_DIR/backups` location.

Ensure the media is mounted before starting the panel. For systemd services,
add host mount dependencies as appropriate so a missing mount does not send
backups to the filesystem underneath the mount point. A storage error fails
the backup; writes never retry in the original backup directory.

For a containerized panel, use the
[Compose backup-media override](../README.md#separate-backup-media).

## Update

```sh
./mcmctl update
```

The command fast-forwards the current branch of the selected source checkout,
builds the new binary and frontend, stops the panel briefly, snapshots the
installed version, settings, and SQLite database, then starts the update.
A failed build leaves the current panel running. A failed startup or readiness
check restores the saved version, settings, and database automatically and
reports that the update failed. Minecraft server containers stay running.

Running `./mcmctl update` from a source checkout uses the checkout containing
that script. The installed `mcmctl` command uses the source path saved in
`~/.config/mcm/install.conf`, regardless of your current directory. A successful
update saves its source path so running from the correct checkout also repairs
a stale recorded path. A failed build or startup keeps the previous saved path.

The command prints the source checkout it updates. It uses the branch's
configured upstream; without one, it explicitly pulls
the same branch from `origin` (for example, `origin/main`). To select another
upstream, configure it in the printed source checkout. Updates always require
a fast-forward; a missing remote branch or diverged history stops the update
before building or stopping the panel.

Keep the source checkout at its recorded path, or run `./mcmctl update` from
its new location to record the move. The command refuses to pull
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
MCM's Backups tab with local backup media or S3-compatible storage for those. Database rollback also
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
| `EnvironmentFile= path is not absolute` or `WorkingDirectory= path is not absolute` with a quoted absolute path | An earlier installer put literal quote characters around these directives. Remove those wrapping quotes, reload the unit, and restart using the commands below. |
| `open database: mkdir /data: permission denied` | The native service has a container path in its settings. Set `MCM_DATA_DIR` to the actual host data directory and leave `MCM_DB_PATH` empty to use its `mcm.db`. Leave `MCM_DATA_DIR_HOST` empty as well. Restart the service; no rebuild is needed. |
| Podman socket missing or unavailable | Check `systemctl --user status podman.socket --no-pager -l` and `podman info` as the service user. |
| `unable to parse docker host` followed by `/run/user/.../podman.sock` | The endpoint is a bare path. Current builds accept it; with an older binary, set `MCM_CONTAINER_HOST=unix:///run/user/1000/podman/podman.sock` in `~/.config/mcm/mcm.env`, using your actual UID, then restart the service. |
| `There is no tracking information for the current branch` on update | Current builds fall back to the same branch on `origin`. For older versions, check `SOURCE_DIR` in `~/.config/mcm/install.conf`, then run `git -C /that/source/path branch --set-upstream-to=origin/main main`. Configuring another checkout does not fix the recorded one. |
| Update prints an unexpected nested source path or says `origin` is not a repository while a manual pull works | Run `./mcmctl update` using the script in the checkout you successfully pulled; current versions use and remember that checkout. For older versions, correct `SOURCE_DIR` in `~/.config/mcm/install.conf` to that checkout's absolute path before updating. |
| Build failed | Resolve the reported image-download or build error and retry; an update has not stopped the installed version yet. |
| Update refused because of local changes | Commit/stash them, or intentionally use `update --local`. |
| Panel cannot become ready | Check the printed journal, configured port, data permissions, container API, and runtime image. Default HTTPS probes trust `MCM_DATA_DIR/tls/cert.pem`. Supplied certificates must be trusted by the host and valid for the configured listen host. |
| Service stops after logout | Enable lingering with `sudo loginctl enable-linger "$USER"`. |
| `unknown mount option` or a socket path ending in `}` | That comes from the earlier Compose setup. The managed installer does not require Compose. If retaining Compose, use the updated `podman-compose.yml` with flat defaults and plain socket paths. |

To repair the quoted paths in an existing unit without rebuilding the app:

```sh
sed -i.bak -E 's/^(EnvironmentFile|WorkingDirectory)="(.*)"$/\1=\2/' "$HOME/.config/systemd/user/mcm.service"
systemctl --user daemon-reload
systemctl --user reset-failed mcm.service
systemctl --user restart mcm.service
```

This preserves the original unit as `mcm.service.bak`. `ExecStart` keeps its
command-line quoting; the two whole-path directives use unquoted values.

For macOS or a containerized panel, use the alternative
[Compose setup](../README.md#alternative-docker-compose). The Linux service
manager deliberately requires Linux with systemd.
