import type {
  DiscordSettings,
  DiscordSettingsInput,
  ScheduledTask,
  ScheduleInput,
  ConsoleLine,
  ExtraPort,
  InstallInfo,
  Me,
  Server,
  ServerStats,
  PerformanceHistory,
  PerformanceRange,
  PerformanceCollectionMode,
  PerformanceSettings,
  SparkReport,
  HostCapacity,
  ServerStatus,
  ServerType,
  JavaRelease,
  BackupRecord,
  BackupProgress,
  BackupProgressResponse,
  TaskProgress,
  TaskProgressResponse,
  VersionInfo,
  VersionMeta,
  Op,
  WhitelistEntry,
  PlayerList,
  Mod,
  ModList,
  ModProvider,
  ServerProperties,
  PlayerCommandAction,
  PlayerCommandArgs,
  FileEntry,
  FileList,
  FileContent,
  ServerLogFile,
  ServerLogContent,
  UnzipResult,
  User,
  PasskeyMeta,
  DNSRecord,
  DNSStatusResponse,
  ServerDNSResponse,
  DNSTestResult,
  InstalledModpack,
  InstalledModpackResponse,
  InstallModpackOptions,
  ModpackManifest,
  PublishDNSInput,
  AvailableModJar,
  ServerModUpdatesResponse,
  VersionUpdateReport,
  BatchUpdateModsResult,
  AvailablePortsResponse,
  PortCheckResponse,
  CopyServerInput,
} from './types';

export class ApiError extends Error {
  status: number;
  detail: string;
  code?: string;

  constructor(status: number, message: string, detail?: string, code?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.detail = detail ?? message;
    this.code = code;
  }
}

export class FileConflictError extends ApiError {
  current: FileContent;

  constructor(message: string, current: FileContent) {
    super(409, message);
    this.name = 'FileConflictError';
    this.current = current;
  }
}

const safeMethods = new Set(['GET', 'HEAD', 'OPTIONS']);

let csrfToken: string | null = null;
let csrfPromise: Promise<string> | null = null;

function getCsrfFromCookie(): string | null {
  if (typeof document === 'undefined') return null;
  const match = document.cookie.match(/(?:^|;\s*)mcm_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : null;
}

async function ensureCsrf(forceRefresh = false): Promise<string> {
  if (!forceRefresh) {
    const cookieToken = getCsrfFromCookie();
    if (cookieToken) {
      csrfToken = cookieToken;
      return cookieToken;
    }
    if (csrfToken) return csrfToken;
  }
  if (!csrfPromise || forceRefresh) {
    csrfPromise = fetch('/api/auth/csrf', { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('csrf unavailable'))))
      .then((body) => {
        csrfToken = String(body?.csrf_token ?? getCsrfFromCookie() ?? '');
        return csrfToken;
      })
      .finally(() => {
        csrfPromise = null;
      });
  }
  return csrfPromise;
}

function isMock(): boolean {
  if (typeof window === 'undefined') return false;
  return (
    window.location.search.includes('mock=true') ||
    window.localStorage?.getItem('mcm-mock') === 'true'
  );
}

let mockPropertiesContent =
  '# Minecraft server properties\nserver-port=25565\nmotd=§aMega SMP Server §7| §e100 Players Online!\nmax-players=150\npvp=true\nview-distance=10\nsimulation-distance=8\nwhite-list=true\nenforce-whitelist=false\ndifficulty=hard\nenable-rcon=true\nrcon.port=25575';

let mockMods: Mod[] = [
  { name: 'EssentialsX', file: 'EssentialsX-2.20.1.jar', enabled: true },
  { name: 'Vault', file: 'Vault.jar', enabled: true },
  { name: 'CoreProtect', file: 'CoreProtect-22.4.jar', enabled: true },
  { name: 'LuckPerms', file: 'LuckPerms-5.4.102.jar', enabled: true },
];

let mockDNSSettings: Record<string, string> = {
  dns_publish: 'true',
  dns_domain: 'example.com',
  dns_zone: 'zone-123456789',
  dns_api_token: 'cf_tok_abcdef123456',
  dns_host: 'mc-node1.example.com',
  dns_service: '_minecraft',
  dns_proto: '_tcp',
  dns_ttl: '120',
  dns_priority: '0',
  dns_weight: '5',
};

let mockDNSRecords: DNSRecord[] = [
  {
    server_id: 'demo',
    record_id: 'cf-rec-demo-1',
    name: '_minecraft._tcp.demo.example.com',
    subdomain: 'demo',
    target: 'mc-node1.example.com',
    port: 25565,
    priority: 0,
    weight: 5,
    ttl: 120,
    zone: 'zone-123456789',
    updated_at: '2026-01-01T00:00:00Z',
  },
];

let mockSchedules: ScheduledTask[] = [];
let mockDiscordSettings: DiscordSettings = {
  enabled: false, configured: false,
  events: ['server_started', 'server_stopped', 'server_crashed', 'server_error', 'backup_completed', 'backup_failed'],
};
const mockSparkReports: Record<string, SparkReport> = {};
const mockPerformanceModes: Record<string, PerformanceCollectionMode> = {};

function handleMockRequest<T>(path: string, init: RequestInit = {}): T | null {
  if (!isMock()) return null;

  const performanceSettingsPath = path.match(/^\/api\/servers\/([^/]+)\/performance\/settings$/);
  if (performanceSettingsPath && init.method === 'PUT') {
    const input = JSON.parse(String(init.body ?? '{}')) as PerformanceSettings;
    mockPerformanceModes[performanceSettingsPath[1]] = input.collection_mode;
    return { collection_mode: input.collection_mode } as T;
  }

  const performancePath = path.match(/^\/api\/servers\/([^/]+)\/performance(?:\?range=(1h|6h|24h|7d))?$/);
  if (performancePath) {
    const [, serverId, range = '1h'] = performancePath;
    const mode = mockPerformanceModes[serverId] ?? 'off';
    const hours = { '1h': 1, '6h': 6, '24h': 24, '7d': 168 }[range] ?? 1;
    const resolution = { '1h': 30, '6h': 60, '24h': 300, '7d': 1800 }[range] ?? 30;
    const count = hours * 3600 / resolution;
    const end = Date.now();
    const start = end - hours * 3600000;
    // Illustrative history is confined to the explicitly labeled demo mode.
    return {
      start: new Date(start).toISOString(), end: new Date(end).toISOString(),
      resolution_seconds: resolution, interval_seconds: 30, retention_days: 7,
      settings: { collection_mode: mode },
      spark: { available: true, source: 'bundled' }, report: mockSparkReports[serverId] ?? null,
      samples: Array.from({ length: count }, (_, index) => {
        const phase = index * 120 / count;
        const spike = phase > 70 && phase < 82;
        return {
          timestamp: new Date(start + index * resolution * 1000).toISOString(), online: true,
          cpu_percent: 18 + 8 * Math.sin(phase / 8) + (spike ? 24 : 0),
          memory_bytes: (1.6 + phase / 500 + 0.1 * Math.sin(phase / 5)) * 1024 ** 3,
          players: mode === 'off' ? null : Math.max(0, Math.round(9 + 6 * Math.sin(phase / 24))),
          tps: mode === 'off' ? null : spike ? 17.8 : 20,
          tick_ms: mode === 'off' ? null : 12 + 4 * Math.sin(phase / 6) + (spike ? 43 : 0),
        };
      }),
    } as T;
  }
  const sparkPath = path.match(/^\/api\/servers\/([^/]+)\/performance\/spark$/);
  if (sparkPath && init.method === 'POST') {
    const report: SparkReport = { id: crypto.randomUUID(), status: 'failed', started_at: new Date().toISOString(), existing: false, error: 'Demo mode cannot capture a real spark report.' };
    mockSparkReports[sparkPath[1]] = report;
    return report as T;
  }

  if (path === '/api/system/capacity') {
    return {
      runtime: 'docker', cpu_cores: 8, memory_total_bytes: 32 * 1024 ** 3,
      usage: { cpu_percent: 14.8, memory_bytes: 3.2 * 1024 ** 3, net_rx_bytes: 12582912, net_tx_bytes: 34603008, active_servers: 1 },
      disk: { path: '/data', total_bytes: 500 * 1024 ** 3, used_bytes: 180 * 1024 ** 3, available_bytes: 310 * 1024 ** 3 },
    } as T;
  }

  const schedulePath = path.match(/^\/api\/servers\/([^/]+)\/schedules(?:\/([^/]+))?$/);
  if (schedulePath) {
    const [, serverId, scheduleId] = schedulePath;
    if (!init.method || init.method === 'GET') return { schedules: mockSchedules.filter((task) => task.server_id === serverId), default_timezone: 'UTC' } as T;
    if (init.method === 'DELETE') { mockSchedules = mockSchedules.filter((task) => task.id !== scheduleId); return { ok: true } as T; }
    const body = JSON.parse(String(init.body ?? '{}'));
    const previous = mockSchedules.find((task) => task.id === scheduleId);
    const now = new Date().toISOString();
    // Demo mode does not dispatch commands or calculate live execution times.
    const task: ScheduledTask = { id: scheduleId ?? crypto.randomUUID(), server_id: serverId, last_run_at: '', last_status: '', last_error: '', next_run_at: '', created_at: now, ...previous, ...body, updated_at: now };
    mockSchedules = [...mockSchedules.filter((item) => item.id !== task.id), task];
    return task as T;
  }

  if (path === '/api/notifications/discord/test') {
    return { ok: true, message: 'Preview only: no message was sent to Discord.' } as T;
  }
  if (path === '/api/notifications/discord') {
    if (init.method === 'PUT') {
      const input = JSON.parse(String(init.body ?? '{}')) as DiscordSettingsInput;
      mockDiscordSettings = { enabled: input.enabled, events: input.events,
        configured: input.webhook_url === undefined ? mockDiscordSettings.configured : !!input.webhook_url.trim() };
    }
    return mockDiscordSettings as T;
  }

  if (path === '/api/settings') {
    if (init.method === 'PUT' && init.body) {
      try {
        const parsed = JSON.parse(init.body as string);
        if (parsed.settings) {
          mockDNSSettings = { ...mockDNSSettings, ...parsed.settings };
        }
      } catch {
        // ignore
      }
    }
    return { settings: mockDNSSettings } as T;
  }

  if (path === '/api/dns/test' && init.method === 'POST') {
    return {
      ok: true,
      zone_name: mockDNSSettings.dns_domain || 'example.com',
      status: 'active',
      message: `Successfully verified zone "${mockDNSSettings.dns_domain || 'example.com'}" (status: active)`,
    } as T;
  }

  if (path === '/api/dns' && (!init.method || init.method === 'GET')) {
    return {
      records: mockDNSRecords,
      config: {
        publish: mockDNSSettings.dns_publish === 'true',
        domain: mockDNSSettings.dns_domain || 'example.com',
        zone: mockDNSSettings.dns_zone || 'zone-123456789',
        has_token: !!mockDNSSettings.dns_api_token,
        host: mockDNSSettings.dns_host || 'mc-node1.example.com',
        service: mockDNSSettings.dns_service || '_minecraft',
        proto: mockDNSSettings.dns_proto || '_tcp',
        ttl: Number(mockDNSSettings.dns_ttl) || 120,
        priority: Number(mockDNSSettings.dns_priority) || 0,
        weight: Number(mockDNSSettings.dns_weight) || 5,
      },
      configured: mockDNSSettings.dns_publish === 'true' && !!mockDNSSettings.dns_api_token,
    } as T;
  }

  if (path.startsWith('/api/servers/') && path.endsWith('/dns')) {
    const serverId = path.split('/')[3];
    if (init.method === 'DELETE') {
      mockDNSRecords = mockDNSRecords.filter((r) => r.server_id !== serverId);
      return { ok: true } as T;
    }
    if (init.method === 'POST') {
      let sub = 'demo';
      let target = mockDNSSettings.dns_host || 'mc-node1.example.com';
      let port = 25565;
      let priority = 0;
      let weight = 5;
      if (init.body) {
        try {
          const parsed = JSON.parse(init.body as string);
          if (parsed.subdomain !== undefined) sub = parsed.subdomain;
          if (parsed.target) target = parsed.target;
          if (parsed.port) port = parsed.port;
          if (parsed.priority !== undefined) priority = parsed.priority;
          if (parsed.weight !== undefined) weight = parsed.weight;
        } catch {
          // ignore
        }
      }
      const domain = mockDNSSettings.dns_domain || 'example.com';
      const cleanSub = sub === '@' ? '' : sub;
      const recName = cleanSub ? `_minecraft._tcp.${cleanSub}.${domain}` : `_minecraft._tcp.${domain}`;
      const joinAddress = cleanSub ? `${cleanSub}.${domain}` : domain;
      const rec: DNSRecord = {
        server_id: serverId,
        record_id: `cf-rec-${serverId}`,
        name: recName,
        subdomain: sub,
        target,
        port,
        priority,
        weight,
        ttl: 120,
        zone: mockDNSSettings.dns_zone || 'zone-123456789',
        updated_at: new Date().toISOString(),
      };
      mockDNSRecords = [...mockDNSRecords.filter((r) => r.server_id !== serverId), rec];
      return { ok: true, record: rec, join_address: joinAddress } as T;
    }
    const rec = mockDNSRecords.find((r) => r.server_id === serverId) || null;
    const domain = mockDNSSettings.dns_domain || 'example.com';
    const joinAddress = rec ? (rec.subdomain && rec.subdomain !== '@' ? `${rec.subdomain}.${domain}` : domain) : '';
    return {
      record: rec,
      configured: mockDNSSettings.dns_publish === 'true' && !!mockDNSSettings.dns_api_token,
      domain,
      server_id: serverId,
      server_name: 'Mega SMP Server',
      host_port: 25565,
      join_address: joinAddress,
    } as T;
  }

  if (path === '/api/auth/me') {
    return { id: 'admin-1', email: 'admin@mcm.panel' } as T;
  }
  if (path === '/api/users' && (!init.method || init.method === 'GET')) {
    return { users: [{ id: 'admin-1', email: 'admin@mcm.panel', totp_enabled: false, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }] } as T;
  }
  if (path === '/api/auth/csrf') {
    return { csrf_token: 'mock-csrf-token' } as T;
  }
  if (path === '/api/onboarding/status') {
    return { onboarding_required: false } as T;
  }
  if (path === '/api/servers' && (!init.method || init.method === 'GET')) {
    return {
      servers: [
        {
          id: 'demo',
          name: 'Mega SMP Server',
          server_type: 'paper',
          version: '1.21.4',
          build: '145',
          ram_mb: 8192,
          cpu_limit: 4,
          memory_limit_mb: 8192,
          host_port: 25565,
          extra_ports: [],
          container_id: 'mcm-srv-demo',
          state: 'running',
          backup_enabled: true,
          backup_interval_minutes: 60,
          started_at: '2026-01-01T00:00:00Z',
          uptime_seconds: 3600,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/status')) {
    return { id: 'demo', state: 'running', ram_mb: 8192, host_port: 25565, container_id: 'mcm-srv-demo', started_at: '2026-01-01T00:00:00Z', uptime_seconds: 3600 } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/players')) {
    const players = Array.from({ length: 100 }, (_, i) => ({
      name: `Player_${String(i + 1).padStart(3, '0')}`,
    }));
    return { source: 'rcon', players } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/ops')) {
    return {
      ops: [
        { uuid: 'u-1', name: 'Player_001', level: 4, bypassesPlayerLimit: true },
        { uuid: 'u-2', name: 'Player_005', level: 4 },
      ],
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/whitelist')) {
    return {
      whitelist: [
        { uuid: 'w-1', name: 'Player_001' },
        { uuid: 'w-2', name: 'Player_002' },
        { uuid: 'w-3', name: 'Player_003' },
      ],
    } as T;
  }

  if (path.startsWith('/api/servers/') && path.endsWith('/properties')) {
    if (init.method === 'PUT' && init.body) {
      try {
        const parsed = JSON.parse(init.body as string);
        if (typeof parsed.content === 'string') {
          mockPropertiesContent = parsed.content;
        }
      } catch {
        // ignore parse error
      }
    }
    return {
      exists: true,
      content: mockPropertiesContent,
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/mods/download')) {
    let name = 'Downloaded-Mod';
    let file = 'downloaded-mod.jar';
    let deleteOldName = '';
    let projectId: string | undefined;
    let projectSlug: string | undefined;
    let provider: ModProvider | undefined;
    if (init.body) {
      try {
        const parsed = JSON.parse(init.body as string);
        if (parsed.filename) {
          file = parsed.filename;
          name = file.replace(/\.jar$/i, '');
        }
        if (parsed.delete_old_name) {
          deleteOldName = parsed.delete_old_name;
        }
        if (parsed.project_id) projectId = parsed.project_id;
        if (parsed.project_slug) projectSlug = parsed.project_slug;
        if (parsed.provider) provider = parsed.provider;
      } catch {
        // ignore
      }
    }
    if (deleteOldName) {
      mockMods = mockMods.filter(
        (m) =>
          m.name !== deleteOldName &&
          m.file !== deleteOldName &&
          m.file !== `${deleteOldName}.jar` &&
          m.name !== deleteOldName.replace(/\.jar$/i, ''),
      );
    }
    const newMod: Mod = {
      name,
      file,
      enabled: true,
      project_id: projectId,
      project_slug: projectSlug,
      provider,
      ...(file.includes('graves-3.12.0')
        ? {
            mod_id: 'universal-graves',
            title: 'Universal Graves',
            version: '3.12.0+26.2',
            sha1: '56c26a9318c739908dbd0c66cee85b842567c276',
          }
        : {}),
    };
    mockMods = [...mockMods.filter((m) => m.name !== name && m.file !== file), newMod];
    return newMod as T;
  }
  if (path.startsWith('/api/servers/') && path.includes('/mods/updates')) {
    return {
      updates: {},
      last_checked: new Date().toISOString(),
      total_mods: mockMods.length,
      update_count: 0,
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/versions')) {
    return [] as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/update') && init.method === 'POST') {
    let filename = 'updated-mod.jar';
    try {
      const parsed = JSON.parse(init.body as string);
      if (parsed.filename) filename = parsed.filename;
    } catch {
      // ignore
    }
    const parts = path.split('/');
    const oldName = decodeURIComponent(parts[parts.length - 2]);
    const updatedMod: Mod = {
      name: filename.replace(/\.jar$/i, ''),
      file: filename,
      enabled: true,
    };
    mockMods = [...mockMods.filter((m) => m.name !== oldName && m.file !== oldName), updatedMod];
    return updatedMod as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/mods')) {
    return {
      type: 'plugins',
      items: mockMods,
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.includes('/mods/')) {
    const parts = path.split('/');
    const modName = decodeURIComponent(parts[parts.length - 1]);
    if (init.method === 'DELETE') {
      mockMods = mockMods.filter((m) => m.name !== modName && m.file !== modName);
      return { ok: true } as T;
    }
    if (init.method === 'PATCH') {
      let enabled = true;
      if (init.body) {
        try {
          const parsed = JSON.parse(init.body as string);
          if (typeof parsed.enabled === 'boolean') enabled = parsed.enabled;
        } catch {
          // ignore
        }
      }
      mockMods = mockMods.map((m) =>
        m.name === modName || m.file === modName ? { ...m, enabled } : m,
      );
      const updated = mockMods.find((m) => m.name === modName || m.file === modName);
      return updated as T;
    }
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/backups')) {
    return {
      backups: [
        {
          id: 'b-1',
          server_id: 'demo',
          name: 'daily-snapshot-2026-09-09',
          size_bytes: 524288000,
          location: 's3://backups',
          status: 'completed',
          created_at: '2026-09-09T04:00:00Z',
        },
      ],
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/backups/progress')) {
    return { active: false } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/install')) {
    return {
      server_id: 'demo',
      installed: true,
      version: '1.21.4',
      build: '145',
    } as T;
  }
  if (path.includes('/command')) {
    return { ok: true, response: 'Command executed successfully.' } as T;
  }
  if (path === '/api/servers' && init.method === 'POST') {
    let name = 'New Server';
    let serverType: ServerType = 'paper';
    let version = '1.20.1';
    let build = '1';
    let ramMb = 2048;
    if (init.body) {
      try {
        const parsed = JSON.parse(init.body as string);
        if (parsed.name) name = parsed.name;
        if (parsed.server_type) serverType = parsed.server_type;
        if (parsed.version) version = parsed.version;
        if (parsed.build) build = parsed.build;
        if (parsed.ram_mb) ramMb = parsed.ram_mb;
      } catch {
        // ignore
      }
    }
    const newServer: Server = {
      id: `srv-${Date.now().toString(36)}`,
      name,
      server_type: serverType,
      version,
      build,
      ram_mb: ramMb,
      cpu_limit: 0,
      memory_limit_mb: 0,
      host_port: 25565,
      extra_ports: [],
      container_id: '',
      state: 'stopped',
      backup_enabled: false,
      backup_interval_minutes: 60,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    return newServer as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/modpack/install') && init.method === 'POST') {
    let createdWith = false;
    let projId = '';
    let projSlug = '';
    if (init.body) {
      try {
        const parsed = JSON.parse(init.body as string);
        if (parsed.created_with_modpack) createdWith = true;
        if (parsed.project_id) projId = parsed.project_id;
        if (parsed.project_slug) projSlug = parsed.project_slug;
      } catch {
        // ignore
      }
    }
    return {
      name: 'Installed Modpack',
      version: '1.0.0',
      format: 'modrinth',
      minecraft_version: '1.20.1',
      loader: 'fabric',
      loader_version: '0.15.11',
      installed_at: new Date().toISOString(),
      source: 'modrinth',
      project_id: projId,
      project_slug: projSlug,
      installed_files: [],
      created_with_modpack: createdWith,
    } as T;
  }
  if (path.startsWith('/api/servers/')) {
    return {
      id: 'demo',
      name: 'Mega SMP Server',
      server_type: 'paper',
      version: '1.21.4',
      build: '145',
      ram_mb: 8192,
      cpu_limit: 4,
      memory_limit_mb: 8192,
      host_port: 25565,
      extra_ports: [],
      container_id: 'mcm-srv-demo',
      state: 'running',
      backup_enabled: true,
      backup_interval_minutes: 60,
      started_at: '2026-01-01T00:00:00Z',
      uptime_seconds: 3600,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    } as T;
  }

  return null;
}

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const mockResult = handleMockRequest<T>(path, init);
  if (mockResult !== null) return mockResult;

  const method = init.method ?? 'GET';
  const headers: Record<string, string> = {
    ...(init.headers as Record<string, string> | undefined),
  };
  const isFormData = init.body instanceof FormData;
  if (!isFormData) headers['Content-Type'] = 'application/json';
  if (!safeMethods.has(method)) {
    const token = await ensureCsrf();
    if (token) headers['X-CSRF-Token'] = token;
  }
  const res = await fetch(path, {
    credentials: 'include',
    ...init,
    headers,
  });

  if (!res.ok) {
    const error = await responseError(res);
    if (res.status === 403 && (error.code === 'csrf_missing' || error.detail.toLowerCase().includes('csrf'))) {
      const freshToken = await ensureCsrf(true);
      if (freshToken) {
        headers['X-CSRF-Token'] = freshToken;
        const retryRes = await fetch(path, {
          credentials: 'include',
          ...init,
          headers,
        });
        if (retryRes.ok) {
          if (retryRes.status === 204) return undefined as T;
          return readResponse<T>(retryRes);
        }
        throw await responseError(retryRes);
      }
    }

    throw error;
  }

  if (res.status === 204) return undefined as T;
  return readResponse<T>(res);
}

async function responseError(response: Response): Promise<ApiError> {
  let detail = `Request failed (${response.status})`;
  let body;
  try {
    body = await response.json();
    detail = body?.error?.message ?? body?.message ?? detail;
  } catch {
    /* non-JSON error body */
  }
  if (response.status === 409 && body?.error?.code === 'file_changed' && body.current) {
    return new FileConflictError(detail, body.current);
  }
  return new ApiError(response.status, detail, detail, body?.error?.code);
}


async function readResponse<T>(response: Response): Promise<T> {
  try {
    return await response.json() as T;
  } catch {
    throw new ApiError(502, 'Could not read the server response. Try again.');
  }
}


export const api = {
  listSchedules: (id: string) => request<{ schedules: ScheduledTask[]; default_timezone: string }>(`/api/servers/${id}/schedules`),
  createSchedule: (id: string, input: ScheduleInput) => request<ScheduledTask>(`/api/servers/${id}/schedules`, { method: 'POST', body: JSON.stringify(input) }),
  updateSchedule: (id: string, scheduleId: string, input: ScheduleInput) => request<ScheduledTask>(`/api/servers/${id}/schedules/${scheduleId}`, { method: 'PUT', body: JSON.stringify(input) }),
  setScheduleEnabled: (id: string, scheduleId: string, enabled: boolean) => request<ScheduledTask>(`/api/servers/${id}/schedules/${scheduleId}`, { method: 'PATCH', body: JSON.stringify({ enabled }) }),
  deleteSchedule: (id: string, scheduleId: string) => request<{ ok: boolean }>(`/api/servers/${id}/schedules/${scheduleId}`, { method: 'DELETE' }),
  onboardingStatus: () => request<{ onboarding_required: boolean }>('/api/onboarding/status'),

  onboarding: (email: string, password: string) =>
    request<Me>('/api/onboarding', { method: 'POST', body: JSON.stringify({ email, password }) }),

  login: (email: string, password: string) =>
    request<Me>('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),

  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  me: () => request<Me>('/api/auth/me'),

  listServers: () =>
    request<{ servers: Server[] }>('/api/servers').then((res) => res.servers ?? []),

  hostCapacity: () => request<HostCapacity>('/api/system/capacity'),

  createServer: (input: CreateServerInput) =>
    request<Server>('/api/servers', { method: 'POST', body: JSON.stringify(input) }),

  copyServer: (id: string, input: CopyServerInput) =>
    request<Server>(`/api/servers/${id}/copy`, { method: 'POST', body: JSON.stringify(input) }),

  getServer: (id: string) => request<Server>(`/api/servers/${id}`),

  updateServer: (id: string, input: Partial<UpdateServerInput>) =>
    request<Server>(`/api/servers/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  deleteServer: (id: string) => request<void>(`/api/servers/${id}`, { method: 'DELETE' }),

  startServer: (id: string) => request<Server>(`/api/servers/${id}/start`, { method: 'POST' }),
  stopServer: (id: string) => request<Server>(`/api/servers/${id}/stop`, { method: 'POST' }),
  killServer: (id: string) => request<Server>(`/api/servers/${id}/kill`, { method: 'POST' }),
  restartServer: (id: string) => request<Server>(`/api/servers/${id}/restart`, { method: 'POST' }),
  recreateServer: (id: string) => request<Server>(`/api/servers/${id}/recreate`, { method: 'POST' }),

  serverStatus: (id: string) => request<ServerStatus>(`/api/servers/${id}/status`),

  performanceHistory: (id: string, range: PerformanceRange = '1h') =>
    request<PerformanceHistory>(`/api/servers/${id}/performance?range=${range}`),

  setPerformanceCollection: (id: string, mode: PerformanceCollectionMode) =>
    request<PerformanceSettings>(`/api/servers/${id}/performance/settings`, { method: 'PUT', body: JSON.stringify({ collection_mode: mode }) }),

  captureSparkReport: (id: string) =>
    request<SparkReport>(`/api/servers/${id}/performance/spark`, { method: 'POST' }),

  serverStats: (id: string) => {
    if (isMock()) {
      return Promise.resolve<ServerStats>({
        server_id: id,
        online: true,
        cpu_percent: 14.8,
        cpu_cores: 4,
        cpu_limit: 2,
        memory_bytes: 1845493760,
        memory_limit_bytes: 4294967296,
        memory_percent: 42.97,
        disk_bytes: 471859200,
        disk_read_bytes: 10485760,
        disk_write_bytes: 25165824,
        net_rx_bytes: 12582912,
        net_tx_bytes: 34603008,
      });
    }
    return request<ServerStats>(`/api/servers/${id}/stats`);
  },

  openConsoleStream: (id: string, onLine: (line: ConsoleLine) => void): (() => void) => {
    if (isMock()) {
      const mockLines: ConsoleLine[] = [
        { timestamp: '12:00:01', level: 'INFO', message: 'Starting minecraft server version 1.21.4 (Paper #145)' },
        { timestamp: '12:00:03', level: 'INFO', message: 'Loading server.properties' },
        { timestamp: '12:00:05', level: 'INFO', message: 'Default game type: SURVIVAL' },
        { timestamp: '12:00:08', level: 'INFO', message: 'Preparing start region for dimension minecraft:overworld' },
        { timestamp: '12:00:12', level: 'INFO', message: 'Done (11.84s)! For help, type "help"' },
        { timestamp: '12:00:13', level: 'INFO', message: '[Essentials] Enabling Essentials v2.20.1' },
        { timestamp: '12:00:14', level: 'INFO', message: '[LuckPerms] Enabling LuckPerms v5.4.102' },
        { timestamp: '12:00:30', level: 'INFO', message: 'Player_001 joined the game' },
        { timestamp: '12:00:45', level: 'INFO', message: 'There are 100 of a max 150 players online' },
      ];
      const timer = setTimeout(() => {
        mockLines.forEach((l) => onLine(l));
      }, 50);
      return () => clearTimeout(timer);
    }
    const source = new EventSource(`/api/servers/${id}/console`, { withCredentials: true });
    source.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as ConsoleLine;
        if (parsed && typeof parsed.message === 'string') {
          onLine(parsed);
        }
      } catch {
        /* ignore malformed events */
      }
    };
    return () => source.close();
  },


  consoleCommand: (id: string, command: string) =>
    request<{ ok: boolean }>(`/api/servers/${id}/console/command`, {
      method: 'POST',
      body: JSON.stringify({ command }),
    }),

  installInfo: (id: string) => request<InstallInfo>(`/api/servers/${id}/install`),

  installServer: (id: string, input: InstallInput) =>
    request<InstallInfo>(`/api/servers/${id}/install`, { method: 'POST', body: JSON.stringify(input) }),

  jarVersions: (type: ServerType) => request<VersionMeta[]>(`/api/jars/${type}/versions`),

  jarBuilds: (type: ServerType, version: string) =>
    request<VersionInfo[]>(`/api/jars/${type}/versions/${encodeURIComponent(version)}/builds`),

  javaVersions: () => request<JavaRelease[]>('/api/system/java-versions'),

  availablePorts: () => request<AvailablePortsResponse>('/api/ports/available'),

  checkPort: (port: number, serverId?: string, protocol?: string) => {
    const params = new URLSearchParams({ port: String(port) });
    if (serverId) params.set('server_id', serverId);
    if (protocol) params.set('protocol', protocol);
    return request<PortCheckResponse>(`/api/ports/check?${params.toString()}`);
  },

  getDiscordSettings: () => request<DiscordSettings>('/api/notifications/discord'),

  saveDiscordSettings: (settings: DiscordSettingsInput) =>
    request<DiscordSettings>('/api/notifications/discord', { method: 'PUT', body: JSON.stringify(settings) }),

  testDiscordWebhook: (webhook_url?: string) =>
    request<{ ok: boolean; message: string }>('/api/notifications/discord/test', {
      method: 'POST', body: JSON.stringify(webhook_url === undefined ? {} : { webhook_url }),
    }),

  getSettings: () => request<{ settings: Record<string, string> }>('/api/settings'),

  putSettings: (settings: Record<string, string>) =>
    request<{ settings: Record<string, string> }>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({ settings }),
    }),

  getDNS: () => request<DNSStatusResponse>('/api/dns'),

  testDNS: (creds?: { api_token?: string; zone?: string; domain?: string }) =>
    request<DNSTestResult>('/api/dns/test', {
      method: 'POST',
      body: JSON.stringify(creds ?? {}),
    }),

  getServerDNS: (serverId: string) =>
    request<ServerDNSResponse>(`/api/servers/${serverId}/dns`),

  publishServerDNS: (serverId: string, input?: PublishDNSInput) =>
    request<{ ok: boolean; record: DNSRecord; join_address: string }>(
      `/api/servers/${serverId}/dns`,
      {
        method: 'POST',
        body: JSON.stringify(input ?? {}),
      },
    ),

  removeServerDNS: (serverId: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/dns`, { method: 'DELETE' }),

  listUsers: () =>
    request<{ users: User[] }>('/api/users').then((res) => res.users ?? []),

  createUser: (email: string, password: string) =>
    request<User>('/api/users', { method: 'POST', body: JSON.stringify({ email, password }) }),

  updateUser: (id: string, input: { email?: string; password?: string }) =>
    request<User>(`/api/users/${id}`, { method: 'PATCH', body: JSON.stringify(input) }),

  deleteUser: (id: string) =>
    request<{ ok: boolean }>(`/api/users/${id}`, { method: 'DELETE' }),

  totpStatus: () => request<{ totp_enabled: boolean }>('/api/auth/totp'),

  totpEnroll: () =>
    request<{ secret: string; qr_uri: string }>('/api/auth/totp/enroll', {
      method: 'POST',
    }),

  totpConfirm: (code: string) =>
    request<{ totp_enabled: boolean }>('/api/auth/totp/enroll/confirm', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  totpDisable: (code: string) =>
    request<{ totp_enabled: boolean }>('/api/auth/totp/disable', {
      method: 'POST',
      body: JSON.stringify({ code }),
    }),

  passkeyList: () => request<PasskeyMeta[]>('/api/passkey'),

  passkeyRegisterBegin: () =>
    request<{ registration_id: string; options: PublicKeyCredentialCreationOptions }>(
      '/api/passkey/register/begin',
      { method: 'POST' },
    ),

  passkeyRegisterFinish: (registration_id: string, credential: unknown, name?: string) =>
    request<{ ok: boolean }>('/api/passkey/register/finish', {
      method: 'POST',
      body: JSON.stringify({ registration_id, name, ...(credential as object) }),
    }),

  passkeyDelete: (id: string) =>
    request<{ ok: boolean }>('/api/passkey', { method: 'DELETE', body: JSON.stringify({ id }) }),

  listBackups: (serverId: string) =>
    request<{ backups: BackupRecord[] }>(`/api/servers/${serverId}/backups`),

  players: (serverId: string) => request<PlayerList>(`/api/servers/${serverId}/players`),

  runPlayerCommand: (
    serverId: string,
    name: string,
    action: PlayerCommandAction,
    args: PlayerCommandArgs = {},
  ) =>
    request<{ ok: boolean; response: string }>(
      `/api/servers/${serverId}/players/${encodeURIComponent(name)}/command`,
      {
        method: 'POST',
        body: JSON.stringify({ action, args }),
      },
    ),

  ops: (serverId: string) => request<{ ops: Op[] }>(`/api/servers/${serverId}/ops`),

  addOp: (serverId: string, name: string, level?: number) =>
    request<{ ops: Op[] }>(`/api/servers/${serverId}/ops`, {
      method: 'POST',
      body: JSON.stringify({ name, level: level ?? 4 }),
    }),

  removeOp: (serverId: string, name: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/ops/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),

  whitelist: (serverId: string) =>
    request<{ whitelist: WhitelistEntry[] }>(`/api/servers/${serverId}/whitelist`),

  addWhitelist: (serverId: string, name: string) =>
    request<{ whitelist: WhitelistEntry[] }>(`/api/servers/${serverId}/whitelist`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),

  removeWhitelist: (serverId: string, name: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/whitelist/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),

  mods: (serverId: string) => request<ModList>(`/api/servers/${serverId}/mods`),

  checkModUpdates: (serverId: string, force?: boolean) => {
    const cfHeader = typeof window !== 'undefined' && localStorage.getItem('mcm_curseforge_api_key')
      ? { 'x-curseforge-api-key': localStorage.getItem('mcm_curseforge_api_key')!.trim() }
      : undefined;
    return request<ServerModUpdatesResponse>(
      `/api/servers/${serverId}/mods/updates${force ? '?force=true' : ''}`,
      { headers: cfHeader },
    );
  },

  checkModUpdatesForce: (serverId: string) => {
    const cfHeader = typeof window !== 'undefined' && localStorage.getItem('mcm_curseforge_api_key')
      ? { 'x-curseforge-api-key': localStorage.getItem('mcm_curseforge_api_key')!.trim() }
      : undefined;
    return request<ServerModUpdatesResponse>(
      `/api/servers/${serverId}/mods/updates/check`,
      { method: 'POST', headers: cfHeader },
    );
  },

  checkVersionUpgradeCompatibility: (serverId: string, targetVersion: string, force = false) => {
    const cfHeader = typeof window !== 'undefined' && localStorage.getItem('mcm_curseforge_api_key')
      ? { 'x-curseforge-api-key': localStorage.getItem('mcm_curseforge_api_key')!.trim() }
      : undefined;
    const q = new URLSearchParams();
    if (targetVersion) q.set('target_version', targetVersion);
    if (force) q.set('force', 'true');
    return request<VersionUpdateReport>(
      `/api/servers/${serverId}/mods/upgrade-check?${q.toString()}`,
      { headers: cfHeader },
    );
  },

  applyVersionUpgradeMods: (serverId: string, targetVersion: string, modNames?: string[]) =>
    request<BatchUpdateModsResult>(
      `/api/servers/${serverId}/mods/upgrade-apply`,
      {
        method: 'POST',
        body: JSON.stringify({
          target_version: targetVersion,
          mods: modNames || [],
        }),
      },
    ),

  batchUpdateMods: (serverId: string, modNames?: string[]) =>
    request<BatchUpdateModsResult>(
      `/api/servers/${serverId}/mods/batch-update`,
      {
        method: 'POST',
        body: JSON.stringify({
          mods: modNames || [],
        }),
      },
    ),

  getModAvailableVersions: (serverId: string, modName: string) => {
    const cfHeader = typeof window !== 'undefined' && localStorage.getItem('mcm_curseforge_api_key')
      ? { 'x-curseforge-api-key': localStorage.getItem('mcm_curseforge_api_key')!.trim() }
      : undefined;
    return request<AvailableModJar[]>(
      `/api/servers/${serverId}/mods/${encodeURIComponent(modName)}/versions`,
      { headers: cfHeader },
    );
  },

  updateModVersion: (
    serverId: string,
    modName: string,
    url: string,
    filename: string,
    meta?: { deleteOld?: boolean; projectId?: string; projectSlug?: string; provider?: ModProvider },
  ) =>
    request<Mod>(
      `/api/servers/${serverId}/mods/${encodeURIComponent(modName)}/update`,
      {
        method: 'POST',
        body: JSON.stringify({
          url,
          filename,
          delete_old: meta?.deleteOld ?? true,
          project_id: meta?.projectId,
          project_slug: meta?.projectSlug,
          provider: meta?.provider,
        }),
      },
    ),

  downloadMod: (
    serverId: string,
    url: string,
    filename: string,
    deleteOldName?: string,
    meta?: { projectId?: string; projectSlug?: string; provider?: ModProvider },
  ) =>
    request<Mod>(`/api/servers/${serverId}/mods/download`, {
      method: 'POST',
      body: JSON.stringify({
        url,
        filename,
        delete_old_name: deleteOldName,
        project_id: meta?.projectId,
        project_slug: meta?.projectSlug,
        provider: meta?.provider,
      }),
    }),

  uploadMod: (
    serverId: string,
    file: File,
    onProgress?: (loaded: number, total: number) => void,
    deleteOldName?: string,
    meta?: { projectId?: string; projectSlug?: string; provider?: ModProvider },
  ) =>
    uploadWithProgress<Mod>(
      `/api/servers/${serverId}/mods`,
      [
        ['file', file],
        ...(deleteOldName ? [['delete_old_name', deleteOldName] as [string, string]] : []),
        ...(meta?.projectId ? [['project_id', meta.projectId] as [string, string]] : []),
        ...(meta?.projectSlug ? [['project_slug', meta.projectSlug] as [string, string]] : []),
        ...(meta?.provider ? [['provider', meta.provider] as [string, string]] : []),
      ],
      onProgress,
    ),

  setModEnabled: (serverId: string, name: string, enabled: boolean) =>
    request<Mod>(`/api/servers/${serverId}/mods/${encodeURIComponent(name)}`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    }),

  deleteMod: (serverId: string, name: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/mods/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),

  getProperties: (serverId: string) =>
    request<ServerProperties>(`/api/servers/${serverId}/properties`),

  saveProperties: (serverId: string, content: string) =>
    request<ServerProperties>(`/api/servers/${serverId}/properties`, {
      method: 'PUT',
      body: JSON.stringify({ content }),
    }),

  createBackup: (serverId: string, name?: string, storage?: 'local' | 's3') =>
    request<BackupRecord>(`/api/servers/${serverId}/backup`, {
      method: 'POST',
      body: JSON.stringify({ name: name ?? '', storage: storage ?? '' }),
    }),

  getBackupProgress: (serverId: string) =>
    request<BackupProgressResponse>(`/api/servers/${serverId}/backups/progress`),

  subscribeBackupEvents: (
    serverId: string,
    onProgress: (p: BackupProgress) => void,
    onError?: (err: Event) => void,
  ) => {
    if (isMock() || typeof EventSource === 'undefined') return () => {};
    const source = new EventSource(`/api/servers/${serverId}/backups/events`, {
      withCredentials: true,
    });
    source.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as BackupProgress;
        onProgress(data);
      } catch {
        /* ignore parse errors on comments/heartbeats */
      }
    };
    if (onError) {
      source.onerror = onError;
    }
    return () => source.close();
  },

  getTaskProgress: (serverId: string) =>
    request<TaskProgressResponse>(`/api/servers/${serverId}/tasks/progress`),

  subscribeTaskEvents: (
    serverId: string,
    onProgress: (p: TaskProgress) => void,
    onError?: (err: Event) => void,
  ) => {
    if (typeof EventSource === 'undefined') {
      return () => {};
    }
    const source = new EventSource(`/api/servers/${serverId}/tasks/events`, {
      withCredentials: true,
    });
    source.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data) as TaskProgress;
        onProgress(data);
      } catch {
        /* ignore parse errors on comments/heartbeats */
      }
    };
    if (onError) {
      source.onerror = onError;
    }
    return () => source.close();
  },

  uploadBackup: (
    serverId: string,
    file: File,
    name?: string,
    storage?: 'local' | 's3',
    onProgress?: (loaded: number, total: number) => void,
  ) => {
    const fields: [string, string | Blob][] = [['file', file]];
    if (name) fields.push(['name', name]);
    if (storage) fields.push(['storage', storage]);
    return uploadWithProgress<BackupRecord>(
      `/api/servers/${serverId}/backups/upload`,
      fields,
      onProgress,
    );
  },

  downloadBackupFile: (backupId: string, filename?: string) => {
    const a = document.createElement('a');
    a.href = `/api/backups/${backupId}/download`;
    a.download = filename ?? `backup-${backupId}.tar.gz`;
    document.body.appendChild(a);
    a.click();
    a.remove();
  },

  restoreBackup: (serverId: string, backupId: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/restore/${backupId}`, { method: 'POST' }),

  deleteBackup: (backupId: string) =>
    request<{ ok: boolean }>(`/api/backups/${backupId}`, { method: 'DELETE' }),

  listServerLogs: (serverId: string, archivePath?: string) => {
    const q = archivePath ? `?path=${encodeURIComponent(archivePath)}` : '';
    return request<{ files: ServerLogFile[] }>(`/api/servers/${serverId}/logs${q}`);
  },

  readServerLog: (serverId: string, path: string, entry?: string) => {
    const q = new URLSearchParams({ path });
    if (entry) q.set('entry', entry);
    return request<ServerLogContent>(`/api/servers/${serverId}/logs/content?${q}`);
  },

  listFiles: (serverId: string, path: string) => {
    const q = path ? `?path=${encodeURIComponent(path)}` : '';
    return request<FileList>(`/api/servers/${serverId}/files${q}`);
  },

  downloadFile: async (serverId: string, path: string): Promise<Blob> => {
    const res = await fetch(`/api/servers/${serverId}/files/download?path=${encodeURIComponent(path)}`, {
      credentials: 'include',
    });
    if (!res.ok) {
      let detail = `Download failed (${res.status})`;
      try {
        const body = await res.json();
        detail = body?.error?.message ?? detail;
      } catch {
        /* non-JSON body */
      }
      throw new ApiError(res.status, detail, detail);
    }
    return res.blob();
  },

  uploadFiles: (
    serverId: string,
    files: File[],
    dir: string,
    onProgress?: (loaded: number, total: number) => void,
  ): Promise<FileEntry[]> => {
    const q = dir ? `?dir=${encodeURIComponent(dir)}` : '';
    const parts: Array<[string, Blob | string]> = [];
    for (const file of files) {
      parts.push(['file', file]);
    }
    return uploadWithProgress<FileEntry[]>(
      `/api/servers/${serverId}/files/upload${q}`,
      parts,
      onProgress,
    );
  },

  archiveFile: (serverId: string, source: string, name?: string, dir?: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/archive`, {
      method: 'POST',
      body: JSON.stringify({ source, name: name ?? '', dir: dir ?? '' }),
    }),

  archiveFiles: (serverId: string, sources: string[], dir: string, name?: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/archive`, {
      method: 'POST',
      body: JSON.stringify({ sources, dir, name: name ?? '' }),
    }),

  exportServer: (serverId: string, serverName?: string) => {
    const a = document.createElement('a');
    a.href = `/api/servers/${serverId}/export`;
    a.download = serverName ? `${serverName}-export.zip` : `server-export.zip`;
    document.body.appendChild(a);
    a.click();
    a.remove();
  },

  unzipFile: (serverId: string, archive: string, dest?: string) =>
    request<UnzipResult>(`/api/servers/${serverId}/files/unzip`, {
      method: 'POST',
      body: JSON.stringify({ archive, dest: dest ?? '' }),
    }),

  downloadFromUrl: (serverId: string, url: string, dir: string, name?: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/from_url`, {
      method: 'POST',
      body: JSON.stringify({ url, dir, name: name ?? '' }),
    }),

  readFileContent: (serverId: string, path: string) =>
    request<FileContent>(`/api/servers/${serverId}/files/content?path=${encodeURIComponent(path)}`),

  writeFileContent: (serverId: string, path: string, content: string, expectedRevision: string) =>
    request<FileEntry & { revision: string }>(`/api/servers/${serverId}/files/content`, {
      method: 'PUT',
      body: JSON.stringify({ path, content, expected_revision: expectedRevision }),
    }),

  deleteFile: (serverId: string, path: string) =>
    request<{ ok: boolean }>(
      `/api/servers/${serverId}/files?path=${encodeURIComponent(path)}`,
      { method: 'DELETE' },
    ),

  mkdir: (serverId: string, path: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/mkdir`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),

  renameFile: (serverId: string, path: string, name: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/rename`, {
      method: 'POST',
      body: JSON.stringify({ path, name }),
    }),

  getInstalledModpack: (serverId: string) =>
    request<InstalledModpackResponse>(`/api/servers/${serverId}/modpack`),

  inspectModpackFile: (
    serverId: string,
    file: File,
    onProgress?: (loaded: number, total: number) => void,
  ) =>
    uploadWithProgress<ModpackManifest>(
      `/api/servers/${serverId}/modpack/inspect`,
      [['file', file]],
      onProgress,
    ),

  inspectModpackUpload: (
    file: File,
    onProgress?: (loaded: number, total: number) => void,
  ) =>
    uploadWithProgress<ModpackManifest>(
      '/api/modpack/inspect',
      [['file', file]],
      onProgress,
    ),

  inspectModpackUrl: (serverId: string, url: string) =>
    request<ModpackManifest>(`/api/servers/${serverId}/modpack/inspect`, {
      method: 'POST',
      body: JSON.stringify({ url }),
    }),

  installModpackFile: (
    serverId: string,
    file: File,
    autoConfigureServer = true,
    createdWithModpack = false,
    onProgress?: (loaded: number, total: number) => void,
  ) => {
    const cfKey = typeof window !== 'undefined' ? localStorage.getItem('mcm_curseforge_api_key') : null;
    return uploadWithProgress<InstalledModpack>(
      `/api/servers/${serverId}/modpack/install`,
      [
        ['file', file],
        ['auto_configure_server', String(autoConfigureServer)],
        ['created_with_modpack', String(createdWithModpack)],
        ...(cfKey ? [['curseforge_api_key', cfKey] as [string, string]] : []),
      ],
      onProgress,
    );
  },

  installModpackRemote: (serverId: string, options: InstallModpackOptions) => {
    const cfKey =
      options.curseforge_api_key ||
      (typeof window !== 'undefined' ? localStorage.getItem('mcm_curseforge_api_key') ?? undefined : undefined);
    return request<InstalledModpack>(`/api/servers/${serverId}/modpack/install`, {
      method: 'POST',
      headers: cfKey ? { 'x-curseforge-api-key': cfKey } : undefined,
      body: JSON.stringify({
        ...options,
        curseforge_api_key: cfKey,
      }),
    });
  },

  uninstallModpack: (serverId: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/modpack`, {
      method: 'DELETE',
    }),
};

export interface CreateServerInput {
  name: string;
  server_type: ServerType;
  version: string;
  build: string;
  ram_mb: number;
  host_port?: number;
  no_host_port?: boolean;
  java_version?: number;
  extra_ports?: ExtraPort[];
}

export interface UpdateServerInput {
  name: string;
  ram_mb: number;
  host_port?: number;
  java_version?: number;
  cpu_limit?: number;
  memory_limit_mb?: number;
  backup_enabled?: boolean;
  backup_interval_minutes?: number;
  extra_ports?: ExtraPort[];
}

export interface InstallInput {
  version: string;
  build: string;
}

// uploadWithProgress uploads a multipart body while reporting progress. The
// endpoint must return the JSON type T on success.
export async function uploadWithProgress<T>(
  path: string,
  parts: Array<[string, Blob | string]>,
  onProgress?: (loaded: number, total: number) => void,
): Promise<T> {
  const body = new FormData();
  for (const [key, value] of parts) {
    body.append(key, value);
  }
  const token = await ensureCsrf();
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', path);
    xhr.withCredentials = true;
    if (token) xhr.setRequestHeader('X-CSRF-Token', token);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress?.(e.loaded, e.total);
    };
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(xhr.responseText ? (JSON.parse(xhr.responseText) as T) : (undefined as T));
        } catch {
          reject(new ApiError(xhr.status, 'Invalid response'));
        }
      } else {
        let detail = `Upload failed (${xhr.status})`;
        try {
          const body = JSON.parse(xhr.responseText);
          detail = body?.error?.message ?? body?.message ?? detail;
        } catch {
          /* non-JSON body */
        }
        reject(new ApiError(xhr.status, detail, detail));
      }
    };
    xhr.onerror = () => reject(new ApiError(0, 'Network error during upload'));
    xhr.send(body);
  });
}
