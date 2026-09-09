export type ServerType = 'paper' | 'fabric' | 'vanilla' | 'forge' | 'neoforge' | 'spigot';
export type ServerState = 'stopped' | 'starting' | 'running' | 'stopping' | 'error';

export interface ExtraPort {
  id: string;
  description: string;
  host_port: number;
  container_port: number;
  protocol: 'tcp' | 'udp';
}

export interface Server {
  id: string;
  name: string;
  server_type: ServerType;
  version: string;
  build: string;
  ram_mb: number;
  cpu_limit: number;
  memory_limit_mb: number;
  host_port: number;
  extra_ports: ExtraPort[];
  container_id: string | null;
  state: ServerState;
  backup_enabled: boolean;
  backup_interval_minutes: number;
  spin_down_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface Me {
  id: string;
  email: string;
  totp_enabled?: boolean;
  created_at?: string;
}

export interface PasskeyMeta {
  id: string;
  name: string;
}

export interface User {
  id: string;
  email: string;
  totp_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServerStatus {
  id: string;
  state: ServerState;
  ram_mb: number;
  host_port: number;
  container_id: string | null;
}

export interface ConsoleLine {
  timestamp: string;
  level?: string;
  message: string;
}

export interface VersionInfo {
  version: string;
  build: string;
  display: string;
}

export interface InstallInfo {
  server_id: string;
  installed: boolean;
  version: string | null;
  build: string | null;
}

export interface VersionMeta {
  name: string;
  latest?: string;
}

export interface Settings {
  [key: string]: unknown;
}

export type BackupStatus = 'pending' | 'completed' | 'failed';

export interface BackupRecord {
  id: string;
  server_id: string;
  name: string;
  size_bytes: number;
  location: string;
  status: BackupStatus;
  created_at: string;
}

export interface Player {
  name: string;
}

export interface PlayerCommandArgs {
  reason?: string;
  target?: string;
  item?: string;
  amount?: number;
  mode?: string;
  command?: string;
  nbt?: string;
}

export type PlayerCommandAction =
  | 'kick'
  | 'ban'
  | 'pardon'
  | 'op'
  | 'deop'
  | 'give'
  | 'gamemode'
  | 'tp'
  | 'kill'
  | 'custom';

export interface PlayerList {
  players: Player[];
  source: 'rcon' | 'console';
}

export interface Op {
  uuid: string;
  name: string;
  level: number;
  bypassesPlayerLimit?: boolean;
}

export interface WhitelistEntry {
  uuid: string;
  name: string;
}

export interface Mod {
  name: string;
  file: string;
  enabled: boolean;
}

export interface ModList {
  type: 'mods' | 'plugins';
  items: Mod[];
}

export interface ServerProperties {
  content: string;
  exists: boolean;
}

export interface FileEntry {
  name: string;
  is_directory: boolean;
  size_bytes: number;
  modified_at: string;
}

export interface FileList {
  path: string;
  entries: FileEntry[];
}

export interface FileContent {
  path: string;
  content: string;
}

export interface UnzipResult {
  ok: boolean;
  count: number;
}

export interface ModrinthSearchHit {
  project_id: string;
  project_type: string;
  slug: string;
  author: string;
  title: string;
  description: string;
  categories: string[];
  display_categories?: string[];
  versions: string[];
  downloads: number;
  follows: number;
  icon_url: string | null;
  date_created: string;
  date_modified: string;
  latest_version: string;
  license: string;
  client_side: 'required' | 'optional' | 'unsupported';
  server_side: 'required' | 'optional' | 'unsupported';
  gallery?: string[];
  color?: number;
}

export interface ModrinthSearchResult {
  hits: ModrinthSearchHit[];
  offset: number;
  limit: number;
  total_hits: number;
}

export interface ModrinthVersionFile {
  hashes?: { sha1?: string; sha512?: string };
  url: string;
  filename: string;
  primary: boolean;
  size: number;
  file_type?: string | null;
}

export interface ModrinthVersion {
  id: string;
  project_id: string;
  author_id: string;
  name: string;
  version_number: string;
  game_versions: string[];
  version_type: 'release' | 'beta' | 'alpha';
  loaders: string[];
  featured: boolean;
  status: string;
  date_published: string;
  downloads: number;
  changelog?: string;
  files: ModrinthVersionFile[];
}

export interface ModrinthProject {
  id: string;
  slug: string;
  title: string;
  description: string;
  body?: string;
  categories: string[];
  client_side: string;
  server_side: string;
  body_url?: string | null;
  issues_url?: string | null;
  source_url?: string | null;
  wiki_url?: string | null;
  discord_url?: string | null;
  icon_url: string | null;
  downloads: number;
  followers: number;
  license?: { id: string; name: string; url: string | null };
}
