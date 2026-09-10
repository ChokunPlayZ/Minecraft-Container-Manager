import { describe, expect, it, vi, beforeEach } from 'vitest';
import {
  findInstalledMod,
  formatCount,
  formatFileSize,
  getLoaderLabel,
  getServerLoaders,
  isModInstalled,
  isVersionFileInstalled,
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

    expect(findInstalledMod({ slug: 'essentialsx', title: 'EssentialsX' }, installed)?.file).toBe(
      'EssentialsX-2.20.1.jar',
    );
    expect(findInstalledMod({ slug: 'worldedit', title: 'WorldEdit' }, installed)).toBeUndefined();

    expect(
      isVersionFileInstalled({ filename: 'EssentialsX-2.20.1.jar' }, installed),
    ).toBe(true);
    expect(
      isVersionFileInstalled({ filename: 'EssentialsX-2.21.0.jar' }, installed),
    ).toBe(false);
  });

  it('distinguishes between similar mod names (e.g. AppleSkin vs Appleskin+- and Sodium vs Sodium Extra)', () => {
    const regularAppleSkin: Mod[] = [
      { name: 'appleskin-fabric-mc26.2-3.0.10', file: 'appleskin-fabric-mc26.2-3.0.10.jar', enabled: true },
    ];

    const appleSkinProject = { title: 'AppleSkin', slug: 'appleskin' };
    const appleSkinPlusMinusProject = { title: 'Appleskin+-', slug: 'appleskinplusminus' };

    // When regular appleskin is installed, Appleskin+- must NOT be marked as installed
    expect(isModInstalled(appleSkinProject, regularAppleSkin)).toBe(true);
    expect(isModInstalled(appleSkinPlusMinusProject, regularAppleSkin)).toBe(false);
    expect(findInstalledMod(appleSkinProject, regularAppleSkin)?.file).toBe(
      'appleskin-fabric-mc26.2-3.0.10.jar',
    );
    expect(findInstalledMod(appleSkinPlusMinusProject, regularAppleSkin)).toBeUndefined();

    // When Appleskin+- is installed, regular AppleSkin must NOT be marked as installed
    const plusMinusAppleSkin: Mod[] = [
      { name: 'appleskin+--1.0.3', file: 'appleskin+--1.0.3.jar', enabled: true },
    ];
    expect(isModInstalled(appleSkinProject, plusMinusAppleSkin)).toBe(false);
    expect(isModInstalled(appleSkinPlusMinusProject, plusMinusAppleSkin)).toBe(true);
    expect(findInstalledMod(appleSkinPlusMinusProject, plusMinusAppleSkin)?.file).toBe(
      'appleskin+--1.0.3.jar',
    );
    expect(findInstalledMod(appleSkinProject, plusMinusAppleSkin)).toBeUndefined();

    // Sodium vs Sodium Extra
    const sodiumInstalled: Mod[] = [
      { name: 'sodium-fabric-0.6.0', file: 'sodium-fabric-0.6.0.jar', enabled: true },
    ];
    const sodiumProject = { title: 'Sodium', slug: 'sodium' };
    const sodiumExtraProject = { title: 'Sodium Extra', slug: 'sodium-extra' };

    expect(isModInstalled(sodiumProject, sodiumInstalled)).toBe(true);
    expect(isModInstalled(sodiumExtraProject, sodiumInstalled)).toBe(false);
  });

  it('accurately matches Universal Graves jar and rejects other grave plugins', () => {
    const universalGravesMod: Mod = {
      name: 'graves-3.12.0+26.2',
      file: 'graves-3.12.0+26.2.jar',
      enabled: true,
      mod_id: 'universal-graves',
      title: 'Universal Graves',
      version: '3.12.0+26.2',
      sha1: '56c26a9318c739908dbd0c66cee85b842567c276',
    };
    const installed = [universalGravesMod];

    const universalGravesProject = {
      project_id: 'yn9u3ypm',
      title: 'Universal Graves',
      slug: 'universal-graves',
    };
    const lyGravesProject = {
      project_id: 'kieAM9Us',
      title: 'Graves',
      slug: 'ly-graves',
    };
    const playerGravesProject = {
      project_id: 'Lz6s3KKO',
      title: 'Graves',
      slug: 'player-graves',
    };
    const ketketGravesProject = {
      project_id: 'bYcfmIoG',
      title: 'Graves',
      slug: 'ketket-graves',
    };

    // 1. Universal Graves must be matched
    expect(isModInstalled(universalGravesProject, installed)).toBe(true);
    expect(findInstalledMod(universalGravesProject, installed)?.file).toBe('graves-3.12.0+26.2.jar');

    // 2. Other grave plugins must NOT be matched
    expect(isModInstalled(lyGravesProject, installed)).toBe(false);
    expect(findInstalledMod(lyGravesProject, installed)).toBeUndefined();

    expect(isModInstalled(playerGravesProject, installed)).toBe(false);
    expect(findInstalledMod(playerGravesProject, installed)).toBeUndefined();

    expect(isModInstalled(ketketGravesProject, installed)).toBe(false);
    expect(findInstalledMod(ketketGravesProject, installed)).toBeUndefined();

    // 3. Fallback without mod_id: author-prefixed slugs (ly-graves) must still reject graves-3.12.0.jar
    const uninspectedJar: Mod = {
      name: 'graves-3.12.0+26.2',
      file: 'graves-3.12.0+26.2.jar',
      enabled: true,
    };
    expect(isModInstalled(lyGravesProject, [uninspectedJar])).toBe(false);
    expect(findInstalledMod(lyGravesProject, [uninspectedJar])).toBeUndefined();
    expect(isModInstalled(playerGravesProject, [uninspectedJar])).toBe(false);

    // 4. SHA1 hash map match
    const hashProjectMap = new Map<string, string>([
      ['56c26a9318c739908dbd0c66cee85b842567c276', 'yn9u3ypm'],
    ]);
    const hashOnlyMod: Mod = {
      name: 'custom-file-name',
      file: 'custom-file-name.jar',
      enabled: true,
      sha1: '56c26a9318c739908dbd0c66cee85b842567c276',
    };
    expect(isModInstalled(universalGravesProject, [hashOnlyMod], hashProjectMap)).toBe(true);
    expect(isModInstalled(lyGravesProject, [hashOnlyMod], hashProjectMap)).toBe(false);
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
