import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Mod, ModrinthProject, ModrinthVersion, CurseForgeFile, CurseForgeMod } from '../src/api/types';
import {
  resolveModrinthDependencies,
  resolveCurseForgeDependencies,
} from '../src/api/mod-dependencies';
import * as modrinthApi from '../src/api/modrinth';
import * as curseforgeApi from '../src/api/curseforge';

describe('Mod Dependencies Resolution Engine', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  describe('resolveModrinthDependencies', () => {
    it('resolves required library dependencies and ignores embedded/incompatible ones', async () => {
      const primaryVersion: ModrinthVersion = {
        id: 'ver-primary',
        project_id: 'proj-primary',
        author_id: 'author-1',
        name: 'Iris Shaders',
        version_number: '1.7.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 100,
        files: [{ url: 'https://example.com/iris.jar', filename: 'iris-1.7.0.jar', primary: true, size: 2048 }],
        dependencies: [
          {
            project_id: 'proj-sodium',
            version_id: null,
            file_name: null,
            dependency_type: 'required',
          },
          {
            project_id: 'proj-embedded-lib',
            version_id: null,
            file_name: null,
            dependency_type: 'embedded',
          },
          {
            project_id: 'proj-conflict-mod',
            version_id: null,
            file_name: null,
            dependency_type: 'incompatible',
          },
        ],
      };

      const sodiumProject: ModrinthProject = {
        id: 'proj-sodium',
        slug: 'sodium',
        title: 'Sodium',
        description: 'Rendering optimization engine',
        categories: ['optimization', 'library'],
        client_side: 'required',
        server_side: 'required',
        icon_url: 'https://example.com/sodium.png',
        downloads: 5000,
        followers: 1000,
      };

      const sodiumVersion: ModrinthVersion = {
        id: 'ver-sodium',
        project_id: 'proj-sodium',
        author_id: 'author-sodium',
        name: 'Sodium 0.5.8',
        version_number: '0.5.8',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 500,
        files: [{ url: 'https://example.com/sodium.jar', filename: 'sodium-fabric-0.5.8.jar', primary: true, size: 4096 }],
        dependencies: [],
      };

      vi.spyOn(modrinthApi, 'getProjects').mockResolvedValue([sodiumProject]);
      vi.spyOn(modrinthApi, 'getProjectVersions').mockResolvedValue([sodiumVersion]);

      const result = await resolveModrinthDependencies({
        version: primaryVersion,
        primaryProject: { id: 'proj-primary', slug: 'iris', title: 'Iris Shaders' },
        serverLoaders: ['fabric'],
        serverVersion: '1.20.1',
        installedMods: [],
      });

      expect(result.primaryTitle).toBe('Iris Shaders');
      expect(result.dependencies).toHaveLength(1);
      expect(result.missingCount).toBe(1);
      expect(result.satisfiedCount).toBe(0);

      const sodiumDep = result.dependencies[0];
      expect(sodiumDep.title).toBe('Sodium');
      expect(sodiumDep.dependencyType).toBe('required');
      expect(sodiumDep.isLibrary).toBe(true);
      expect(sodiumDep.alreadyInstalled).toBe(false);
      expect(sodiumDep.filename).toBe('sodium-fabric-0.5.8.jar');
      expect(sodiumDep.downloadUrl).toBe('https://example.com/sodium.jar');
    });

    it('identifies dependencies that are already installed on the server', async () => {
      const primaryVersion: ModrinthVersion = {
        id: 'ver-primary',
        project_id: 'proj-primary',
        author_id: 'author-1',
        name: 'Create Fabric',
        version_number: '0.5.1',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 100,
        files: [{ url: 'https://example.com/create.jar', filename: 'create-0.5.1.jar', primary: true, size: 10000 }],
        dependencies: [
          {
            project_id: 'proj-fabric-api',
            version_id: null,
            file_name: null,
            dependency_type: 'required',
          },
        ],
      };

      const fabricApiProject: ModrinthProject = {
        id: 'proj-fabric-api',
        slug: 'fabric-api',
        title: 'Fabric API',
        description: 'Core API library for Fabric mods',
        categories: ['library'],
        client_side: 'required',
        server_side: 'required',
        icon_url: 'https://example.com/fapi.png',
        downloads: 50000,
        followers: 10000,
      };

      vi.spyOn(modrinthApi, 'getProjects').mockResolvedValue([fabricApiProject]);

      const installedMods: Mod[] = [
        {
          name: 'fabric-api',
          file: 'fabric-api-0.92.0+1.20.1.jar',
          enabled: true,
          project_id: 'proj-fabric-api',
          project_slug: 'fabric-api',
        },
      ];

      const result = await resolveModrinthDependencies({
        version: primaryVersion,
        primaryProject: { id: 'proj-primary', slug: 'create-fabric', title: 'Create Fabric' },
        serverLoaders: ['fabric'],
        serverVersion: '1.20.1',
        installedMods,
      });

      expect(result.dependencies).toHaveLength(1);
      expect(result.missingCount).toBe(0);
      expect(result.satisfiedCount).toBe(1);

      const fapiDep = result.dependencies[0];
      expect(fapiDep.title).toBe('Fabric API');
      expect(fapiDep.alreadyInstalled).toBe(true);
      expect(fapiDep.installedFile).toBe('fabric-api-0.92.0+1.20.1.jar');
    });

    it('resolves transitive dependencies (Mod A -> Lib B -> Lib C)', async () => {
      const modAVersion: ModrinthVersion = {
        id: 'ver-a',
        project_id: 'proj-a',
        author_id: 'author',
        name: 'Mod A',
        version_number: '1.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 10,
        files: [{ url: 'https://example.com/modA.jar', filename: 'modA.jar', primary: true, size: 100 }],
        dependencies: [
          { project_id: 'proj-b', version_id: null, file_name: null, dependency_type: 'required' },
        ],
      };

      const projB: ModrinthProject = {
        id: 'proj-b',
        slug: 'lib-b',
        title: 'Library B',
        description: 'Intermediate library',
        categories: ['library'],
        client_side: 'required',
        server_side: 'required',
        icon_url: null,
        downloads: 10,
        followers: 1,
      };

      const verB: ModrinthVersion = {
        id: 'ver-b',
        project_id: 'proj-b',
        author_id: 'author',
        name: 'Lib B',
        version_number: '1.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 10,
        files: [{ url: 'https://example.com/libB.jar', filename: 'libB.jar', primary: true, size: 100 }],
        dependencies: [
          { project_id: 'proj-c', version_id: null, file_name: null, dependency_type: 'required' },
        ],
      };

      const projC: ModrinthProject = {
        id: 'proj-c',
        slug: 'lib-c',
        title: 'Library C',
        description: 'Base library',
        categories: ['library'],
        client_side: 'required',
        server_side: 'required',
        icon_url: null,
        downloads: 10,
        followers: 1,
      };

      const verC: ModrinthVersion = {
        id: 'ver-c',
        project_id: 'proj-c',
        author_id: 'author',
        name: 'Lib C',
        version_number: '1.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 10,
        files: [{ url: 'https://example.com/libC.jar', filename: 'libC.jar', primary: true, size: 100 }],
        dependencies: [],
      };

      vi.spyOn(modrinthApi, 'getProjects').mockImplementation(async (ids) => {
        const results: ModrinthProject[] = [];
        if (ids.includes('proj-b')) results.push(projB);
        if (ids.includes('proj-c')) results.push(projC);
        return results;
      });

      vi.spyOn(modrinthApi, 'getProjectVersions').mockImplementation(async (slug) => {
        if (slug === 'lib-b') return [verB];
        if (slug === 'lib-c') return [verC];
        return [];
      });

      const result = await resolveModrinthDependencies({
        version: modAVersion,
        primaryProject: { id: 'proj-a', slug: 'mod-a', title: 'Mod A' },
        serverLoaders: ['fabric'],
        serverVersion: '1.20.1',
        installedMods: [],
        maxDepth: 3,
      });

      expect(result.dependencies).toHaveLength(2);
      expect(result.missingCount).toBe(2);
      const titles = result.dependencies.map((d) => d.title);
      expect(titles).toContain('Library B');
      expect(titles).toContain('Library C');
    });

    it('prevents circular dependency loops', async () => {
      const verA: ModrinthVersion = {
        id: 'ver-a',
        project_id: 'proj-a',
        author_id: 'author',
        name: 'Mod A',
        version_number: '1.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 10,
        files: [{ url: 'https://example.com/a.jar', filename: 'a.jar', primary: true, size: 100 }],
        dependencies: [
          { project_id: 'proj-b', version_id: null, file_name: null, dependency_type: 'required' },
        ],
      };

      const projB: ModrinthProject = {
        id: 'proj-b',
        slug: 'mod-b',
        title: 'Mod B',
        description: 'B depends on A',
        categories: [],
        client_side: 'required',
        server_side: 'required',
        icon_url: null,
        downloads: 10,
        followers: 1,
      };

      const verB: ModrinthVersion = {
        id: 'ver-b',
        project_id: 'proj-b',
        author_id: 'author',
        name: 'Mod B',
        version_number: '1.0',
        game_versions: ['1.20.1'],
        version_type: 'release',
        loaders: ['fabric'],
        featured: true,
        status: 'listed',
        date_published: '2024-01-01T00:00:00Z',
        downloads: 10,
        files: [{ url: 'https://example.com/b.jar', filename: 'b.jar', primary: true, size: 100 }],
        dependencies: [
          // Circular reference back to A!
          { project_id: 'proj-a', version_id: null, file_name: null, dependency_type: 'required' },
        ],
      };

      vi.spyOn(modrinthApi, 'getProjects').mockResolvedValue([projB]);
      vi.spyOn(modrinthApi, 'getProjectVersions').mockResolvedValue([verB]);

      // Should complete cleanly without hanging in an infinite loop
      const result = await resolveModrinthDependencies({
        version: verA,
        primaryProject: { id: 'proj-a', slug: 'mod-a', title: 'Mod A' },
        serverLoaders: ['fabric'],
        serverVersion: '1.20.1',
        installedMods: [],
        maxDepth: 4,
      });

      expect(result.dependencies).toHaveLength(1);
      expect(result.dependencies[0].title).toBe('Mod B');
    });
  });

  describe('resolveCurseForgeDependencies', () => {
    it('resolves required CurseForge file dependencies', async () => {
      const file: CurseForgeFile = {
        id: 1001,
        gameId: 432,
        modId: 500,
        isAvailable: true,
        displayName: 'Example Mod 1.0',
        fileName: 'example-1.0.jar',
        releaseType: 1,
        fileStatus: 4,
        hashes: [],
        fileDate: '2024-01-01T00:00:00Z',
        fileLength: 5000,
        downloadCount: 100,
        downloadUrl: 'https://example.com/example.jar',
        gameVersions: ['1.20.1'],
        dependencies: [
          { modId: 600, relationType: 3 }, // 3 = Required
        ],
      };

      const depMod: CurseForgeMod = {
        id: 600,
        gameId: 432,
        name: 'Required Curse Library',
        slug: 'required-curse-lib',
        summary: 'A required library mod',
        links: { websiteUrl: '' },
        status: 4,
        downloadCount: 1000,
        isFeatured: false,
        primaryCategoryId: 6,
        categories: [{ id: 1, gameId: 432, name: 'Library', slug: 'library', url: '', iconUrl: '' }],
        authors: [],
        logo: { id: 1, modId: 600, title: '', description: '', thumbnailUrl: '', url: '' },
        mainFileId: 2001,
        latestFiles: [],
        dateCreated: '',
        dateModified: '',
        dateReleased: '',
        allowModDistribution: true,
        gamePopularityRank: 1,
        isAvailable: true,
      };

      const depFile: CurseForgeFile = {
        id: 2001,
        gameId: 432,
        modId: 600,
        isAvailable: true,
        displayName: 'Required Lib 1.0',
        fileName: 'required-lib-1.0.jar',
        releaseType: 1,
        fileStatus: 4,
        hashes: [],
        fileDate: '2024-01-01T00:00:00Z',
        fileLength: 3000,
        downloadCount: 500,
        downloadUrl: 'https://example.com/lib.jar',
        gameVersions: ['1.20.1'],
      };

      vi.spyOn(curseforgeApi, 'getCurseForgeMods').mockResolvedValue([depMod]);
      vi.spyOn(curseforgeApi, 'getCurseForgeFiles').mockResolvedValue([depFile]);

      const result = await resolveCurseForgeDependencies({
        file,
        primaryMod: { id: 500, slug: 'example', name: 'Example Mod' },
        apiKey: 'test-key',
        gameVersion: '1.20.1',
        modLoaderType: 4,
        installedMods: [],
      });

      expect(result.dependencies).toHaveLength(1);
      expect(result.missingCount).toBe(1);
      expect(result.dependencies[0].title).toBe('Required Curse Library');
      expect(result.dependencies[0].dependencyType).toBe('required');
      expect(result.dependencies[0].isLibrary).toBe(true);
      expect(result.dependencies[0].filename).toBe('required-lib-1.0.jar');
      expect(result.dependencies[0].downloadUrl).toBe('https://example.com/lib.jar');
    });
  });
});
