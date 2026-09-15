import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import { parsePortPoolPreview, PortPoolSettingsCard } from '../src/components/port-pool-settings';

describe('parsePortPoolPreview', () => {
  it('parses valid single port', () => {
    const res = parsePortPoolPreview('25565');
    expect(res.count).toBe(1);
    expect(res.error).toBeUndefined();
  });

  it('parses valid port range', () => {
    const res = parsePortPoolPreview('25565-25575');
    expect(res.count).toBe(11);
    expect(res.error).toBeUndefined();
  });

  it('parses mixed ports and ranges with duplicate removal', () => {
    const res = parsePortPoolPreview('25565, 25566, 25565-25570, 25580');
    // 25565..25570 (6 ports) + 25580 (1 port) = 7 ports
    expect(res.count).toBe(7);
    expect(res.error).toBeUndefined();
  });

  it('rejects empty input', () => {
    const res = parsePortPoolPreview('   ');
    expect(res.error).toBe('Port pool cannot be empty');
  });

  it('rejects invalid range format', () => {
    const res = parsePortPoolPreview('25565-25566-25567');
    expect(res.error).toContain('Invalid range syntax');
  });

  it('rejects out-of-range port numbers', () => {
    const res = parsePortPoolPreview('70000');
    expect(res.error).toContain('Invalid port number');
  });

  it('rejects inverted range (start > end)', () => {
    const res = parsePortPoolPreview('25570-25565');
    expect(res.error).toContain('start <= end');
  });
});

describe('PortPoolSettingsCard component', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('loads current settings and displays available stats', async () => {
    vi.spyOn(api, 'getSettings').mockResolvedValue({
      settings: { port_pool: '25565-25570' },
    });
    vi.spyOn(api, 'availablePorts').mockResolvedValue({
      available: [25565, 25566, 25567],
      pool: '25565-25570',
    });

    render(<PortPoolSettingsCard />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('25565-25570')).toBeInTheDocument();
    });

    // Total in pool: 6
    expect(screen.getByText('6')).toBeInTheDocument();
    // Available / In-Use: 3
    expect(screen.getAllByText('3').length).toBeGreaterThanOrEqual(2);
  });

  it('updates port pool configuration and saves via api', async () => {
    const user = userEvent.setup();

    vi.spyOn(api, 'getSettings').mockResolvedValue({
      settings: { port_pool: '25565-25570' },
    });
    vi.spyOn(api, 'availablePorts').mockResolvedValue({
      available: [25565, 25566],
      pool: '25565-25570',
    });
    const putSpy = vi.spyOn(api, 'putSettings').mockResolvedValue({
      settings: { port_pool: '25565-25575' },
    });

    render(<PortPoolSettingsCard />);

    await waitFor(() => {
      expect(screen.getByDisplayValue('25565-25570')).toBeInTheDocument();
    });

    const input = screen.getByDisplayValue('25565-25570');
    await user.clear(input);
    await user.type(input, '25565-25575');

    const saveBtn = screen.getByRole('button', { name: /Save Changes/i });
    expect(saveBtn).not.toBeDisabled();
    await user.click(saveBtn);

    expect(putSpy).toHaveBeenCalledWith({ port_pool: '25565-25575' });
    await waitFor(() => {
      expect(screen.getByText(/Port pool configuration saved successfully/i)).toBeInTheDocument();
    });
  });
});
