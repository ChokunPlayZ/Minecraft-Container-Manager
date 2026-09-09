import { describe, expect, it, vi, beforeEach } from 'vitest';
import {
  formatCount,
  formatFileSize,
  getLoaderLabel,
  getServerLoaders,
  isModInstalled,
  searchModrinth,
  getProjectVersions,
} from '../src/api/modrinth';
import type { Mod } from '../src/api/types';

describe('modrinth api & matching utilities', () => {
  it('maps server types to correct Modrinth loaders', () => {
    expect(getServerLoaders('paper')).toEqual(['paper', 'purpur', 'spigot', 'bukkit', 'folia']);
    expect(getServerLoaders('spigot')).toEqual(['spigot', 'bukkit', 'paper']);
    expect(getServerLoaders('fabric')).toEqual(['fabric', 'quilt']);
    expect(getServerLoaders('forge')).toEqual(['forge']);
    expect(getServerLoaders('neoforge')).toEqual(['neoforge', 'forge']);
    expect(getServerLoaders('vanilla')).toEqual([]);
  });

  it('provides friendly labels for loaders', () => {
    expect(getLoaderLabel('paper')).toBe('Paper / Spigot');
    expect(getLoaderLabel('fabric')).toBe('Fabric');
    expect(getLoaderLabel('forge')).toBe('Forge');
    expect(getLoaderLabel('neoforge')).toBe('NeoForge');
    expect(getLoaderLabel('vanilla')).toBe('Vanilla');
  });

  it('formats counts into readable numbers', () => {
    expect(formatCount(450)).toBe('450');
    expect(formatCount(1500)).toBe('1.5K');
    expect(formatCount(25000000)).toBe('25M');
    expect(formatCount(25400000)).toBe('25.4M');
  });

  it('formats file sizes accurately', () => {
    expect(formatFileSize(500)).toBe('500 B');
    expect(formatFileSize(1536)).toBe('1.5 KB');
    expect(formatFileSize(2097152)).toBe('2 MB');
  });

  it('accurately identifies installed mods by slug, title, or filename', () => {
    const installed: Mod[] = [
      { name: 'EssentialsX', file: 'EssentialsX-2.20.1.jar', enabled: true },
      { name: 'Fabric-API', file: 'fabric-api-0.119.4+1.21.4.jar', enabled: true },
      { name: 'LuckPerms', file: 'LuckPerms-5.4.102.jar', enabled: false },
    ];

    expect(isModInstalled({ slug: 'essentialsx', title: 'EssentialsX' }, installed)).toBe(true);
    expect(isModInstalled({ slug: 'fabric-api', title: 'Fabric API' }, installed)).toBe(true);
    expect(isModInstalled({ slug: 'luckperms', title: 'LuckPerms' }, installed)).toBe(true);
    expect(isModInstalled({ slug: 'worldedit', title: 'WorldEdit' }, installed)).toBe(false);
  });

  describe('searchModrinth API client', () => {
    beforeEach(() => {
      vi.restoreAllMocks();
    });

    it('builds search facets with loaders, game version, and server_side filter', async () => {
      let capturedUrl = '';
      vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
        capturedUrl = String(url);
        return {
          ok: true,
          json: async () => ({ hits: [], offset: 0, limit: 20, total_hits: 0 }),
        } as Response;
      });

      await searchModrinth({
        query: 'luckperms',
        loaders: ['paper', 'spigot'],
        gameVersion: '1.21.4',
        category: 'management',
        serverSideOnly: true,
        sort: 'downloads',
      });

      expect(capturedUrl).toContain('https://api.modrinth.com/v2/search');
      expect(capturedUrl).toContain('query=luckperms');
      expect(capturedUrl).toContain('index=downloads');

      // Decode facets from URL
      const urlObj = new URL(capturedUrl);
      const facetsParam = urlObj.searchParams.get('facets');
      expect(facetsParam).toBeTruthy();
      const parsedFacets = JSON.parse(facetsParam!);

      expect(parsedFacets).toContainEqual(['categories:paper', 'categories:spigot']);
      expect(parsedFacets).toContainEqual(['versions:1.21.4']);
      expect(parsedFacets).toContainEqual(['categories:management']);
      expect(parsedFacets).toContainEqual(['server_side!=unsupported']);
    });

    it('handles query for project versions', async () => {
      let capturedUrl = '';
      vi.spyOn(global, 'fetch').mockImplementation(async (url) => {
        capturedUrl = String(url);
        return {
          ok: true,
          json: async () => [
            {
              id: 'v1',
              name: 'Release 1.0',
              version_number: '1.0.0',
              files: [{ filename: 'mod.jar', url: 'https://cdn.modrinth.com/mod.jar', primary: true, size: 1024 }],
            },
          ],
        } as Response;
      });

      const versions = await getProjectVersions('essentialsx', {
        loaders: ['paper'],
        gameVersions: ['1.21.4'],
      });

      expect(capturedUrl).toContain('/project/essentialsx/version');
      expect(versions).toHaveLength(1);
      expect(versions[0].files[0].filename).toBe('mod.jar');
    });
  });
});
