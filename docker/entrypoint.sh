#!/bin/sh
set -eu

# Container-side directory where MCM keeps persistent data. This must match the
# volume mount target; PUID/PGID control the ownership so the app can write on
# fresh named volumes and host bind mounts.
MCM_DATA_DIR="${MCM_DATA_DIR:-/data}"
MCM_BACKUP_DIR="${MCM_BACKUP_DIR:-$MCM_DATA_DIR/backups}"

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# The default Dockerfile runs as root so we can fix up ownership of the data
# directory (a just-created volume or bind mount is owned by root), then drop
# to the requested UID/GID. Setting `user:` in compose/docker-compose.yaml
# skirts this block entirely and runs the app as whatever user was requested.
if [ "$(id -u)" = "0" ]; then
    mkdir -p "$MCM_DATA_DIR"
    chown -R "${PUID}:${PGID}" "$MCM_DATA_DIR"
    if [ "$MCM_BACKUP_DIR" != "$MCM_DATA_DIR/backups" ]; then
        mkdir -p "$MCM_BACKUP_DIR"
        chown -R "${PUID}:${PGID}" "$MCM_BACKUP_DIR"
    fi
    # su-exec sets exactly one group. Use the mounted runtime socket's group
    # as the process group; PUID still owns the data files prepared above.
    runtime_gid="$PGID"
    runtime_endpoint="${MCM_CONTAINER_HOST:-${DOCKER_HOST:-unix:///var/run/docker.sock}}"
    case "$runtime_endpoint" in
        unix://*)
            runtime_socket="${runtime_endpoint#unix://}"
            if [ -S "$runtime_socket" ]; then
                runtime_gid=$(stat -c '%g' "$runtime_socket")
            fi
            ;;
    esac
    exec su-exec "${PUID}:${runtime_gid}" /usr/local/bin/mcm "$@"
fi

exec /usr/local/bin/mcm "$@"
