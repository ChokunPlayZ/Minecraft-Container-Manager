#!/bin/sh
set -eu

SERVER_TYPE="${SERVER_TYPE:-paper}"
VERSION="${VERSION:-latest}"
BUILD="${BUILD:-}"
RAM_MB="${RAM_MB:-2048}"
DATA_DIR="${MCM_DATA_DIR:-/data}"

SERVER_JAR="${DATA_DIR}/server.jar"
LAUNCH=jar

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
        # Fill groups versions by version group, most recent first; the first
        # entry of the newest group is the latest release.
        mc=$(curl -fsSL "https://fill.papermc.io/v3/projects/paper" \
            | jq -r '.versions | to_entries | .[0].value[0]')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest Paper version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    builds_json=$(curl -fsSL "https://fill.papermc.io/v3/projects/paper/versions/${mc}/builds")

    if [ -n "$BUILD" ]; then
        build="$BUILD"
    else
        build=$(printf '%s' "$builds_json" | jq -r '.[0].id' 2>/dev/null)
        [ -n "$build" ] && [ "$build" != "null" ] || { log "Could not resolve Paper build for $mc" >&2; exit 1; }
    fi

    url=$(printf '%s' "$builds_json" | jq -r --arg build "$build" \
        '.[] | select(.id == ($build | tonumber)) | .downloads["server:default"].url // empty' | head -n1)
    [ -n "$url" ] || { log "Could not resolve Paper download for $mc (build $build)" >&2; exit 1; }

    log "Resolved Paper build: $build (version: $mc)"
    download "$url"
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

resolve_spigot() {
    if [ "$VERSION" = "latest" ]; then
        mc=$(curl -fsSL "https://hub.spigotmc.org/versions/" \
            | jq -r 'keys | .[-1]')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest Spigot version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    log "Resolved Spigot version: $mc"
    download "https://download.getbukkit.org/spigot/spigot-${mc}.jar"
}

resolve_forge() {
    if [ "$VERSION" = "latest" ]; then
        mc=$(curl -fsSL "https://files.minecraftforge.net/net/minecraftforge/forge/maven-metadata.json" \
            | jq -r 'keys | .[-1]')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest Forge version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    if [ -n "$BUILD" ]; then
        forge="$BUILD"
    else
        forge=$(curl -fsSL "https://files.minecraftforge.net/net/minecraftforge/forge/maven-metadata.json" \
            | jq -r --arg mc "$mc" '.[$mc][-1].version // empty' 2>/dev/null)
        [ -n "$forge" ] || { log "Could not resolve Forge build for $mc" >&2; exit 1; }
    fi

    log "Resolved Forge: $forge (MC: $mc)"
    install_forge_installer "$forge"
}

resolve_neoforge() {
    if [ "$VERSION" = "latest" ]; then
        mc=$(curl -fsSL "https://api.neoforged.net/neoforges/releases" \
            | jq -r '[.versions[] | split("-")[0]] | unique | .[-1]')
        [ -n "$mc" ] && [ "$mc" != "null" ] || { log "Could not resolve latest NeoForge version" >&2; exit 1; }
    else
        mc="$VERSION"
    fi

    if [ -n "$BUILD" ]; then
        nf="$BUILD"
    else
        nf=$(curl -fsSL "https://api.neoforged.net/neoforges/releases" \
            | jq -r --arg pre "$mc-" '[.versions[] | select(startswith($pre))][-1] // empty' 2>/dev/null)
        [ -n "$nf" ] || { log "Could not resolve NeoForge build for $mc" >&2; exit 1; }
    fi

    log "Resolved NeoForge: $nf"
    install_forge_installer "$nf" neoforge
}

install_forge_installer() {
    ver="$1"
    case "$2" in
        neoforge)
            base="https://maven.neoforged.net/releases/net/neoforged/neoforge"
            base_url="$base/${ver}/neoforge-${ver}-installer.jar"
            ;;
        *)
            base="https://maven.minecraftforge.net/net/minecraftforge/forge"
            base_url="$base/${ver}/forge-${ver}-server.jar"
            ;;
    esac

    log "Downloading installer from $base_url"
    curl -fsSL --retry 3 --retry-delay 2 -o "${DATA_DIR}/installer.jar" "$base_url"
    ( cd "$DATA_DIR" && java -jar installer.jar --installServer )
    rm -f "${DATA_DIR}/installer.jar"
    if [ -f "${DATA_DIR}/run.sh" ]; then
        chmod +x "${DATA_DIR}/run.sh"
        LAUNCH=forge
        log "Installed $ver; will launch via run.sh"
    else
        log "Installer completed but no run.sh found; falling back to direct jar run" >&2
        LAUNCH=jar
    fi
}

server_installed() {
    case "$SERVER_TYPE" in
        forge|neoforge) [ -f "${DATA_DIR}/run.sh" ] ;;
        *) [ -f "$SERVER_JAR" ] ;;
    esac
}

if server_installed; then
    log "Server already installed; skipping download"
else
    case "$SERVER_TYPE" in
        paper)   resolve_paper ;;
        fabric)  resolve_fabric ;;
        vanilla) resolve_vanilla ;;
        spigot)  resolve_spigot ;;
        forge)   resolve_forge ;;
        neoforge) resolve_neoforge ;;
        *)
            log "Unknown SERVER_TYPE '$SERVER_TYPE'; expected paper|fabric|vanilla|spigot|forge|neoforge" >&2
            exit 1
            ;;
    esac
fi

# Accept the Minecraft EULA when not already accepted.
if [ ! -f "${DATA_DIR}/eula.txt" ] || ! grep -qi '^eula=true' "${DATA_DIR}/eula.txt"; then
    printf 'eula=true\n' > "${DATA_DIR}/eula.txt"
    log "Accepted EULA in ${DATA_DIR}/eula.txt"
fi

log "Starting server: type=$SERVER_TYPE ram=${RAM_MB}M"
if [ "$LAUNCH" = "forge" ] && [ -f "${DATA_DIR}/run.sh" ]; then
    exec sh "${DATA_DIR}/run.sh" nogui
fi
exec java -Xms512M -Xmx"${RAM_MB}"M -jar "$SERVER_JAR" nogui
