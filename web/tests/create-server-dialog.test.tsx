import { describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { CreateServerDialog } from '../src/components/create-server-dialog';
import { api, ApiError } from '../src/api/client';
import type { Server } from '../src/api/types';

const mockCreatedServer: Server = {
  id: 'srv-new-1',
  name: 'My Async Server',
  server_type: 'paper',
  version: '1.21.1',
  build: '123',
  ram_mb: 2048,
  cpu_limit: 1,
  memory_limit_mb: 2048,
  host_port: 25565,
  extra_ports: [],
  container_id: null,
  state: 'installing',
  backup_enabled: false,
  backup_interval_minutes: 0,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

describe('CreateServerDialog', () => {
  it('immediately closes the dialog on submit and invokes onCreated in background', async () => {
    vi.spyOn(api, 'javaVersions').mockResolvedValue([
      { version: 21, is_lts: true, name: 'Java 21' },
    ]);
    vi.spyOn(api, 'availablePorts').mockResolvedValue({
      available: [25565, 25566],
      used: [],
    });
    vi.spyOn(api, 'listServers').mockResolvedValue([]);
    vi.spyOn(api, 'jarVersions').mockResolvedValue([{ name: '1.21.1' }]);
    vi.spyOn(api, 'jarBuilds').mockResolvedValue([
      { version: '1.21.1', build: '123', display: 'Build 123' },
    ]);

    let resolveCreateServer: ((s: Server) => void) | null = null;
    vi.spyOn(api, 'createServer').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveCreateServer = resolve;
        })
    );

    const onCreatedMock = vi.fn();
    const onErrorMock = vi.fn();

    render(<CreateServerDialog onCreated={onCreatedMock} onError={onErrorMock} />);

    // Open dialog
    const openBtn = screen.getByRole('button', { name: /create server/i });
    await act(async () => {
      fireEvent.click(openBtn);
    });

    // Check dialog content is open
    expect(screen.getByRole('heading', { name: 'Create server' })).toBeInTheDocument();

    // Fill in server name
    const nameInput = screen.getByLabelText(/server name/i);
    await act(async () => {
      fireEvent.change(nameInput, { target: { value: 'My Async Server' } });
    });

    // Find submit button and submit
    const submitBtn = screen.getByRole('button', { name: /^create$/i });
    await act(async () => {
      fireEvent.click(submitBtn);
    });

    // The dialog should close IMMEDIATELY upon submit!
    expect(screen.queryByRole('heading', { name: 'Create server' })).toBeNull();
    // onCreated has not resolved yet since createServer is in flight
    expect(onCreatedMock).not.toHaveBeenCalled();

    // Now resolve the background creation
    await act(async () => {
      resolveCreateServer!(mockCreatedServer);
    });

    // onCreated callback is triggered
    expect(onCreatedMock).toHaveBeenCalledTimes(1);
    expect(onErrorMock).not.toHaveBeenCalled();
  });

  it('notifies onError callback if background creation fails', async () => {
    vi.spyOn(api, 'javaVersions').mockResolvedValue([]);
    vi.spyOn(api, 'availablePorts').mockResolvedValue({ available: [25565], used: [] });
    vi.spyOn(api, 'listServers').mockResolvedValue([]);
    vi.spyOn(api, 'jarVersions').mockResolvedValue([{ name: '1.21.1' }]);
    vi.spyOn(api, 'jarBuilds').mockResolvedValue([]);

    let rejectCreateServer: ((err: unknown) => void) | null = null;
    vi.spyOn(api, 'createServer').mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectCreateServer = reject;
        })
    );

    const onCreatedMock = vi.fn();
    const onErrorMock = vi.fn();

    render(<CreateServerDialog onCreated={onCreatedMock} onError={onErrorMock} />);

    // Open dialog
    const openBtn = screen.getByRole('button', { name: /create server/i });
    await act(async () => {
      fireEvent.click(openBtn);
    });

    // Fill in server name
    const nameInput = screen.getByLabelText(/server name/i);
    await act(async () => {
      fireEvent.change(nameInput, { target: { value: 'Failing Server' } });
    });

    const submitBtn = screen.getByRole('button', { name: /^create$/i });
    await act(async () => {
      fireEvent.click(submitBtn);
    });

    // Modal closed immediately
    expect(screen.queryByRole('heading', { name: 'Create server' })).toBeNull();

    // Reject creation in background
    await act(async () => {
      rejectCreateServer!(new ApiError(500, 'Docker daemon unavailable'));
    });

    expect(onErrorMock).toHaveBeenCalledWith('Docker daemon unavailable');
    expect(onCreatedMock).not.toHaveBeenCalled();
  });
});
