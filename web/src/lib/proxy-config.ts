export interface ProxyBackendServer {
  name: string;
  address: string;
  motd?: string;
  restricted?: boolean;
}

export type PlayerForwardingMode = 'modern' | 'bungeeguard' | 'legacy' | 'none';

export interface ProxyConfig {
  bind: string;
  motd: string;
  showMaxPlayers: number;
  onlineMode: boolean;
  playerInfoForwardingMode: PlayerForwardingMode;
  forwardingSecret: string;
  forceDefaultServer: boolean;
  servers: ProxyBackendServer[];
  tryServers: string[];
}

export const DEFAULT_VELOCITY_TOML = `# Velocity Proxy Configuration
config-version = "2.7"
bind = "0.0.0.0:25577"
motd = "<#09add3>A Velocity Proxy"
show-max-players = 500
online-mode = true
force-key-authentication = true
player-info-forwarding-mode = "modern"
forwarding-secret = ""

# Registered backend servers
# Containers within the panel network can be referenced as "mcm-<server-id>:25565"
[servers]

# Priority fallback servers when players connect
try = []

[forced-hosts]
`;

export const DEFAULT_BUNGEE_YAML = `# BungeeCord / Waterfall Proxy Configuration
listeners:
- query_port: 25577
  motd: '&1A BungeeCord Proxy'
  tab_list: GLOBAL_PING
  query_enabled: false
  proxy_protocol: false
  forced_hosts: {}
  ping_passthrough: false
  priorities: []
  bind_local_address: true
  host: 0.0.0.0:25577
  max_players: 500
  tab_size: 60
  force_default_server: false
remote_ping_timeout: 5000
network_compression_threshold: 256
permissions:
  default:
  - bungeecord.command.server
  - bungeecord.command.list
  admin:
  - bungeecord.command.alert
  - bungeecord.command.end
  - bungeecord.command.ip
  - bungeecord.command.reload
log_pings: true
connection_throttle_limit: 4000
server_connect_timeout: 5000
timeout: 30000
player_limit: -1
prevent_proxy_connections: false
ip_forward: true
servers: {}
`;

/**
 * Extracts the MCM server ID from an address like "mcm-1234-5678:25565"
 */
export function extractContainerId(address: string): string | null {
  const match = address.match(/mcm-([a-f0-9-]+)(?::\d+)?/i);
  return match ? match[1] : null;
}

/**
 * Generates a cryptographically secure random secret for modern player forwarding.
 */
export function generateForwardingSecret(): string {
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const bytes = new Uint8Array(16);
    crypto.getRandomValues(bytes);
    return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  }
  return Math.random().toString(36).substring(2) + Math.random().toString(36).substring(2);
}

/**
 * Parses Velocity's velocity.toml format into structured ProxyConfig.
 */
export function parseVelocityToml(content: string): ProxyConfig {
  const lines = content.split('\n');
  const cfg: ProxyConfig = {
    bind: '0.0.0.0:25577',
    motd: 'A Velocity Proxy',
    showMaxPlayers: 500,
    onlineMode: true,
    playerInfoForwardingMode: 'modern',
    forwardingSecret: '',
    forceDefaultServer: false,
    servers: [],
    tryServers: [],
  };

  let currentSection = '';
  let inTryBlock = false;

  for (let i = 0; i < lines.length; i++) {
    let line = lines[i].trim();
    if (!line || line.startsWith('#')) continue;

    // Remove inline comments if not inside quotes
    if (line.includes('#') && !line.includes('"#')) {
      const idx = line.indexOf('#');
      line = line.substring(0, idx).trim();
    }

    if (line.startsWith('[') && line.endsWith(']')) {
      currentSection = line.slice(1, -1).trim();
      inTryBlock = false;
      continue;
    }

    if (currentSection === '') {
      if (line.startsWith('bind')) {
        const val = extractTomlValue(line);
        if (val) cfg.bind = val;
      } else if (line.startsWith('motd')) {
        const val = extractTomlValue(line);
        if (val) cfg.motd = val;
      } else if (line.startsWith('show-max-players')) {
        const val = extractTomlValue(line);
        if (val) cfg.showMaxPlayers = parseInt(val, 10) || 500;
      } else if (line.startsWith('online-mode')) {
        cfg.onlineMode = extractTomlValue(line) === 'true';
      } else if (line.startsWith('player-info-forwarding-mode')) {
        const val = extractTomlValue(line).toLowerCase();
        if (val === 'modern' || val === 'bungeeguard' || val === 'legacy' || val === 'none') {
          cfg.playerInfoForwardingMode = val;
        }
      } else if (line.startsWith('forwarding-secret')) {
        cfg.forwardingSecret = extractTomlValue(line);
      }
    } else if (currentSection === 'servers') {
      if (line.startsWith('try')) {
        inTryBlock = true;
        const inlineArray = extractTomlArray(line);
        if (inlineArray.length > 0) {
          cfg.tryServers = inlineArray;
          if (line.includes(']')) inTryBlock = false;
        }
        continue;
      }
      if (inTryBlock) {
        if (line.includes(']')) {
          inTryBlock = false;
          const cleaned = line.replace(']', '').trim();
          if (cleaned) {
            const v = cleaned.replace(/[",]/g, '').trim();
            if (v) cfg.tryServers.push(v);
          }
        } else {
          const v = line.replace(/[",]/g, '').trim();
          if (v) cfg.tryServers.push(v);
        }
        continue;
      }

      // Key = "Value"
      const eqIdx = line.indexOf('=');
      if (eqIdx > 0) {
        const key = line.substring(0, eqIdx).trim();
        const val = extractTomlValue(line.substring(eqIdx + 1));
        if (key && val && key !== 'try') {
          cfg.servers.push({
            name: key,
            address: val,
          });
        }
      }
    }
  }

  return cfg;
}

function extractTomlValue(line: string): string {
  const eq = line.indexOf('=');
  const target = eq >= 0 ? line.substring(eq + 1).trim() : line.trim();
  if ((target.startsWith('"') && target.endsWith('"')) || (target.startsWith("'") && target.endsWith("'"))) {
    return target.slice(1, -1);
  }
  return target;
}

function extractTomlArray(line: string): string[] {
  const start = line.indexOf('[');
  const end = line.lastIndexOf(']');
  if (start >= 0 && end > start) {
    const raw = line.substring(start + 1, end).trim();
    return raw
      .split(',')
      .map((s) => s.replace(/[ "']/g, '').trim())
      .filter(Boolean);
  }
  return [];
}

/**
 * Serializes ProxyConfig into Velocity TOML format.
 */
export function serializeVelocityToml(cfg: ProxyConfig): string {
  const tryArr = cfg.tryServers.map((s) => `  "${s}"`).join(',\n');
  const tryBlock = cfg.tryServers.length > 0 ? `try = [\n${tryArr}\n]` : `try = []`;

  const serverEntries = cfg.servers
    .map((s) => `  ${s.name} = "${s.address}"`)
    .join('\n');

  return `# Velocity Proxy Configuration
config-version = "2.7"
bind = "${cfg.bind || '0.0.0.0:25577'}"
motd = "${cfg.motd || 'A Velocity Proxy'}"
show-max-players = ${cfg.showMaxPlayers || 500}
online-mode = ${cfg.onlineMode}
force-key-authentication = true
player-info-forwarding-mode = "${cfg.playerInfoForwardingMode || 'modern'}"
forwarding-secret = "${cfg.forwardingSecret || ''}"

# Registered backend servers
# Containers within the panel network can be referenced as "mcm-<server-id>:25565"
[servers]
${serverEntries}

# Priority fallback servers when players connect
${tryBlock}

[forced-hosts]
`;
}

/**
 * Parses BungeeCord / Waterfall config.yml into structured ProxyConfig.
 */
export function parseBungeeYaml(content: string): ProxyConfig {
  const lines = content.split('\n');
  const cfg: ProxyConfig = {
    bind: '0.0.0.0:25577',
    motd: '&1A BungeeCord Proxy',
    showMaxPlayers: 500,
    onlineMode: true,
    playerInfoForwardingMode: 'legacy',
    forwardingSecret: '',
    forceDefaultServer: false,
    servers: [],
    tryServers: [],
  };

  let section: '' | 'servers' | 'listeners' | 'priorities' = '';
  let currentServer: ProxyBackendServer | null = null;

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i];
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) continue;

    const indent = rawLine.search(/\S/);

    if (indent === 0 && !line.startsWith('-')) {
      if (currentServer) {
        cfg.servers.push(currentServer);
        currentServer = null;
      }
      if (line.startsWith('servers:')) {
        section = 'servers';
        continue;
      } else if (line.startsWith('listeners:')) {
        section = 'listeners';
        continue;
      } else if (line.startsWith('online_mode:')) {
        cfg.onlineMode = line.split(':')[1]?.trim() === 'true';
        section = '';
        continue;
      } else if (line.startsWith('ip_forward:')) {
        const isFwd = line.split(':')[1]?.trim() === 'true';
        cfg.playerInfoForwardingMode = isFwd ? 'legacy' : 'none';
        section = '';
        continue;
      } else {
        section = '';
      }
    }

    if (section === 'priorities') {
      if (line.startsWith('- ')) {
        const p = line.substring(2).trim().replace(/['"]/g, '');
        if (p) cfg.tryServers.push(p);
        continue;
      } else {
        section = 'listeners';
      }
    }

    if (section === 'listeners') {
      const clean = line.startsWith('- ') ? line.substring(2).trim() : line;
      if (clean.startsWith('host:')) {
        cfg.bind = clean.substring('host:'.length).trim().replace(/['"]/g, '');
      } else if (clean.startsWith('max_players:')) {
        cfg.showMaxPlayers = parseInt(clean.substring('max_players:'.length).trim(), 10) || 500;
      } else if (clean.startsWith('motd:')) {
        cfg.motd = clean.substring('motd:'.length).trim().replace(/^['"]|['"]$/g, '');
      } else if (clean.startsWith('force_default_server:')) {
        cfg.forceDefaultServer = clean.substring('force_default_server:'.length).trim() === 'true';
      } else if (clean.startsWith('priorities:')) {
        section = 'priorities';
      }
    } else if (section === 'servers') {
      if (indent >= 1 && indent <= 4 && line.endsWith(':') && !line.startsWith('-')) {
        if (currentServer) {
          cfg.servers.push(currentServer);
        }
        currentServer = {
          name: line.slice(0, -1).trim(),
          address: '',
          motd: '',
          restricted: false,
        };
      } else if (currentServer) {
        if (line.startsWith('address:')) {
          currentServer.address = line.substring('address:'.length).trim().replace(/['"]/g, '');
        } else if (line.startsWith('motd:')) {
          currentServer.motd = line.substring('motd:'.length).trim().replace(/^['"]|['"]$/g, '');
        } else if (line.startsWith('restricted:')) {
          currentServer.restricted = line.substring('restricted:'.length).trim() === 'true';
        }
      }
    }
  }

  if (currentServer) {
    cfg.servers.push(currentServer);
  }

  return cfg;
}

/**
 * Serializes ProxyConfig into BungeeCord/Waterfall YAML format.
 */
export function serializeBungeeYaml(cfg: ProxyConfig): string {
  const prioritiesYaml =
    cfg.tryServers.length > 0
      ? cfg.tryServers.map((s) => `    - ${s}`).join('\n')
      : '    - lobby';

  const serversYaml =
    cfg.servers.length > 0
      ? cfg.servers
          .map(
            (s) => `  ${s.name}:
    motd: '${s.motd || '&1A Minecraft Server'}'
    address: ${s.address}
    restricted: ${s.restricted ?? false}`
          )
          .join('\n')
      : '  lobby:\n    motd: \'&1Just another BungeeCord\'\n    address: localhost:25565\n    restricted: false';

  return `# BungeeCord / Waterfall Proxy Configuration
listeners:
  - query_port: 25577
    motd: '${cfg.motd || '&1A BungeeCord Proxy'}'
    tab_list: GLOBAL_PING
    query_enabled: false
    proxy_protocol: false
    forced_hosts: {}
    ping_passthrough: false
    priorities:
${prioritiesYaml}
    bind_local_address: true
    host: ${cfg.bind || '0.0.0.0:25577'}
    max_players: ${cfg.showMaxPlayers || 500}
    tab_size: 60
    force_default_server: ${cfg.forceDefaultServer}
remote_ping_timeout: 5000
network_compression_threshold: 256
permissions:
  default:
  - bungeecord.command.server
  - bungeecord.command.list
  admin:
  - bungeecord.command.alert
  - bungeecord.command.end
  - bungeecord.command.ip
  - bungeecord.command.reload
log_pings: true
connection_throttle_limit: 4000
server_connect_timeout: 5000
timeout: 30000
player_limit: -1
prevent_proxy_connections: false
ip_forward: ${cfg.playerInfoForwardingMode !== 'none'}
servers:
${serversYaml}
`;
}
