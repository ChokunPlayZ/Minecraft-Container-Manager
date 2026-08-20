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
  PlayerCommandAction,
  PlayerCommandArgs,
  FileEntry,
  FileList,
  UnzipResult,
  User,
  PasskeyMeta,
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

  getSettings: () => request<Settings>('/api/settings'),

  putSettings: (settings: Settings) =>
    request<Settings>('/api/settings', { method: 'PUT', body: JSON.stringify(settings) }),

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

  uploadFile: (serverId: string, file: File, dir: string) => {
    const body = new FormData();
    body.append('file', file);
    const q = dir ? `?dir=${encodeURIComponent(dir)}` : '';
    return request<FileEntry>(`/api/servers/${serverId}/files/upload${q}`, {
      method: 'POST',
      body,
    });
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
