import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, FileConflictError } from '../src/api/client';

afterEach(() => vi.unstubAllGlobals());

describe('API connection feedback', () => {
  it('shows a readable error when the server returns HTML instead of JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>Unavailable</html>', { status: 200 })));
    await expect(api.listUsers()).rejects.toBeInstanceOf(ApiError);
    await expect(api.listUsers()).rejects.toMatchObject({ detail: 'Could not read the server response. Try again.' });
  });
});

describe('file conflict responses', () => {
  const current = { path: 'config.yml', content: 'external edits', revision: 'latest', exists: true };
  const conflictResponse = () => Response.json({ error: { code: 'file_changed', message: 'Compare before saving' }, current }, { status: 409 });

  it('sends the opened revision and exposes the current file for comparison', async () => {
    const fetch = vi.fn().mockImplementation(async (path: string) => path === '/api/auth/csrf'
      ? Response.json({ csrf_token: 'token' })
      : conflictResponse());
    vi.stubGlobal('fetch', fetch);
    await expect(api.writeFileContent('server', 'config.yml', 'my edits', 'opened')).rejects.toBeInstanceOf(FileConflictError);
    await expect(api.writeFileContent('server', 'config.yml', 'my edits', 'opened')).rejects.toMatchObject({ current, status: 409 });
    expect(fetch).toHaveBeenLastCalledWith('/api/servers/server/files/content', expect.objectContaining({
      body: JSON.stringify({ path: 'config.yml', content: 'my edits', expected_revision: 'opened' }),
    }));
  });

  it('retains conflict details after refreshing an expired CSRF token', async () => {
    let puts = 0;
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async (path: string) => {
      if (path === '/api/auth/csrf') return Response.json({ csrf_token: 'fresh-token' });
      if (++puts === 1) return Response.json({ error: { code: 'csrf_missing', message: 'Expired token' } }, { status: 403 });
      return conflictResponse();
    }));
    await expect(api.writeFileContent('server', 'config.yml', 'my edits', 'opened')).rejects.toMatchObject({ current, status: 409 });
    expect(puts).toBe(2);
  });
});
