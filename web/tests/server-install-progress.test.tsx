import { describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { ServerInstallProgress } from '../src/components/server-install-progress';
import { api } from '../src/api/client';
import type { TaskProgress } from '../src/api/types';

describe('ServerInstallProgress', () => {
  it('renders default installing state before any task progress arrives', async () => {
    vi.spyOn(api, 'getTaskProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation(() => () => {});

    render(<ServerInstallProgress serverId="srv-1" state="installing" compact={true} />);

    expect(screen.getByTestId('server-install-progress-compact')).toBeInTheDocument();
    expect(screen.getByText('Installing Server')).toBeInTheDocument();
    expect(screen.getByText('Setting up server container...')).toBeInTheDocument();
  });

  it('renders default building state when state is building', async () => {
    vi.spyOn(api, 'getTaskProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation(() => () => {});

    render(<ServerInstallProgress serverId="srv-1" state="building" compact={false} />);

    expect(screen.getByTestId('server-install-progress-hero')).toBeInTheDocument();
    expect(screen.getByText('Building Server from Source')).toBeInTheDocument();
    expect(screen.getByText('Compiling code via BuildTools...')).toBeInTheDocument();
  });

  it('updates with download progress, percent, and byte stats via SSE', async () => {
    let subscriberCallback: ((p: TaskProgress) => void) | null = null;
    vi.spyOn(api, 'getTaskProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((_, onProgress) => {
      subscriberCallback = onProgress;
      return () => {};
    });

    render(<ServerInstallProgress serverId="srv-1" state="installing" compact={true} />);

    expect(subscriberCallback).not.toBeNull();

    await act(async () => {
      subscriberCallback?.({
        server_id: 'srv-1',
        operation: 'server_install',
        stage: 'downloading_jar',
        stage_title: 'Downloading Server Jar (paper 1.21.1)',
        percent: 45,
        bytes_done: 45 * 1024 * 1024,
        bytes_total: 100 * 1024 * 1024,
        message: 'Downloading paper-1.21.1-123.jar (45.0 MB / 100.0 MB)...',
      });
    });

    expect(screen.getByText('Downloading Server Jar (paper 1.21.1)')).toBeInTheDocument();
    expect(screen.getByText('45%')).toBeInTheDocument();
    expect(screen.getByText('45.0 MB / 100.0 MB')).toBeInTheDocument();
  });

  it('renders hero banner layout with stage counters and details', async () => {
    let subscriberCallback: ((p: TaskProgress) => void) | null = null;
    vi.spyOn(api, 'getTaskProgress').mockResolvedValue({ active: false });
    vi.spyOn(api, 'subscribeTaskEvents').mockImplementation((_, onProgress) => {
      subscriberCallback = onProgress;
      return () => {};
    });

    render(<ServerInstallProgress serverId="srv-1" state="installing" compact={false} />);

    await act(async () => {
      subscriberCallback?.({
        server_id: 'srv-1',
        operation: 'modpack_install',
        stage: 'downloading_mods',
        stage_title: 'Downloading Modpack Mods',
        stage_index: 3,
        stage_total: 4,
        percent: 60,
        bytes_done: 60 * 1024 * 1024,
        bytes_total: 100 * 1024 * 1024,
        message: 'Downloading mod-30.jar',
      });
    });

    expect(screen.getByTestId('server-install-progress-hero')).toBeInTheDocument();
    expect(screen.getByText('Downloading Modpack Mods')).toBeInTheDocument();
    expect(screen.getByText('Stage 3 of 4')).toBeInTheDocument();
    expect(screen.getByText('60%')).toBeInTheDocument();
    expect(screen.getByText('60.0 MB / 100.0 MB')).toBeInTheDocument();
  });
});
