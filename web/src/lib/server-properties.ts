export type PropertyCategory =
  | 'motd'
  | 'general'
  | 'gameplay'
  | 'security'
  | 'performance'
  | 'rcon'
  | 'custom';

export type PropertyType = 'boolean' | 'number' | 'select' | 'text' | 'motd';

export interface PropertySchema {
  key: string;
  label: string;
  description: string;
  category: Exclude<PropertyCategory, 'custom'>;
  type: PropertyType;
  defaultValue?: string;
  options?: { label: string; value: string }[];
  min?: number;
  max?: number;
  step?: number;
  placeholder?: string;
}

export const PROPERTY_CATEGORIES: { id: PropertyCategory; label: string; icon: string }[] = [
  { id: 'motd', label: 'MOTD Designer', icon: 'Sparkles' },
  { id: 'general', label: 'General & Network', icon: 'Globe' },
  { id: 'gameplay', label: 'Gameplay & World', icon: 'Gamepad2' },
  { id: 'security', label: 'Security & Access', icon: 'Shield' },
  { id: 'performance', label: 'Performance', icon: 'Cpu' },
  { id: 'rcon', label: 'RCON & Remote', icon: 'Terminal' },
  { id: 'custom', label: 'Custom & Other', icon: 'Sliders' },
];

export const PROPERTY_SCHEMAS: PropertySchema[] = [
  // MOTD
  {
    key: 'motd',
    label: 'Message of the Day (MOTD)',
    description: 'The server description displayed in the Minecraft multiplayer server browser.',
    category: 'motd',
    type: 'motd',
    defaultValue: 'A Minecraft Server',
  },

  // General & Network
  {
    key: 'server-port',
    label: 'Server Port',
    description: 'The primary game port listening for Minecraft client connections.',
    category: 'general',
    type: 'number',
    min: 1,
    max: 65535,
    step: 1,
    defaultValue: '25565',
  },
  {
    key: 'server-ip',
    label: 'Bind IP',
    description: 'The network address the server binds to. Leave empty to bind to all interfaces.',
    category: 'general',
    type: 'text',
    placeholder: '0.0.0.0 or leave blank',
    defaultValue: '',
  },
  {
    key: 'max-players',
    label: 'Max Players',
    description: 'The maximum number of players permitted to join simultaneously.',
    category: 'general',
    type: 'number',
    min: 1,
    max: 100000,
    step: 1,
    defaultValue: '20',
  },
  {
    key: 'online-mode',
    label: 'Online Mode (Mojang Auth)',
    description: 'Verifies connecting players against official Mojang/Microsoft account authentication.',
    category: 'general',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'prevent-proxy-connections',
    label: 'Prevent Proxy / VPN Connections',
    description: 'Rejects connections from players using commercial VPNs or proxies (if supported by Mojang).',
    category: 'general',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'enable-status',
    label: 'Enable Status Listing',
    description: 'Whether the server responds to ping requests and appears online in server browsers.',
    category: 'general',
    type: 'boolean',
    defaultValue: 'true',
  },

  // Gameplay & World
  {
    key: 'gamemode',
    label: 'Default Game Mode',
    description: 'The game mode applied to players when joining for the first time.',
    category: 'gameplay',
    type: 'select',
    defaultValue: 'survival',
    options: [
      { label: 'Survival', value: 'survival' },
      { label: 'Creative', value: 'creative' },
      { label: 'Adventure', value: 'adventure' },
      { label: 'Spectator', value: 'spectator' },
    ],
  },
  {
    key: 'difficulty',
    label: 'Game Difficulty',
    description: 'Controls hostile mob damage, hunger starvation, and general game challenge.',
    category: 'gameplay',
    type: 'select',
    defaultValue: 'easy',
    options: [
      { label: 'Peaceful', value: 'peaceful' },
      { label: 'Easy', value: 'easy' },
      { label: 'Normal', value: 'normal' },
      { label: 'Hard', value: 'hard' },
    ],
  },
  {
    key: 'hardcore',
    label: 'Hardcore Mode',
    description: 'Locks difficulty to Hard and permanently bans/spectates players upon death.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'pvp',
    label: 'Player vs Player (PvP)',
    description: 'Enables combat and damage between players.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'level-name',
    label: 'World Folder Name',
    description: 'The directory name where the primary world save files are stored.',
    category: 'gameplay',
    type: 'text',
    defaultValue: 'world',
  },
  {
    key: 'level-seed',
    label: 'World Seed',
    description: 'World generation seed. Leave blank to generate a random seed.',
    category: 'gameplay',
    type: 'text',
    placeholder: 'Optional world seed',
    defaultValue: '',
  },
  {
    key: 'level-type',
    label: 'World Generator Type',
    description: 'Specifies terrain generation structure (default, flat, amplified, etc.).',
    category: 'gameplay',
    type: 'select',
    defaultValue: 'minecraft:normal',
    options: [
      { label: 'Default / Normal', value: 'minecraft:normal' },
      { label: 'Flat World', value: 'minecraft:flat' },
      { label: 'Large Biomes', value: 'minecraft:large_biomes' },
      { label: 'Amplified Terrain', value: 'minecraft:amplified' },
      { label: 'Single Biome Surface', value: 'minecraft:single_biome_surface' },
    ],
  },
  {
    key: 'allow-flight',
    label: 'Allow Flight',
    description: 'Allows players in Survival mode to fly (e.g. mods, plugins) without being kicked.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'allow-nether',
    label: 'Allow Nether Dimension',
    description: 'Allows portal travel into the Nether dimension.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'spawn-protection',
    label: 'Spawn Protection Radius',
    description: 'Radius in blocks around world spawn where non-operators cannot break or place blocks.',
    category: 'gameplay',
    type: 'number',
    min: 0,
    max: 1000,
    step: 1,
    defaultValue: '16',
  },
  {
    key: 'spawn-monsters',
    label: 'Spawn Hostile Monsters',
    description: 'Determines if zombies, skeletons, creepers, and other hostile mobs spawn.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'spawn-animals',
    label: 'Spawn Passive Animals',
    description: 'Determines if cows, pigs, sheep, chickens, and other passive animals spawn.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'spawn-npcs',
    label: 'Spawn NPCs (Villagers)',
    description: 'Determines if villagers spawn in generated villages.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'generate-structures',
    label: 'Generate Structures',
    description: 'Generates villages, strongholds, mineshafts, dungeons, and temples in the world.',
    category: 'gameplay',
    type: 'boolean',
    defaultValue: 'true',
  },

  // Security & Access
  {
    key: 'white-list',
    label: 'Enforce Whitelist',
    description: 'Only players listed in the whitelist are permitted to connect.',
    category: 'security',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'enforce-whitelist',
    label: 'Strict Whitelist Reload',
    description: 'Instantly kicks currently connected non-whitelisted players whenever whitelist updates.',
    category: 'security',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'enforce-secure-profile',
    label: 'Enforce Secure Chat Profile',
    description: 'Requires players to have cryptographically signed Mojang chat keys.',
    category: 'security',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'hide-online-players',
    label: 'Hide Online Player List',
    description: 'Hides player names and avatars when hovering over player count in server list.',
    category: 'security',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'op-permission-level',
    label: 'Operator Permission Level',
    description: 'Default permission privileges granted to newly assigned server operators.',
    category: 'security',
    type: 'select',
    defaultValue: '4',
    options: [
      { label: 'Level 1 - Bypass spawn protection', value: '1' },
      { label: 'Level 2 - Basic cheats & command blocks', value: '2' },
      { label: 'Level 3 - Server moderation (/kick, /ban, /op)', value: '3' },
      { label: 'Level 4 - Full admin (/stop and all commands)', value: '4' },
    ],
  },
  {
    key: 'rate-limit',
    label: 'Packet Rate Limit',
    description: 'Max packets a client can send per second before being kicked (0 disables).',
    category: 'security',
    type: 'number',
    min: 0,
    max: 10000,
    step: 10,
    defaultValue: '0',
  },

  // Performance & Limits
  {
    key: 'view-distance',
    label: 'Render View Distance',
    description: 'The maximum chunk render distance sent to connected players (chunks).',
    category: 'performance',
    type: 'number',
    min: 2,
    max: 32,
    step: 1,
    defaultValue: '10',
  },
  {
    key: 'simulation-distance',
    label: 'Simulation Distance',
    description: 'Chunk radius around players where mobs tick, crops grow, and redstone operates.',
    category: 'performance',
    type: 'number',
    min: 2,
    max: 32,
    step: 1,
    defaultValue: '10',
  },
  {
    key: 'entity-broadcast-range-percentage',
    label: 'Entity Broadcast Range (%)',
    description: 'Percentage of view distance at which entity packet updates are sent to players.',
    category: 'performance',
    type: 'number',
    min: 10,
    max: 500,
    step: 10,
    defaultValue: '100',
  },
  {
    key: 'network-compression-threshold',
    label: 'Network Compression Threshold',
    description: 'Packet size in bytes where compression begins (-1 disables compression, 0 compresses all).',
    category: 'performance',
    type: 'number',
    min: -1,
    max: 65535,
    step: 64,
    defaultValue: '256',
  },
  {
    key: 'max-world-size',
    label: 'Max World Border Size',
    description: 'Maximum block radius from world center before the world border boundary.',
    category: 'performance',
    type: 'number',
    min: 1000,
    max: 29999984,
    step: 1000,
    defaultValue: '29999984',
  },
  {
    key: 'max-tick-time',
    label: 'Max Tick Watchdog Time (ms)',
    description: 'Max time a single tick may freeze before watchdog stops server (-1 disables watchdog).',
    category: 'performance',
    type: 'number',
    min: -1,
    max: 600000,
    step: 1000,
    defaultValue: '60000',
  },
  {
    key: 'sync-chunk-writes',
    label: 'Sync Chunk Disk Writes',
    description: 'Synchronously commits world chunk files to storage to prevent world corruption.',
    category: 'performance',
    type: 'boolean',
    defaultValue: 'true',
  },

  // RCON & Remote
  {
    key: 'enable-rcon',
    label: 'Enable RCON Server',
    description: 'Enables Minecraft Remote Console protocol for external command execution.',
    category: 'rcon',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'rcon.port',
    label: 'RCON Port',
    description: 'Port for incoming RCON administrative connections.',
    category: 'rcon',
    type: 'number',
    min: 1,
    max: 65535,
    step: 1,
    defaultValue: '25575',
  },
  {
    key: 'rcon.password',
    label: 'RCON Password',
    description: 'Secret authentication password required to issue RCON console commands.',
    category: 'rcon',
    type: 'text',
    placeholder: 'Enter secure RCON password',
    defaultValue: '',
  },
  {
    key: 'broadcast-rcon-to-ops',
    label: 'Broadcast RCON Output to Operators',
    description: 'Notifies connected server operators in chat when an RCON command is executed.',
    category: 'rcon',
    type: 'boolean',
    defaultValue: 'true',
  },
  {
    key: 'enable-query',
    label: 'Enable GameSpy4 Query',
    description: 'Enables GameSpy4 query protocol for server stats, player lists, and pinging.',
    category: 'rcon',
    type: 'boolean',
    defaultValue: 'false',
  },
  {
    key: 'query.port',
    label: 'Query Port',
    description: 'Port for GameSpy4 query protocol (usually same as game port).',
    category: 'rcon',
    type: 'number',
    min: 1,
    max: 65535,
    step: 1,
    defaultValue: '25565',
  },
];

export const SCHEMA_MAP = new Map<string, PropertySchema>(
  PROPERTY_SCHEMAS.map((s) => [s.key, s]),
);

export interface ParsedPropertyLine {
  id: string;
  type: 'property' | 'comment' | 'empty';
  key?: string;
  value?: string;
  raw: string;
}

let lineCounter = 0;
function nextId(): string {
  lineCounter += 1;
  return `prop_line_${Date.now()}_${lineCounter}`;
}

/**
 * Parses raw .properties text line-by-line, preserving comments and blank lines.
 */
export function parseProperties(raw: string): ParsedPropertyLine[] {
  const lines = raw.split(/\r?\n/);
  return lines.map((line) => {
    const trimmed = line.trim();
    if (trimmed === '') {
      return { id: nextId(), type: 'empty', raw: line };
    }
    if (trimmed.startsWith('#') || trimmed.startsWith('!')) {
      return { id: nextId(), type: 'comment', raw: line };
    }
    const eqIdx = line.indexOf('=');
    if (eqIdx > 0) {
      const key = line.slice(0, eqIdx).trim();
      const value = line.slice(eqIdx + 1);
      return { id: nextId(), type: 'property', key, value, raw: line };
    }
    // Line without an = sign, treat as comment or raw
    return { id: nextId(), type: 'comment', raw: line };
  });
}

/**
 * Converts parsed property lines back to raw string format.
 */
export function serializeProperties(lines: ParsedPropertyLine[]): string {
  return lines.map((l) => {
    if (l.type === 'property' && l.key !== undefined) {
      return `${l.key}=${l.value ?? ''}`;
    }
    return l.raw;
  }).join('\n');
}

/**
 * Extract key-value dictionary for fast lookup.
 */
export function getPropertiesMap(lines: ParsedPropertyLine[]): Record<string, string> {
  const map: Record<string, string> = {};
  for (const line of lines) {
    if (line.type === 'property' && line.key) {
      map[line.key] = line.value ?? '';
    }
  }
  return map;
}

/**
 * Updates a property in the lines array in-place, or appends it if not present.
 */
export function updatePropertyInLines(
  lines: ParsedPropertyLine[],
  key: string,
  value: string,
): ParsedPropertyLine[] {
  let found = false;
  const updated = lines.map((line) => {
    if (line.type === 'property' && line.key === key) {
      found = true;
      return {
        ...line,
        value,
        raw: `${key}=${value}`,
      };
    }
    return line;
  });

  if (!found) {
    updated.push({
      id: nextId(),
      type: 'property',
      key,
      value,
      raw: `${key}=${value}`,
    });
  }

  return updated;
}

/**
 * Removes a property from the lines array.
 */
export function deletePropertyFromLines(
  lines: ParsedPropertyLine[],
  key: string,
): ParsedPropertyLine[] {
  return lines.filter((line) => !(line.type === 'property' && line.key === key));
}

/**
 * Returns all custom/unmapped properties from the parsed lines.
 */
export function getCustomProperties(lines: ParsedPropertyLine[]): { key: string; value: string }[] {
  const custom: { key: string; value: string }[] = [];
  for (const line of lines) {
    if (line.type === 'property' && line.key && !SCHEMA_MAP.has(line.key)) {
      custom.push({ key: line.key, value: line.value ?? '' });
    }
  }
  return custom;
}
