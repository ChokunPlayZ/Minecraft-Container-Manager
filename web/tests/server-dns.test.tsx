import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { ServerDNSCard } from '../src/components/server-dns-card';

const mockServer: Server = {
  id: 'srv-test',
  name: 'Vanilla Survival',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 8192,
  cpu_limit: 4,
  memory_limit_mb: 8192,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-test',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('ServerDNSCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders server DNS card and shows published record', async () => {
    vi.spyOn(api, 'getServerDNS').mockResolvedValue({
      record: {
        server_id: 'srv-test',
        record_id: 'rec-test-1',
        name: '_minecraft._tcp.survival.example.com',
        subdomain: 'survival',
        target: 'mc-node1.example.com',
        port: 25565,
        priority: 0,
        weight: 5,
        ttl: 120,
        zone: 'zone-123',
        updated_at: '2026-01-01T00:00:00Z',
      },
      configured: true,
      domain: 'example.com',
      server_id: 'srv-test',
      server_name: 'Vanilla Survival',
      host_port: 25565,
      join_address: 'survival.example.com',
    });

    render(<ServerDNSCard server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Cloudflare SRV Routing')).toBeInTheDocument();
    });

    // Check active badge
    expect(screen.getByText('Active SRV')).toBeInTheDocument();

    // Check join address display
    expect(screen.getAllByText('survival.example.com').length).toBeGreaterThanOrEqual(1);

    // Check subdomain input
    expect(screen.getByLabelText(/Subdomain Label/i)).toHaveValue('survival');
  });

  it('publishes a new SRV record with custom subdomain', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getServerDNS').mockResolvedValue({
      record: null,
      configured: true,
      domain: 'example.com',
      server_id: 'srv-test',
      server_name: 'Vanilla Survival',
      host_port: 25565,
      join_address: '',
    });

    const publishSpy = vi.spyOn(api, 'publishServerDNS').mockResolvedValue({
      ok: true,
      join_address: 'play.example.com',
      record: {
        server_id: 'srv-test',
        record_id: 'rec-new',
        name: '_minecraft._tcp.play.example.com',
        subdomain: 'play',
        target: 'example.com',
        port: 25565,
        priority: 0,
        weight: 5,
        ttl: 120,
        updated_at: '2026-01-01T00:00:00Z',
      },
    });

    render(<ServerDNSCard server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Publish SRV Record/i })).toBeInTheDocument();
    });

    const input = screen.getByLabelText(/Subdomain Label/i);
    await user.clear(input);
    await user.type(input, 'play');

    const publishBtn = screen.getByRole('button', { name: /Publish SRV Record/i });
    await user.click(publishBtn);

    await waitFor(() => {
      expect(publishSpy).toHaveBeenCalledWith(
        'srv-test',
        expect.objectContaining({
          subdomain: 'play',
          port: 25565,
        }),
      );
      expect(screen.getByText('SRV record successfully published to Cloudflare!')).toBeInTheDocument();
    });
  });
});
