import { renderWithAdmin as render } from '../src/test/auth';
import { screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { WhitelistPanel } from '../src/components/whitelist-panel';

const mockServer: Server = {
  id: 'test-server-id',
  name: 'Test Server',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 4096,
  cpu_limit: 2,
  memory_limit_mb: 4096,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-123',
  state: 'running',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('WhitelistPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('shows a skin head for each whitelisted player', async () => {
    vi.spyOn(api, 'whitelist').mockResolvedValue({
      whitelist: [{ uuid: 'uuid-1', name: 'Steve' }],
    });

    render(<WhitelistPanel server={mockServer} />);

    const avatar = await screen.findByRole('img', { name: 'Steve' });
    expect(avatar).toHaveAttribute('src', 'https://mc-heads.net/avatar/Steve/28');
  });
});
