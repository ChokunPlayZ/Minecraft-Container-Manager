import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  findInstalledHangarProject,
  getHangarVersions,
  isHangarProjectInstalled,
  isHangarVersionInstalled,
  searchHangar,
} from '../src/api/hangar';
import type { HangarProject, HangarVersion, Mod } from '../src/api/types';

describe('Hangar API client', () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it('searchHangar sends correct query params and parses result', async () => {
    const mockHangarResponse = {
      pagination: { count: 1, limit: 20, offset: 0 },
      result: [
        {
          id: 101,
          name: 'Chunky',
          namespace: { owner: 'pop4959', slug: 'Chunky' },
          stats: { downloads: 150000, stars: 250, views: 500000, recentDownloads: 1200, recentViews: 5000, watchers: 10 },
          category: 'world_management',
          description: 'Pre-generates chunks quickly and efficiently.',
          lastUpdated: '2026-08-01T00:00:00Z',
          visibility: 'public',
          avatarUrl: 'https://hangarcdn.papermc.io/avatars/project/101.webp',
          supportedPlatforms: { PAPER: ['1.20', '1.21'] },
        },
      ],
    };

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockHangarResponse,
    });
    global.fetch = fetchMock;

    const res = await searchHangar({
      query: 'chunky',
      category: 'world_management',
      sort: '-downloads',
      platform: 'PAPER',
      limit: 20,
      offset: 0,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calledUrl = fetchMock.mock.calls[0][0] as string;
    expect(calledUrl).toContain('https://hangar.papermc.io/api/v1/projects');
    expect(calledUrl).toContain('q=chunky');
    expect(calledUrl).toContain('category=world_management');
    expect(calledUrl).toContain('sort=-downloads');
    expect(calledUrl).toContain('platform=PAPER');

    expect(res.result).toHaveLength(1);
    expect(res.result[0].name).toBe('Chunky');
    expect(res.pagination.count).toBe(1);
  });

  it('getHangarVersions retrieves releases', async () => {
    const mockVersionsResponse = {
      pagination: { count: 1, limit: 25, offset: 0 },
      result: [
        {
          id: 501,
          projectId: 101,
          name: '1.4.10',
          visibility: 'public',
          description: 'Chunky 1.4.10 bugfix release',
          stats: { totalDownloads: 12000, platformDownloads: { PAPER: 12000 } },
          author: 'pop4959',
          channel: { name: 'Release', color: '#00cc88' },
          downloads: {
            PAPER: {
              fileInfo: {
                name: 'Chunky-1.4.10.jar',
                sizeBytes: 850000,
                sha256Hash: 'abcd1234efgh5678',
              },
              downloadUrl: 'https://hangarcdn.papermc.io/plugins/pop4959/Chunky/versions/1.4.10/PAPER/Chunky-1.4.10.jar',
              externalUrl: null,
            },
          },
          platformDependencies: { PAPER: ['1.20', '1.21'] },
          createdAt: '2026-07-20T12:00:00Z',
        },
      ],
    };

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => mockVersionsResponse,
    });

    const vers = await getHangarVersions('Chunky');
    expect(vers).toHaveLength(1);
    expect(vers[0].name).toBe('1.4.10');
    expect(vers[0].downloads.PAPER?.fileInfo?.name).toBe('Chunky-1.4.10.jar');
  });

  it('findInstalledHangarProject and isHangarProjectInstalled match installed plugins', () => {
    const installedMods: Mod[] = [
      { name: 'Chunky', file: 'Chunky-1.4.10.jar', enabled: true },
      { name: 'LuckPerms', file: 'LuckPerms-Bukkit-5.4.102.jar', enabled: true },
    ];

    const chunkyProject: Pick<HangarProject, 'name' | 'namespace'> = {
      name: 'Chunky',
      namespace: { owner: 'pop4959', slug: 'chunky' },
    };

    const viaVersionProject: Pick<HangarProject, 'name' | 'namespace'> = {
      name: 'ViaVersion',
      namespace: { owner: 'viaversion', slug: 'viaversion' },
    };

    expect(isHangarProjectInstalled(chunkyProject, installedMods)).toBe(true);
    expect(findInstalledHangarProject(chunkyProject, installedMods)?.file).toBe('Chunky-1.4.10.jar');
    expect(isHangarProjectInstalled(viaVersionProject, installedMods)).toBe(false);
  });

  it('isHangarVersionInstalled detects file match on server', () => {
    const installedMods: Mod[] = [
      { name: 'Chunky', file: 'Chunky-1.4.10.jar', enabled: true },
    ];

    const verMatching: HangarVersion = {
      id: 1,
      projectId: 101,
      name: '1.4.10',
      visibility: 'public',
      stats: { totalDownloads: 100, platformDownloads: { PAPER: 100 } },
      author: 'pop4959',
      channel: { name: 'Release', color: '#00cc88' },
      downloads: {
        PAPER: {
          fileInfo: {
            name: 'Chunky-1.4.10.jar',
            sizeBytes: 12345,
            sha256Hash: 'hash',
          },
          downloadUrl: 'https://cdn.example.com/chunky.jar',
          externalUrl: null,
        },
      },
      platformDependencies: {},
      createdAt: '2026-01-01T00:00:00Z',
    };

    const verOther: HangarVersion = {
      ...verMatching,
      downloads: {
        PAPER: {
          fileInfo: {
            name: 'Chunky-1.5.0.jar',
            sizeBytes: 12345,
            sha256Hash: 'hash2',
          },
          downloadUrl: 'https://cdn.example.com/chunky-1.5.jar',
          externalUrl: null,
        },
      },
    };

    expect(isHangarVersionInstalled(verMatching, installedMods)).toBe(true);
    expect(isHangarVersionInstalled(verOther, installedMods)).toBe(false);
  });
});
