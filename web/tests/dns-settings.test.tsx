import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { DNSSettingsCard } from '../src/components/dns-settings';

describe('DNSSettingsCard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it.each([
    { provider: 'porkbun', tokenKey: 'dns_porkbun_api_token', secretKey: 'dns_porkbun_api_secret', zoneKey: 'dns_porkbun_zone', name: 'Porkbun', tokenLabel: 'Porkbun API Key' },
    { provider: 'digitalocean', tokenKey: 'dns_digitalocean_api_token', secretKey: '', zoneKey: 'dns_digitalocean_zone', name: 'DigitalOcean', tokenLabel: 'DigitalOcean API Token' },
    { provider: 'route53', tokenKey: 'dns_route53_api_token', secretKey: 'dns_route53_api_secret', zoneKey: 'dns_route53_zone', name: 'Amazon Route 53', tokenLabel: 'AWS Access Key ID' },
  ])('loads and tests $name credentials while publishing is disabled', async ({ provider, tokenKey, secretKey, zoneKey, name, tokenLabel }) => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getSettings').mockResolvedValue({ settings: {
      dns_provider: provider, dns_publish: 'false', dns_domain: 'example.com',
      [tokenKey]: 'provider-key', ...(secretKey ? { [secretKey]: 'provider-secret' } : {}),
      [zoneKey]: provider === 'route53' ? 'Z123' : '',
    } });
    vi.spyOn(api, 'getDNS').mockResolvedValue({ records: [], configured: false });
    const test = vi.spyOn(api, 'testDNS').mockResolvedValue({ ok: true, message: 'Verified' });
    render(<DNSSettingsCard />);
    await waitFor(() => expect(screen.getByLabelText(tokenLabel)).toHaveValue('provider-key'));
    const button = screen.getByRole('button', { name: `Test ${name} Connection` });
    expect(button).toBeEnabled();
    await user.click(button);
    expect(test).toHaveBeenCalledWith(expect.objectContaining({
      provider, api_token: 'provider-key', api_secret: secretKey ? 'provider-secret' : undefined,
      zone: provider === 'route53' ? 'Z123' : 'example.com', domain: 'example.com',
    }));
  });

  it('retains separate credentials and unfinished edits when switching providers', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getSettings').mockResolvedValue({ settings: {
      dns_domain: 'example.com', dns_zone: 'cf-zone', dns_api_token: 'cf-token',
    } });
    vi.spyOn(api, 'getDNS').mockResolvedValue({ records: [], configured: false });
    const save = vi.spyOn(api, 'putSettings').mockResolvedValue({ settings: {} });
    render(<DNSSettingsCard />);
    await waitFor(() => expect(screen.getByLabelText('Cloudflare API Token')).toHaveValue('cf-token'));
    async function selectProvider(name: string) {
      await user.click(screen.getByRole('combobox', { name: 'DNS Provider' }));
      await user.click(screen.getByRole('option', { name }));
    }
    await selectProvider('Porkbun');
    expect(screen.getByLabelText('Porkbun API Key')).toHaveValue('');
    await user.type(screen.getByLabelText('Porkbun API Key'), 'pk-key');
    await user.type(screen.getByLabelText('Porkbun Secret API Key'), 'sk-key');
    await selectProvider('DigitalOcean');
    expect(screen.getByLabelText('DigitalOcean API Token')).toHaveValue('');
    await selectProvider('Porkbun');
    expect(screen.getByLabelText('Porkbun API Key')).toHaveValue('pk-key');
    expect(screen.getByLabelText('Porkbun Secret API Key')).toHaveValue('sk-key');
    await user.click(screen.getByRole('button', { name: 'Save DNS Settings' }));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({
      dns_provider: 'porkbun', dns_api_token: 'cf-token', dns_zone: 'cf-zone',
      dns_porkbun_api_token: 'pk-key', dns_porkbun_api_secret: 'sk-key',
    }));
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

    render(<DNSSettingsCard />);

    await waitFor(() => {
      expect(screen.getByText('DNS SRV Records')).toBeInTheDocument();
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

    render(<DNSSettingsCard />);

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Test Cloudflare Connection/i })).toBeInTheDocument();
    });

    const testBtn = screen.getByRole('button', { name: /Test Cloudflare Connection/i });
    await user.click(testBtn);

    await waitFor(() => {
      expect(testSpy).toHaveBeenCalledWith(expect.objectContaining({
        api_token: 'token-abc',
        zone: 'zone-123',
        domain: 'example.com',
        provider: 'cloudflare',
      }));
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

    render(<DNSSettingsCard />);

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
      expect(screen.getByText('DNS settings saved.')).toBeInTheDocument();
    });
  });
});
