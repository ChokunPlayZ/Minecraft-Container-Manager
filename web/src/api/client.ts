import type {
  ConsoleLine,
  ExtraPort,
  InstallInfo,
  Me,
  Server,
  ServerStatus,
  ServerType,
  Settings,
  BackupRecord,
  VersionInfo,
  VersionMeta,
  Op,
  WhitelistEntry,
  PlayerList,
  Mod,
  ModList,
  ServerProperties,
  PlayerCommandAction,
  PlayerCommandArgs,
  FileEntry,
  FileList,
  FileContent,
  UnzipResult,
  User,
  PasskeyMeta,
  DNSRecord,
  DNSConfig,
  DNSStatusResponse,
  ServerDNSResponse,
  DNSTestResult,
  PublishDNSInput,
} from './types';

export class ApiError extends Error {
  status: number;
  detail: string;

  constructor(status: number, message: string, detail?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.detail = detail ?? message;
  }
}

const safeMethods = new Set(['GET', 'HEAD', 'OPTIONS']);

let csrfToken: string | null = null;
let csrfPromise: Promise<string> | null = null;

async function ensureCsrf(): Promise<string> {
  if (csrfToken) return csrfToken;
  if (!csrfPromise) {
    csrfPromise = fetch('/api/auth/csrf', { credentials: 'include' })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error('csrf unavailable'))))
      .then((body) => {
        csrfToken = String(body?.csrf_token ?? '');
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

function handleMockRequest<T>(path: string, init: RequestInit = {}): T | null {
  if (!isMock()) return null;

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
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
    } as T;
  }
  if (path.startsWith('/api/servers/') && path.endsWith('/status')) {
    return { id: 'demo', state: 'running', ram_mb: 8192, host_port: 25565, container_id: 'mcm-srv-demo' } as T;
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
    const newMod: Mod = { name, file, enabled: true };
    mockMods = [...mockMods.filter((m) => m.name !== name && m.file !== file), newMod];
    return newMod as T;
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
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    } as T;
  }

  return null;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
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
    headers,
    ...init,
  });

  if (!res.ok) {
    let detail = `Request failed (${res.status})`;
    try {
      const body = await res.json();
      detail = body?.error?.message ?? body?.message ?? detail;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, detail, detail);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}


export const api = {
  onboardingStatus: () => request<{ onboarding_required: boolean }>('/api/onboarding/status'),

  onboarding: (email: string, password: string) =>
    request<Me>('/api/onboarding', { method: 'POST', body: JSON.stringify({ email, password }) }),

  login: (email: string, password: string) =>
    request<Me>('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),

  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  me: () => request<Me>('/api/auth/me'),

  listServers: () =>
    request<{ servers: Server[] }>('/api/servers').then((res) => res.servers ?? []),

  createServer: (input: CreateServerInput) =>
    request<Server>('/api/servers', { method: 'POST', body: JSON.stringify(input) }),

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

  availablePorts: () => request<number[]>('/api/ports/available'),

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

  downloadMod: (serverId: string, url: string, filename: string, deleteOldName?: string) =>
    request<Mod>(`/api/servers/${serverId}/mods/download`, {
      method: 'POST',
      body: JSON.stringify({ url, filename, delete_old_name: deleteOldName }),
    }),

  uploadMod: (
    serverId: string,
    file: File,
    onProgress?: (loaded: number, total: number) => void,
    deleteOldName?: string,
  ) =>
    uploadWithProgress<Mod>(
      `/api/servers/${serverId}/mods`,
      [
        ['file', file],
        ...(deleteOldName ? [['delete_old_name', deleteOldName] as [string, string]] : []),
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

  createBackup: (serverId: string, name?: string) =>
    request<BackupRecord>(`/api/servers/${serverId}/backup`, {
      method: 'POST',
      body: JSON.stringify({ name: name ?? '' }),
    }),

  restoreBackup: (serverId: string, backupId: string) =>
    request<{ ok: boolean }>(`/api/servers/${serverId}/restore/${backupId}`, { method: 'POST' }),

  deleteBackup: (backupId: string) =>
    request<{ ok: boolean }>(`/api/backups/${backupId}`, { method: 'DELETE' }),

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

  archiveFile: (serverId: string, source: string, name?: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/archive`, {
      method: 'POST',
      body: JSON.stringify({ source, name: name ?? '' }),
    }),

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

  writeFileContent: (serverId: string, path: string, content: string) =>
    request<FileEntry>(`/api/servers/${serverId}/files/content`, {
      method: 'PUT',
      body: JSON.stringify({ path, content }),
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
};

export interface CreateServerInput {
  name: string;
  server_type: ServerType;
  version: string;
  build: string;
  ram_mb: number;
  extra_ports?: ExtraPort[];
}

export interface UpdateServerInput {
  name: string;
  ram_mb: number;
  host_port?: number;
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
