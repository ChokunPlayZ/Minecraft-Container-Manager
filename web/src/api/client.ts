import type {
  ConsoleLine,
  InstallInfo,
  Me,
  Server,
  ServerStatus,
  ServerType,
  Settings,
  BackupRecord,
  VersionInfo,
  VersionMeta,
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

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
    ...init,
  });

  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = await res.json();
      detail = body?.detail ?? body?.message ?? JSON.stringify(body);
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, `Request failed (${res.status})`, detail);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  onboarding: (email: string, password: string) =>
    request<Me>('/api/onboarding', { method: 'POST', body: JSON.stringify({ email, password }) }),

  login: (email: string, password: string) =>
    request<Me>('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),

  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),

  me: () => request<Me>('/api/auth/me'),

  listServers: () => request<Server[]>('/api/servers'),

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

  consoleTail: (id: string, opts?: { since?: string }) => {
    const q = opts?.since ? `?since=${encodeURIComponent(opts.since)}` : '';
    return request<ConsoleLine[]>(`/api/servers/${id}/console${q}`);
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
}

export interface UpdateServerInput {
  name: string;
  ram_mb: number;
  backup_enabled?: boolean;
  backup_interval_minutes?: number;
}

export interface InstallInput {
  version: string;
  build: string;
}
