import type { UserAccess, PermissionCatalog } from './types';
import { getArtifactHashes } from './artifact-hashes';
import { handleMockRequest, isMock } from './demo-transport';
import { ensureCsrf, safeMethods } from './csrf';
import { announceUpdateDiscovery } from '../lib/update-discovery';
import type {
  PanelUpdateStatus,
  UpdateCheckerStatus,
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

export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const mockResult = handleMockRequest<T>(path, init);
  if (mockResult !== null) return mockResult;

  const method = (init.method ?? 'GET').toUpperCase();
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

  jarVersions: (type: ServerType) => request<VersionMeta[]>(`/api/jars/${type}/versions`).then(announceUpdateDiscovery),

  jarBuilds: (type: ServerType, version: string) =>
    request<VersionInfo[]>(`/api/jars/${type}/versions/${encodeURIComponent(version)}/builds`).then(announceUpdateDiscovery),

  javaVersions: () => request<JavaRelease[]>('/api/system/java-versions'),

  availablePorts: (serverId?: string) => request<AvailablePortsResponse>(serverId ? `/api/ports/available?server_id=${encodeURIComponent(serverId)}` : '/api/ports/available'),

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

  getUpdateChecker: () => request<UpdateCheckerStatus>('/api/updates'),
  getPanelUpdate: () => request<PanelUpdateStatus>('/api/panel/update'),
  checkPanelUpdate: () => request<PanelUpdateStatus>('/api/panel/update/check', { method: 'POST' }),
  startPanelUpdate: (commit: string) => request<PanelUpdateStatus>('/api/panel/update', { method: 'POST', body: JSON.stringify({ commit }) }),
  saveUpdateChecker: (enabled: boolean) => request<UpdateCheckerStatus>('/api/updates/settings', { method: 'PUT', body: JSON.stringify({ enabled }) }),
  runUpdateCheck: () => request<{ accepted: boolean }>('/api/updates/check', { method: 'POST' }),

  putSettings: (settings: Record<string, string>) =>
    request<{ settings: Record<string, string> }>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify({ settings }),
    }),

  getDNS: () => request<DNSStatusResponse>('/api/dns'),

  testDNS: (creds?: { provider?: string; api_token?: string; api_secret?: string; session_token?: string; zone?: string; domain?: string }) =>
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

  permissionCatalog: () => request<PermissionCatalog>('/api/permissions'),

  createUser: (email: string, password: string, access?: UserAccess) =>
    request<User>('/api/users', { method: 'POST', body: JSON.stringify({ email, password, access }) }),

  updateUser: (id: string, input: { email?: string; password?: string; access?: UserAccess }) =>
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
      `/api/servers/${serverId}/mods/updates${force ? '?force=true' : '?cached=true'}`,
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
    meta?: { deleteOld?: boolean; projectId?: string; projectSlug?: string; provider?: ModProvider; hashes?: Record<string,string> },
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
 hashes:meta?.hashes??getArtifactHashes(url),
        }),
      },
    ),

  downloadMod: (
    serverId: string,
    url: string,
    filename: string,
    deleteOldName?: string,
    meta?: { projectId?: string; projectSlug?: string; provider?: ModProvider; hashes?: Record<string,string> },
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
 hashes:meta?.hashes??getArtifactHashes(url),
      }),
    }),

  uploadMod: (
    serverId: string,
    file: File,
    onProgress?: (loaded: number, total: number) => void,
    deleteOldName?: string,
    meta?: { projectId?: string; projectSlug?: string; provider?: ModProvider; hashes?: Record<string,string> },
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
    if (isMock() || typeof EventSource === 'undefined') {
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
    if(isMock())return;
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
    if(isMock())return;
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
  if(isMock())return handleMockRequest<T>(path,{method:'POST',body}) as T;
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
