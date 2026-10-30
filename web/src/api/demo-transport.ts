import { demoPermissionCatalog } from './demo-permissions';
import { dnsProviders, isDNSProvider } from '../lib/dns-providers';
import type { BackupPolicy, BackupRecord, BackupRetentionPreview, PanelUpdateStatus, UpdateCheckerStatus, DiscordSettings, DiscordSettingsInput, ScheduledTask, Server, PerformanceCollectionMode, PerformanceSettings, SparkReport, ServerType, Mod, ModProvider, DNSRecord } from './types';

export function isMock(): boolean {
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

const mockDNSPreferences: Record<string, string> = { demo: 'demo' };

function mockDNSConfig() {
  const provider = isDNSProvider(mockDNSSettings.dns_provider ?? '') ? mockDNSSettings.dns_provider as keyof typeof dnsProviders : 'cloudflare';
  const info = dnsProviders[provider];
  const token = mockDNSSettings[info.tokenKey];
  const secret = info.secretKey ? mockDNSSettings[info.secretKey] : true;
  const zone = mockDNSSettings[info.zoneKey] || ((provider === 'porkbun' || provider === 'digitalocean') ? mockDNSSettings.dns_domain : '');
  return { provider, zone, has_token: !!token, configured: mockDNSSettings.dns_publish === 'true' && !!token && !!secret && !!zone && !!mockDNSSettings.dns_domain };
}

let mockSchedules: ScheduledTask[] = [];
const mockBackupPolicies: Record<string, BackupPolicy> = {};
const mockBackups: Record<string, BackupRecord[]> = {};
function demoBackups(serverId: string) {
  return mockBackups[serverId] ??= [
    { id: `${serverId}-latest`, server_id: serverId, name: 'daily-snapshot', size_bytes: 524288000, location: 'local:latest', status: 'completed', created_at: new Date().toISOString() },
    { id: `${serverId}-protected`, server_id: serverId, name: 'before-version-upgrade', size_bytes: 498073600, location: 's3:protected', status: 'completed', protected: true, created_at: new Date(Date.now() - 7 * 86400000).toISOString() },
    { id: `${serverId}-old`, server_id: serverId, name: 'older-daily-snapshot', size_bytes: 471859200, location: 'local:old', status: 'completed', created_at: new Date(Date.now() - 20 * 86400000).toISOString() },
    { id: `${serverId}-trash`, server_id: serverId, name: 'previous-week-snapshot', size_bytes: 450887680, location: 'local:trash', status: 'completed', created_at: new Date(Date.now() - 40 * 86400000).toISOString(), deleted_at: new Date(Date.now() - 31 * 86400000).toISOString() },
  ];
}
function demoRetention(serverId: string): BackupRetentionPreview {
  const policy = mockBackupPolicies[serverId] ?? { keep_count: 10, max_age_days: 0, trash_retention_days: 30 };
  const records = demoBackups(serverId);
  const completed = records.filter((b) => !b.deleted_at && b.status === 'completed');
  let count = 0;
  const toTrash: BackupRetentionPreview['to_trash'] = [];
  for (const b of completed) {
    if (b.protected) continue;
    count++;
    if (b === completed[0]) continue;
    const age = policy.max_age_days > 0 && Date.parse(b.created_at) <= Date.now() - policy.max_age_days * 86400000;
    if (age || (policy.keep_count > 0 && count > policy.keep_count)) toTrash.push({ ...b, reason: age ? 'age' : 'count' });
  }
  const toPurge = records.filter((b) => b.deleted_at && policy.trash_retention_days > 0 && Date.parse(b.deleted_at) <= Date.now() - policy.trash_retention_days * 86400000);
  return { policy, to_trash: toTrash, to_purge: toPurge, trash_bytes: toTrash.reduce((n, b) => n + b.size_bytes, 0), purge_bytes: toPurge.reduce((n, b) => n + b.size_bytes, 0) };
}
const mockImportedServers: Server[] = [];
let mockDiscordSettings: DiscordSettings = {
  enabled: false, configured: false,
  events: ['server_started', 'server_stopped', 'server_crashed', 'server_error', 'backup_completed', 'backup_failed'],
};
const mockSparkReports: Record<string, SparkReport> = {};
const mockPerformanceModes: Record<string, PerformanceCollectionMode> = {};
let mockUpdateChecker: UpdateCheckerStatus = { enabled: false, running: false, timezone: 'UTC', servers: {} };
let mockPanelUpdate: PanelUpdateStatus = {
  build: { commit: '75f02c91d63c92478986b325ac5ddfde48f2cb31', built_at: '2026-10-28T03:30:00Z', dirty: false },
  supported: true, available: false,
};

export function handleMockRequest<T>(path: string, init: RequestInit = {}): T | null {
  if (!isMock()) return null;

  const jarCatalog = path.match(/^\/api\/jars\/([^/]+)\/versions(?:\/([^/]+)\/builds)?$/);
  if (jarCatalog) {
    const [, type, version] = jarCatalog;
    if (!version) return [{ name: '1.21.4' }, { name: '1.21.1' }, { name: '1.20.1' }] as T;
    const builds = type === 'fabric' || type === 'quilt' ? ['0.16.10', '0.16.9'] : ['145', '144', '143'];
    return builds.map((build) => ({ version: decodeURIComponent(version), build, display: build })) as T;
  }

  if (path === '/api/panel/update/check') {
    mockPanelUpdate = { ...mockPanelUpdate, latest_commit: 'adb7819c46d63b13405b2fd259a90adf9480157c', checked_at: new Date().toISOString(), available: true };
    return mockPanelUpdate as T;
  }
  if (path === '/api/panel/update') {
    if (init.method === 'POST') {
      mockPanelUpdate = { ...mockPanelUpdate, job: { state: 'succeeded', started_at: new Date().toISOString(), message: 'Demo update complete. No installation was changed.' }, log: 'Demo: build, backup, and readiness check completed.' };
    }
    return mockPanelUpdate as T;
  }

  if (path === '/api/updates/settings' && init.method === 'PUT') {
    const { enabled } = JSON.parse(String(init.body)) as { enabled: boolean };
    const midnight = new Date(); midnight.setUTCHours(24, 0, 0, 0);
    mockUpdateChecker = { ...mockUpdateChecker, enabled, next_check_at: enabled ? midnight.toISOString() : undefined };
    return mockUpdateChecker as T;
  }
  if (path === '/api/updates/check') {
    mockUpdateChecker = { ...mockUpdateChecker, last_checked: new Date().toISOString(), servers: {
      demo: { checked_at: new Date().toISOString(), software: { supported: true, available: true, latest_version: '1.21.5' } },
    } };
    return { accepted: true } as T;
  }
  if (path === '/api/updates') return mockUpdateChecker as T;

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
        ...mockDNSConfig(),
        publish: mockDNSSettings.dns_publish === 'true',
        domain: mockDNSSettings.dns_domain || 'example.com',
        host: mockDNSSettings.dns_host || 'mc-node1.example.com',
        service: mockDNSSettings.dns_service || '_minecraft',
        proto: mockDNSSettings.dns_proto || '_tcp',
        ttl: Number(mockDNSSettings.dns_ttl) || 120,
        priority: Number(mockDNSSettings.dns_priority) || 0,
        weight: Number(mockDNSSettings.dns_weight) || 5,
      },
      configured: mockDNSConfig().configured,
    } as T;
  }

  if (path.startsWith('/api/servers/') && path.endsWith('/dns')) {
    const serverId = path.split('/')[3];
    if (init.method === 'DELETE') {
      mockDNSRecords = mockDNSRecords.filter((r) => r.server_id !== serverId);
      return { ok: true } as T;
    }
    if (init.method === 'POST') {
      let sub = mockDNSPreferences[serverId] || 'demo';
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
      mockDNSPreferences[serverId] = sub;
      const rec: DNSRecord = {
        provider: mockDNSConfig().provider,
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
    const joinAddress = rec ? rec.name.split('.').slice(2).join('.') : '';
    return {
      record: rec,
      subdomain: mockDNSPreferences[serverId] || rec?.subdomain || '',
      provider: mockDNSConfig().provider,
      configured: mockDNSConfig().configured,
      domain,
      server_id: serverId,
      server_name: 'Mega SMP Server',
      host_port: 25565,
      join_address: joinAddress,
    } as T;
  }

  if (path === '/api/auth/totp' && (!init.method || init.method === 'GET')) return { totp_enabled: false } as T;
  if (path === '/api/passkey' && (!init.method || init.method === 'GET')) return [] as T;

  if (path === '/api/auth/me') {
    return { id: 'admin-1', email: 'admin@mcm.panel', role: 'administrator', permissions: demoPermissionCatalog.roles[0].permissions, all_servers: true, server_ids: [], disabled: false } as T;
  }
  if (path === '/api/permissions') return demoPermissionCatalog as T;
  if (path === '/api/users' && (!init.method || init.method === 'GET')) {
    return { users: [{ id: 'admin-1', email: 'admin@mcm.panel', role: 'administrator', permissions: demoPermissionCatalog.roles[0].permissions, all_servers: true, server_ids: [], disabled: false, totp_enabled: false, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' }] } as T;
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
        ...mockImportedServers,
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
  if (path === '/api/servers/import' && init.method === 'POST' && init.body instanceof FormData) {
    const settings = JSON.parse(String(init.body.get('settings') ?? '{}')) as Partial<Server> & { no_host_port?: boolean };
    const server: Server = {
      id: `import-${Date.now().toString(36)}`, name: settings.name || 'Imported demo server',
      server_type: settings.server_type || 'paper', version: settings.version || '1.21.4', build: settings.build || '',
      ram_mb: settings.ram_mb || 2048, java_version: settings.java_version || 21,
      cpu_limit: 0, memory_limit_mb: 0, host_port: settings.no_host_port ? 0 : 25566,
      extra_ports: [], container_id: '', state: 'stopped',
      created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    };
    mockImportedServers.push(server);
    return server as T;
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
  const backupRoute = path.match(/^\/api\/servers\/([^/]+)\/backups(?:\/(retention|cleanup|trash))?$/);
  if (backupRoute) {
    const [, serverId, action] = backupRoute;
    const records = demoBackups(serverId);
    if (action === 'retention') {
      if (init.method === 'PUT') mockBackupPolicies[serverId] = JSON.parse(String(init.body));
      return demoRetention(serverId) as T;
    }
    if (action === 'cleanup' || action === 'trash') {
      const preview = demoRetention(serverId);
      const purge = action === 'trash' ? records.filter((b) => b.deleted_at) : preview.to_purge;
      const move = action === 'trash' ? [] : preview.to_trash;
      mockBackups[serverId] = records.filter((b) => !purge.some((p) => p.id === b.id)).map((b) => move.some((m) => m.id === b.id) ? { ...b, deleted_at: new Date().toISOString() } : b);
      return { trashed: move.length, purged: purge.length } as T;
    }
    return { backups: records.filter((b) => !b.deleted_at), trash: records.filter((b) => b.deleted_at), policy: demoRetention(serverId).policy } as T;
  }
  const backupAction = path.match(/^\/api\/backups\/([^/]+)(?:\/(recover|protect|purge))?$/);
  if (backupAction) {
    const [, id, action] = backupAction;
    for (const [serverId, records] of Object.entries(mockBackups)) {
      const b = records.find((b) => b.id === id);
      if (!b) continue;
      if (action === 'purge') mockBackups[serverId] = records.filter((r) => r.id !== id);
      else if (action === 'recover') { b.deleted_at = ''; b.protected = b.status === 'completed'; }
      else if (action === 'protect') b.protected = JSON.parse(String(init.body)).protected;
      else b.deleted_at = new Date().toISOString();
    }
    return { ok: true } as T;
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
      started_at: '2026-01-01T00:00:00Z',
      uptime_seconds: 3600,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    } as T;
  }

  throw new Error(`This action is unavailable in the demo: ${path}`);
}
