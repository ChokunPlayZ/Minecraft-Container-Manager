import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  getCurseForgeApiKey,
  getCurseForgeClassId,
  getCurseForgeDownloadUrl,
  getCurseForgeFiles,
  getCurseForgeLoaderType,
  hasCurseForgeApiKey,
  isCurseForgeModInstalled,
  searchCurseForge,
  setCurseForgeApiKey,
} from '../src/api/curseforge';
import type { CurseForgeMod, Mod } from '../src/api/types';

describe('CurseForge API client', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  afterEach(() => {
    global.fetch = originalFetch;
    localStorage.clear();
  });

  it('manages CurseForge API key in localStorage', () => {
    expect(hasCurseForgeApiKey()).toBe(false);
    expect(getCurseForgeApiKey()).toBe('');

    setCurseForgeApiKey('test-key-12345');
    expect(hasCurseForgeApiKey()).toBe(true);
    expect(getCurseForgeApiKey()).toBe('test-key-12345');

    setCurseForgeApiKey('');
    expect(hasCurseForgeApiKey()).toBe(false);
    expect(getCurseForgeApiKey()).toBe('');
  });

  it('maps server types to loaders and classes correctly', () => {
    expect(getCurseForgeLoaderType('forge')).toBe(1);
    expect(getCurseForgeLoaderType('fabric')).toBe(4);
    expect(getCurseForgeLoaderType('neoforge')).toBe(6);
    expect(getCurseForgeLoaderType('paper')).toBeUndefined();

    expect(getCurseForgeClassId('paper')).toBe(5);
    expect(getCurseForgeClassId('spigot')).toBe(5);
    expect(getCurseForgeClassId('fabric')).toBe(6);
    expect(getCurseForgeClassId('forge')).toBe(6);
  });

  it('searchCurseForge passes x-api-key header and query options', async () => {
    const mockCFResult = {
      data: [
        {
          id: 306612,
          gameId: 432,
          name: 'JEI - Just Enough Items',
          slug: 'jei',
          summary: 'Item and Recipe viewing mod',
          downloadCount: 150000000,
          categories: [],
          authors: [{ id: 1, name: 'mezz', url: '' }],
          latestFiles: [],
          dateModified: '2026-08-01T00:00:00Z',
        },
      ],
      pagination: { index: 0, pageSize: 20, totalCount: 1 },
    };

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockCFResult,
    });
    global.fetch = fetchMock;

    const res = await searchCurseForge(
      {
        query: 'jei',
        classId: 6,
        modLoaderType: 4,
        gameVersion: '1.20.4',
        sortField: 6,
        pageSize: 20,
        index: 0,
      },
      'my-cf-token',
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calledUrl = fetchMock.mock.calls[0][0] as string;
    const calledInit = fetchMock.mock.calls[0][1] as RequestInit;

    expect(calledUrl).toContain('https://api.curseforge.com/v1/mods/search');
    expect(calledUrl).toContain('searchFilter=jei');
    expect(calledUrl).toContain('modLoaderType=4');
    expect(calledUrl).toContain('gameVersion=1.20.4');

    expect((calledInit.headers as Record<string, string>)['x-api-key']).toBe('my-cf-token');
    expect(res.data[0].name).toBe('JEI - Just Enough Items');
  });

  it('getCurseForgeDownloadUrl resolves CDN download URL', async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: 'https://edge.forgecdn.net/files/123/456/jei.jar' }),
    });

    const url = await getCurseForgeDownloadUrl(306612, 123456, 'my-key');
    expect(url).toBe('https://edge.forgecdn.net/files/123/456/jei.jar');
  });

  it('isCurseForgeModInstalled detects installed mod matching slug', () => {
    const installedMods: Mod[] = [
      { name: 'jei-1.20.4', file: 'jei-1.20.4-fabric-15.0.0.jar', enabled: true },
    ];

    const jeiMod: Pick<CurseForgeMod, 'name' | 'slug'> = {
      name: 'Just Enough Items',
      slug: 'jei',
    };

    expect(isCurseForgeModInstalled(jeiMod, installedMods)).toBe(true);
  });
});
