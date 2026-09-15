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
  started_at?: string | null;
  uptime_seconds?: number;
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
  started_at?: string | null;
  uptime_seconds?: number;
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

export interface DNSRecord {
  server_id: string;
  record_id: string;
  name: string;
  subdomain?: string;
  target: string;
  port: number;
  priority: number;
  weight: number;
  ttl: number;
  zone?: string;
  updated_at: string;
}

export interface DNSConfig {
  publish: boolean;
  domain: string;
  zone: string;
  has_token: boolean;
  host: string;
  service: string;
  proto: string;
  ttl: number;
  priority: number;
  weight: number;
}

export interface DNSStatusResponse {
  records: DNSRecord[];
  config?: DNSConfig;
  configured: boolean;
}

export interface ServerDNSResponse {
  record: DNSRecord | null;
  configured: boolean;
  domain: string;
  server_id: string;
  server_name: string;
  host_port: number;
  join_address: string;
}

export interface DNSTestResult {
  ok: boolean;
  zone_name?: string;
  status?: string;
  message: string;
}

export interface PublishDNSInput {
  subdomain?: string;
  target?: string;
  port?: number;
  priority?: number;
  weight?: number;
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

export interface BackupProgress {
  id: string;
  server_id: string;
  operation: 'backup' | 'restore' | 'upload';
  stage: 'scanning' | 'compressing' | 'saving' | 'downloading' | 'extracting' | 'completed' | 'failed';
  percent: number;
  message: string;
  bytes_done?: number;
  bytes_total?: number;
  error?: string;
  updated_at?: string;
}

export interface BackupProgressResponse {
  active: boolean;
  progress?: BackupProgress;
}

export interface AvailablePortsResponse {
  available: number[];
  pool?: number[];
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
  mod_id?: string;
  title?: string;
  version?: string;
  sha1?: string;
  description?: string;
  project_id?: string;
  project_slug?: string;
  provider?: ModProvider;
}

export interface ModList {
  type: 'mods' | 'plugins';
  items: Mod[];
}

export interface AvailableModJar {
  version_id: string;
  version_name: string;
  version_number: string;
  filename: string;
  download_url: string;
  size_bytes?: number;
  release_type?: string;
  game_versions?: string[];
  loaders?: string[];
  date_published?: string;
  is_current?: boolean;
}

export interface ServerModUpdateInfo {
  mod_name: string;
  mod_file: string;
  provider: ModProvider;
  project_id?: string;
  project_slug?: string;
  title: string;
  current_version?: string;
  latest_version: string;
  latest_jar: string;
  latest_download_url?: string;
  latest_release_type?: string;
  latest_release_date?: string;
  changelog?: string;
  available_jars?: AvailableModJar[];
}

export interface ServerModUpdatesResponse {
  updates: Record<string, ServerModUpdateInfo>;
  last_checked: string;
  total_mods?: number;
  update_count?: number;
}

export type { ModUpdateInfo } from './mod-updates';

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
  environment?: string[];
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

export type ModProvider = 'modrinth' | 'hangar' | 'spiget' | 'curseforge';

// --- Hangar (PaperMC) ---
export interface HangarPagination {
  count: number;
  limit: number;
  offset: number;
}

export interface HangarNamespace {
  owner: string;
  slug: string;
}

export interface HangarStats {
  views: number;
  downloads: number;
  recentViews: number;
  recentDownloads: number;
  stars: number;
  watchers: number;
}

export interface HangarProject {
  id: number;
  name: string;
  namespace: HangarNamespace;
  stats: HangarStats;
  category: string;
  description: string;
  lastUpdated: string;
  visibility: string;
  avatarUrl: string;
  supportedPlatforms: Record<string, string[]>;
  mainPageContent?: string;
  memberNames?: string[];
}

export interface HangarSearchResult {
  pagination: HangarPagination;
  result: HangarProject[];
}

export interface HangarFileInfo {
  name: string;
  sizeBytes: number;
  sha256Hash: string;
}

export interface HangarPlatformDownload {
  fileInfo?: HangarFileInfo;
  downloadUrl?: string;
  externalUrl?: string | null;
}

export interface HangarVersion {
  id: number;
  projectId: number;
  name: string;
  visibility: string;
  description?: string;
  stats: {
    totalDownloads: number;
    platformDownloads: Record<string, number>;
  };
  author: string;
  channel: {
    name: string;
    color: string;
    description?: string;
  };
  downloads: Record<string, HangarPlatformDownload>;
  platformDependencies: Record<string, string[]>;
  platformDependenciesFormatted?: Record<string, string[]>;
  createdAt: string;
}

export interface HangarVersionsResult {
  pagination: HangarPagination;
  result: HangarVersion[];
}

// --- SpigotMC (via Spiget) ---
export interface SpigetFile {
  type: string;
  size: number;
  sizeUnit: string;
  url: string;
}

export interface SpigetRating {
  count: number;
  average: number;
}

export interface SpigetIcon {
  url: string;
  data?: string;
}

export interface SpigetResource {
  id: number;
  name: string;
  tag: string;
  version: {
    id: number;
    uuid?: string;
  };
  author: {
    id: number;
    name?: string;
  };
  category: {
    id: number;
  };
  rating: SpigetRating;
  downloads: number;
  icon: SpigetIcon;
  releaseDate: number;
  updateDate: number;
  testedVersions?: string[];
  premium?: boolean;
  file?: SpigetFile;
  links?: {
    discussion?: string;
    sourceCode?: string;
    donation?: string;
  };
}

export interface SpigetVersion {
  id: number;
  name: string;
  releaseDate: number;
  resource: number;
  downloads: number;
  rating: SpigetRating;
  uuid?: string;
}

// --- CurseForge ---
export interface CurseForgeFileHash {
  value: string;
  algo: number;
}

export interface CurseForgeFile {
  id: number;
  gameId: number;
  modId: number;
  isAvailable: boolean;
  displayName: string;
  fileName: string;
  releaseType: number; // 1 = Release, 2 = Beta, 3 = Alpha
  fileStatus: number;
  hashes: CurseForgeFileHash[];
  fileDate: string;
  fileLength: number;
  downloadCount: number;
  downloadUrl: string | null;
  gameVersions: string[];
}

export interface CurseForgeModAuthor {
  id: number;
  name: string;
  url: string;
}

export interface CurseForgeModLogo {
  id: number;
  modId: number;
  title: string;
  description: string;
  thumbnailUrl: string;
  url: string;
}

export interface CurseForgeCategory {
  id: number;
  gameId: number;
  name: string;
  slug: string;
  url: string;
  iconUrl: string;
}

export interface CurseForgeMod {
  id: number;
  gameId: number;
  name: string;
  slug: string;
  links: {
    websiteUrl: string;
    wikiUrl?: string;
    issuesUrl?: string;
    sourceUrl?: string;
  };
  summary: string;
  status: number;
  downloadCount: number;
  isFeatured: boolean;
  primaryCategoryId: number;
  categories: CurseForgeCategory[];
  classId?: number;
  authors: CurseForgeModAuthor[];
  logo?: CurseForgeModLogo;
  dateModified: string;
  dateCreated: string;
  dateReleased: string;
  latestFiles: CurseForgeFile[];
}

export interface CurseForgeSearchResult {
  data: CurseForgeMod[];
  pagination: {
    index: number;
    pageSize: number;
    totalCount: number;
  };
}

export type ModpackFormat = 'modrinth' | 'curseforge' | 'generic';

export interface ModpackManifest {
  format: ModpackFormat;
  name: string;
  version: string;
  summary?: string;
  author?: string;
  minecraft_version: string;
  loader: string;
  loader_version: string;
  total_files: number;
  server_files: number;
  client_only_files: number;
  icon_url?: string;
}

export interface InstalledModpack {
  name: string;
  version: string;
  summary?: string;
  author?: string;
  format: ModpackFormat;
  minecraft_version: string;
  loader: string;
  loader_version: string;
  installed_at: string;
  source: string;
  project_id?: string;
  project_slug?: string;
  installed_files: string[];
  icon_url?: string;
  created_with_modpack?: boolean;
}

export interface InstalledModpackResponse {
  installed: boolean;
  modpack: InstalledModpack | null;
}

export interface InstallModpackOptions {
  source?: string;
  url?: string;
  project_id?: string;
  project_slug?: string;
  version_id?: string;
  auto_configure_server?: boolean;
  curseforge_api_key?: string;
  created_with_modpack?: boolean;
}

export interface CopyServerInput {
  name: string;
  host_port?: number;
  ram_mb?: number;
  cpu_limit?: number;
  memory_limit_mb?: number;
  include_world?: boolean;
  include_config?: boolean;
  include_plugins?: boolean;
  include_mods?: boolean;
  include_player_data?: boolean;
  include_logs?: boolean;
  custom_excludes?: string[];
}



