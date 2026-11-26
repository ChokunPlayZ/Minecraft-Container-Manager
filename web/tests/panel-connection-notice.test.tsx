import { StrictMode } from 'react';
import { act, cleanup, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PanelConnectionNotice } from '../src/components/panel-connection-notice';

async function advance(ms: number) {
  await act(async () => { await vi.advanceTimersByTimeAsync(ms); });
}

function pendingProbe(_path: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const signal = init?.signal;
  return new Promise((_, reject) => {
    signal?.addEventListener('abort', () => reject(signal.reason), { once: true });
  });
}

beforeEach(() => vi.useFakeTimers());

afterEach(() => {
  cleanup();
  window.localStorage.removeItem('mcm-mock');
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('Panel connection notice', () => {
  it('checks periodically, shows a lost connection, and clears the alert after recovery', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(Response.json({ status: 'ok' }))
      .mockRejectedValueOnce(new TypeError('Connection lost'))
      .mockResolvedValueOnce(new Response(null, { status: 503 }))
      .mockResolvedValueOnce(Response.json({ status: 'ok' }));
    vi.stubGlobal('fetch', fetch);
    render(<StrictMode><PanelConnectionNotice /></StrictMode>);

    await advance(14_999);
    expect(fetch).not.toHaveBeenCalled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await advance(1);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith('/healthz', expect.objectContaining({ cache: 'no-store' }));
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await advance(15_000);
    expect(screen.getByRole('alert')).toHaveTextContent('MCM Not reachable');
    expect(screen.getByRole('alert')).toHaveTextContent('check internet connection or if your VPN is connected');

    await advance(15_000);
    expect(screen.getByRole('alert')).toBeVisible();

    await advance(15_000);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it.each([
    ['an HTML page', () => new Response('<html>Sign in to the network</html>')],
    ['unexpected JSON', () => Response.json({ status: 'maintenance' })],
  ])('shows the alert when a successful HTTP response contains %s', async (_description, response) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response()));
    render(<PanelConnectionNotice />);

    await advance(15_000);
    expect(screen.getByRole('alert')).toBeVisible();
  });

  it('times out a stalled check and cancels polling and any active check on unmount', async () => {
    const fetch = vi.fn(pendingProbe);
    vi.stubGlobal('fetch', fetch);
    const view = render(<PanelConnectionNotice />);

    await advance(19_999);
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(false);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    await advance(1);
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true);
    expect(screen.getByRole('alert')).toBeVisible();

    await advance(10_000);
    expect(fetch).toHaveBeenCalledTimes(2);
    view.unmount();
    expect(fetch.mock.calls[1][1]?.signal?.aborted).toBe(true);

    await advance(15_000);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('uses the demo transport without sending a network request', async () => {
    window.localStorage.setItem('mcm-mock', 'true');
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    render(<PanelConnectionNotice />);

    await advance(30_000);
    expect(fetch).not.toHaveBeenCalled();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });
});
