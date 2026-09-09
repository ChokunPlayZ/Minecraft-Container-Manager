import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { CloudflareDNSSettingsCard } from '../src/components/cloudflare-dns-settings';

describe('CloudflareDNSSettingsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders DNS settings form and loads current configuration', async () => {
    vi.spyOn(api, 'getSettings').mockResolvedValue({
      settings: {
        dns_publish: 'true',
        dns_domain: 'example.com',
        dns_zone: 'zone-123',
        dns_api_token: 'token-abc',
        dns_host: 'node1.example.com',
        dns_service: '_minecraft',
        dns_proto: '_tcp',
        dns_ttl: '120',
        dns_priority: '0',
        dns_weight: '5',
      },
    });

    vi.spyOn(api, 'getDNS').mockResolvedValue({
      records: [
        {
          server_id: 'srv-1',
          record_id: 'rec-1',
          name: '_minecraft._tcp.smp.example.com',
          subdomain: 'smp',
          target: 'node1.example.com',
          port: 25565,
          priority: 0,
          weight: 5,
          ttl: 120,
          zone: 'zone-123',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
      configured: true,
    });

    render(<CloudflareDNSSettingsCard />);

    await waitFor(() => {
      expect(screen.getByText('Cloudflare SRV Records')).toBeInTheDocument();
    });

    // Check fields populated
    expect(screen.getByLabelText(/Base Domain/i)).toHaveValue('example.com');
    expect(screen.getByLabelText(/Cloudflare Zone ID/i)).toHaveValue('zone-123');
    expect(screen.getByLabelText(/Default Target Host/i)).toHaveValue('node1.example.com');

    // Check active record displayed in table
    expect(screen.getByText('smp.example.com')).toBeInTheDocument();
    expect(screen.getByText('_minecraft._tcp.smp.example.com')).toBeInTheDocument();
    expect(screen.getByText('node1.example.com:25565')).toBeInTheDocument();
  });

  it('tests Cloudflare connection when clicking Test button', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getSettings').mockResolvedValue({
      settings: {
        dns_publish: 'true',
        dns_domain: 'example.com',
        dns_zone: 'zone-123',
        dns_api_token: 'token-abc',
      },
    });

    vi.spyOn(api, 'getDNS').mockResolvedValue({
      records: [],
      configured: true,
    });

    const testSpy = vi.spyOn(api, 'testDNS').mockResolvedValue({
      ok: true,
      zone_name: 'example.com',
      status: 'active',
      message: 'Successfully verified zone "example.com" (status: active)',
    });

    render(<CloudflareDNSSettingsCard />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Test Cloudflare Connection/i })).toBeInTheDocument();
    });

    const testBtn = screen.getByRole('button', { name: /Test Cloudflare Connection/i });
    await user.click(testBtn);

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith({
        api_token: 'token-abc',
        zone: 'zone-123',
        domain: 'example.com',
      });
      expect(screen.getByText('Connection Verified')).toBeInTheDocument();
      expect(screen.getByText(/Successfully verified zone/i)).toBeInTheDocument();
    });
  });

  it('saves updated settings', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getSettings').mockResolvedValue({
      settings: {
        dns_publish: 'false',
        dns_domain: '',
        dns_zone: '',
        dns_api_token: '',
      },
    });

    vi.spyOn(api, 'getDNS').mockResolvedValue({
      records: [],
      configured: false,
    });

    const putSpy = vi.spyOn(api, 'putSettings').mockResolvedValue({
      settings: {},
    });

    render(<CloudflareDNSSettingsCard />);

    await waitFor(() => {
      expect(screen.getByLabelText(/Base Domain/i)).toBeInTheDocument();
    });

    await user.type(screen.getByLabelText(/Base Domain/i), 'mcserver.net');
    await user.type(screen.getByLabelText(/Cloudflare Zone ID/i), 'zone-999');

    const saveBtn = screen.getByRole('button', { name: /Save DNS Settings/i });
    await user.click(saveBtn);

    await waitFor(() => {
      expect(putSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          dns_domain: 'mcserver.net',
          dns_zone: 'zone-999',
        }),
      );
      expect(screen.getByText('Cloudflare DNS settings saved.')).toBeInTheDocument();
    });
  });
});
