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
  PlayerList,
  Mod,
  ModList,
  ServerProperties,
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

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
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
  restartServer: (id: string) => request<Server>(`/api/servers/${id}/restart`, { method: 'POST' }),

  serverStatus: (id: string) => request<ServerStatus>(`/api/servers/${id}/status`),

  openConsoleStream: (id: string, onLine: (line: ConsoleLine) => void): (() => void) => {
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

  installInfo: (id: string) => request<InstallInfo>(`/api/servers/${id}/install`),

  installServer: (id: string, input: InstallInput) =>
    request<InstallInfo>(`/api/servers/${id}/install`, { method: 'POST', body: JSON.stringify(input) }),

  jarVersions: (type: ServerType) => request<VersionMeta[]>(`/api/jars/${type}/versions`),

  jarBuilds: (type: ServerType, version: string) =>
    request<VersionInfo[]>(`/api/jars/${type}/versions/${encodeURIComponent(version)}/builds`),

  availablePorts: () => request<number[]>('/api/ports/available'),

  getSettings: () => request<Settings>('/api/settings'),

  putSettings: (settings: Settings) =>
    request<Settings>('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }),

  listBackups: (serverId: string) =>
    request<{ backups: BackupRecord[] }>(`/api/servers/${serverId}/backups`),

  players: (serverId: string) => request<PlayerList>(`/api/servers/${serverId}/players`),

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

  mods: (serverId: string) => request<ModList>(`/api/servers/${serverId}/mods`),

  uploadMod: (serverId: string, file: File) => {
    const body = new FormData();
    body.append('file', file);
    return request<Mod>(`/api/servers/${serverId}/mods`, { method: 'POST', body });
  },

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
  backup_enabled?: boolean;
  backup_interval_minutes?: number;
  extra_ports?: ExtraPort[];
}

export interface InstallInput {
  version: string;
  build: string;
}
