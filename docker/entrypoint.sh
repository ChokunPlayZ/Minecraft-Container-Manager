#!/bin/sh
set -eu

# Container-side directory where MCM keeps persistent data. This must match the
# volume mount target; PUID/PGID control the ownership so the app can write on
# fresh named volumes and host bind mounts.
MCM_DATA_DIR="${MCM_DATA_DIR:-/data}"

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# The default Dockerfile runs as root so we can fix up ownership of the data
# directory (a just-created volume or bind mount is owned by root), then drop
# to the requested UID/GID. Setting `user:` in compose/docker-compose.yaml
# skirts this block entirely and runs the app as whatever user was requested.
if [ "$(id -u)" = "0" ]; then
    mkdir -p "$MCM_DATA_DIR"
    chown -R "${PUID}:${PGID}" "$MCM_DATA_DIR"
    exec su-exec "${PUID}:${PGID}" /usr/local/bin/mcm "$@"
fi

exec /usr/local/bin/mcm "$@"
