import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  findInstalledSpigetResource,
  getSpigetDownloadUrl,
  getSpigetIconUrl,
  getSpigetSafeFilename,
  getSpigetVersions,
  isSpigetResourceInstalled,
  searchSpiget,
} from '../src/api/spiget';
import type { Mod, SpigetResource } from '../src/api/types';

describe('Spiget (SpigotMC) API client', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('searchSpiget queries search endpoint when query is provided', async () => {
    const mockResources: SpigetResource[] = [
      {
        id: 28140,
        name: 'LuckPerms',
        tag: 'An advanced permissions plugin',
        version: { id: 648014 },
        author: { id: 100356 },
        category: { id: 21 },
        rating: { count: 1000, average: 4.8 },
        downloads: 8000000,
        icon: { url: 'data/resource_icons/28/28140.jpg', data: '' },
        releaseDate: 1471719960,
        updateDate: 1786045114,
        file: { type: '.jar', size: 1500, sizeUnit: 'KB', url: 'resources/28140/download' },
      },
    ];

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockResources,
    });
    global.fetch = fetchMock;

    const res = await searchSpiget({ query: 'luckperms', sort: '-downloads' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toContain('https://api.spiget.org/v2/search/resources/luckperms');
    expect(calledUrl).toContain('sort=-downloads');
    expect(res).toHaveLength(1);
    expect(res[0].name).toBe('LuckPerms');
  });

  it('searchSpiget queries default resources list when query is empty', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [],
    });
    global.fetch = fetchMock;

    await searchSpiget({ query: '', sort: '-rating', limit: 10, page: 2 });
    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toContain('https://api.spiget.org/v2/resources?');
    expect(calledUrl).toContain('size=10');
    expect(calledUrl).toContain('page=2');
  });

  it('getSpigetIconUrl handles data URI, http URL, and relative paths', () => {
    const baseRes: SpigetResource = {
      id: 1,
      name: 'Test',
      tag: '',
      version: { id: 1 },
      author: { id: 1 },
      category: { id: 1 },
      rating: { count: 0, average: 0 },
      downloads: 0,
      icon: { url: '', data: '' },
      releaseDate: 0,
      updateDate: 0,
    };

    expect(getSpigetIconUrl({ ...baseRes, icon: { url: '', data: 'BASE64DATA' } })).toBe(
      'data:image/png;base64,BASE64DATA',
    );
    expect(getSpigetIconUrl({ ...baseRes, icon: { url: 'https://example.com/icon.png' } })).toBe(
      'https://example.com/icon.png',
    );
    expect(getSpigetIconUrl({ ...baseRes, icon: { url: 'data/icons/1.jpg' } })).toBe(
      'https://static.spigotmc.org/data/icons/1.jpg',
    );
    expect(getSpigetIconUrl(baseRes)).toBeNull();
  });

  it('getSpigetDownloadUrl generates direct CDN link', () => {
    expect(getSpigetDownloadUrl(28140)).toBe('https://cdn.spiget.org/file/spiget-resources/28140.jar');
  });

  it('getSpigetSafeFilename produces filesystem safe jar names', () => {
    expect(getSpigetSafeFilename('LuckPerms')).toBe('LuckPerms.jar');
    expect(getSpigetSafeFilename('WorldEdit / FastAsync', '7.2.15')).toBe('WorldEdit___FastAsync-7.2.15.jar');
    expect(getSpigetSafeFilename('Plugin.jar')).toBe('Plugin.jar');
  });

  it('isSpigetResourceInstalled detects installed plugins', () => {
    const installedMods: Mod[] = [
      { name: 'LuckPerms', file: 'LuckPerms.jar', enabled: true },
    ];

    expect(isSpigetResourceInstalled({ id: 28140, name: 'LuckPerms' }, installedMods)).toBe(true);
    expect(isSpigetResourceInstalled({ id: 12345, name: 'Vault' }, installedMods)).toBe(false);
  });
});
