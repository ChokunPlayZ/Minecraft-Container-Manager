export type ServerType = 'paper' | 'fabric';
export type ServerState = 'stopped' | 'starting' | 'running' | 'stopping' | 'error';

export interface Server {
  id: string;
  name: string;
  server_type: ServerType;
  version: string;
  build: string;
  ram_mb: number;
  host_port: number;
  container_id: string | null;
  state: ServerState;
  backup_enabled: boolean;
  backup_interval_minutes: number;
  created_at: string;
  updated_at: string;
}

export interface Me {
  id: string;
  email: string;
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
