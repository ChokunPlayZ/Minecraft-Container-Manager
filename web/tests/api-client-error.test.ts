import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';

afterEach(() => vi.unstubAllGlobals());

describe('API connection feedback', () => {
  it('shows a readable error when the server returns HTML instead of JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>Unavailable</html>', { status: 200 })));
    await expect(api.listUsers()).rejects.toBeInstanceOf(ApiError);
    await expect(api.listUsers()).rejects.toMatchObject({ detail: 'Could not read the server response. Try again.' });
  });
});
