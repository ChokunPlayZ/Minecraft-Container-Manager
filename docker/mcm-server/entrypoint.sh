#!/bin/sh
set -eu

SERVER_TYPE="${SERVER_TYPE:-paper}"
VERSION="${VERSION:-latest}"
BUILD="${BUILD:-}"
RAM_MB="${RAM_MB:-2048}"

SERVER_JAR=/data/server.jar

log() {
    echo "[mcm-server] $*"
}

download() {
    url="$1"
    log "Downloading $url"
    curl -fsSL --retry 3 --retry-delay 2 -o "${SERVER_JAR}.tmp" "$url"
    mv "${SERVER_JAR}.tmp" "$SERVER_JAR"
    log "Downloaded $(wc -c < "$SERVER_JAR") bytes to server.jar"
}

resolve_paper() {
    if [ "$VERSION" = "latest" ]; then
        mc=$(curl -fsSL "https://api.papermc.io/v2/projects/paper" \
            | jq -r '.versions[-1]')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest Paper version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    if [ -n "$BUILD" ]; then
        build="$BUILD"
    else
        build=$(curl -fsSL "https://api.papermc.io/v2/projects/paper/versions/${mc}/builds" \
            | jq -r '.builds[-1].build' 2>/dev/null)
        [ -n "$build" ] && [ "$build" != "null" ] || { log "Could not resolve Paper build for $mc" >&2; exit 1; }
    fi

    log "Resolved Paper build: $build (version: $mc)"
    download "https://api.papermc.io/v2/projects/paper/versions/${mc}/builds/${build}/downloads/paper-${mc}-${build}.jar"
}

resolve_fabric() {
    if [ "$VERSION" = "latest" ]; then
        mc=$(curl -fsSL "https://meta.fabricmc.net/v2/versions/game" \
            | jq -r '[.[] | select(.stable == true)] | .[0].version')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest Fabric version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    if [ -n "$BUILD" ]; then
        loader="$BUILD"
    else
        loader=$(curl -fsSL "https://meta.fabricmc.net/v2/versions/loader/${mc}" \
            | jq -r '[.[] | select(.stable == true)] | .[0].version // empty' 2>/dev/null)
        [ -n "$loader" ] || { log "Could not resolve Fabric loader for $mc" >&2; exit 1; }
    fi

    log "Resolved Fabric loader: $loader (MC: $mc)"
    download "https://meta.fabricmc.net/v2/versions/loader/${mc}/${loader}/1.0.0/server/jar"
}

resolve_vanilla() {
    mc="$VERSION"

    manifest=$(curl -fsSL "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json")
    if [ "$VERSION" = "latest" ]; then
        mc=$(printf '%s' "$manifest" | jq -r '.latest.release')
    fi

    version_url=$(printf '%s' "$manifest" \
        | jq -r --arg mc "$mc" '.versions[] | select(.id == $mc and .type == "release") | .url' \
        | head -n1)
    [ -n "$version_url" ] || { log "Minecraft version '$mc' not found" >&2; exit 1; }

    jar_url=$(curl -fsSL "$version_url" | jq -r '.downloads.server.url // empty')
    [ -n "$jar_url" ] || { log "Could not resolve vanilla server jar for $mc" >&2; exit 1; }

    log "Resolved vanilla server jar for $mc"
    download "$jar_url"
}

if [ ! -f "$SERVER_JAR" ]; then
    case "$SERVER_TYPE" in
        paper)   resolve_paper ;;
        fabric)  resolve_fabric ;;
        vanilla) resolve_vanilla ;;
        *)
            log "Unknown SERVER_TYPE '$SERVER_TYPE'; expected paper|fabric|vanilla" >&2
            exit 1
            ;;
    esac
else
    log "server.jar already present; skipping download"
fi

# Accept the Minecraft EULA when not already accepted.
if [ ! -f /data/eula.txt ] || ! grep -qi '^eula=true' /data/eula.txt; then
    printf 'eula=true\n' > /data/eula.txt
    log "Accepted EULA in /data/eula.txt"
fi

log "Starting server: type=$SERVER_TYPE ram=${RAM_MB}M"
exec java -Xms512M -Xmx"${RAM_MB}"M -jar "$SERVER_JAR" nogui
