import { render, screen, waitFor, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { BackupsPanel } from '../src/components/backups-panel';
import type { BackupProgress, Server } from '../src/api/types';

const mockServer: Server = {
  id: 'test-server',
  name: 'Survival Server',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25565,
  extra_ports: [],
  container_id: 'mcm-srv-test-server',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 1440,
  created_at: '2026-09-15T00:00:00Z',
  updated_at: '2026-09-15T00:00:00Z',
};

describe('BackupsPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders backup list, create input, storage selector, and upload button', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({
      backups: [
        {
          id: 'b-1',
          server_id: 'test-server',
          name: 'initial-backup',
          location: 'local:backups/initial-backup.tar.gz',
          size_bytes: 10485760, // 10MB
          status: 'completed',
          created_at: '2026-09-15T10:00:00Z',
        },
      ],
    });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(() => {});

    render(<BackupsPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('initial-backup')).toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /Upload/i })).toBeInTheDocument();
    expect(screen.getByPlaceholderText('pre-update')).toBeInTheDocument();
    expect(screen.getByText(/10\.0 MB/)).toBeInTheDocument();
    expect(screen.getByText('Local')).toBeInTheDocument();
  });

  it('displays active progress bar when backup progress is active on mount', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [] });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({
      active: true,
      progress: {
        id: 'prog-1',
        server_id: 'test-server',
        operation: 'backup',
        stage: 'compressing',
        percent: 62,
        bytes_done: 65000000,
        bytes_total: 104857600,
        message: 'Compressing world data...',
      },
    });
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(() => {});

    render(<BackupsPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/Compressing world data/i)).toBeInTheDocument();
      expect(screen.getByText('62%')).toBeInTheDocument();
      expect(screen.getByRole('progressbar')).toBeInTheDocument();
    });
  });

  it('updates progress via subscribeBackupEvents callback and shows completion banner', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [] });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });

    let eventCallback: ((p: BackupProgress) => void) | undefined;
    vi.spyOn(api, 'subscribeBackupEvents').mockImplementation((_srvId, cb) => {
      eventCallback = cb;
      return () => {};
    });

    render(<BackupsPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('No backups yet.')).toBeInTheDocument();
    });

    // Simulate progress event received
    act(() => {
      eventCallback?.({
        id: 'prog-sse',
        server_id: 'test-server',
        operation: 'restore',
        stage: 'extracting',
        percent: 85,
        message: 'Extracting archive into world folder...',
      });
    });

    await waitFor(() => {
      expect(screen.getByText(/Extracting archive into world folder/i)).toBeInTheDocument();
      expect(screen.getByText('85%')).toBeInTheDocument();
    });

    // Simulate completion event
    act(() => {
      eventCallback?.({
        id: 'prog-sse',
        server_id: 'test-server',
        operation: 'restore',
        stage: 'completed',
        percent: 100,
        message: 'World restored successfully!',
      });
    });

    await waitFor(() => {
      expect(screen.getByText('World restored successfully!')).toBeInTheDocument();
    });
  });

  it('handles external backup file upload and displays upload progress', async () => {
    vi.spyOn(api, 'listBackups').mockResolvedValue({ backups: [] });
    vi.spyOn(api, 'getBackupProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeBackupEvents').mockReturnValue(() => {});

    const uploadSpy = vi.spyOn(api, 'uploadBackup').mockImplementation(
      async (_id, _file, _name, _storage, onProgress) => {
        onProgress?.(50, 100);
        return {
          id: 'b-new',
          server_id: 'test-server',
          name: 'uploaded-world',
          location: 'local:backups/uploaded-world.tar.gz',
          size_bytes: 100,
          status: 'completed',
          created_at: new Date().toISOString(),
        };
      },
    );

    render(<BackupsPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('No backups yet.')).toBeInTheDocument();
    });

    const file = new File(['dummy backup content'], 'uploaded-world.tar.gz', {
      type: 'application/gzip',
    });

    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    expect(fileInput).toBeInTheDocument();

    await act(async () => {
      await userEvent.upload(fileInput, file);
    });

    expect(uploadSpy).toHaveBeenCalledTimes(1);
    expect(uploadSpy).toHaveBeenCalledWith(
      'test-server',
      expect.any(File),
      undefined,
      'local',
      expect.any(Function),
    );
  });
});
