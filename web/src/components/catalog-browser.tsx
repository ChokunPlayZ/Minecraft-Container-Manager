import { useState } from 'react';
import { Compass, Flame, Layers, Package } from 'lucide-react';
import type { Mod, ModProvider, Server } from '../api/types';
import type { ModUpdateInfo } from '../api/mod-updates';
import { Badge } from './ui/badge';
import { isPluginServerType } from '../api/curseforge';
import { CurseForgeBrowser } from './curseforge-browser';
import { HangarBrowser } from './hangar-browser';
import { ModrinthBrowser } from './modrinth-browser';
import { SpigetBrowser } from './spiget-browser';

interface CatalogBrowserProps {
  server: Server;
  installedMods: Mod[];
  updates?: Record<string, ModUpdateInfo>;
  onOpenPicker?: (mod: Mod, updateInfo?: ModUpdateInfo) => void;
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
  defaultProvider?: ModProvider;
}

export function CatalogBrowser({
  server,
  installedMods,
  updates,
  onOpenPicker,
  onModInstalled,
  onModDeleted,
  defaultProvider = 'modrinth',
}: CatalogBrowserProps) {
  const [activeProvider, setActiveProvider] = useState<ModProvider>(defaultProvider);

  const isPluginServer = isPluginServerType(server.server_type);

  const providers: {
    id: ModProvider;
    name: string;
    subtitle: string;
    color: string;
    activeClass: string;
    icon: typeof Compass;
    badge?: string;
  }[] = [
    {
      id: 'modrinth',
      name: 'Modrinth',
      subtitle: 'Mods & Plugins',
      color: 'emerald',
      activeClass: 'bg-accent text-accent-foreground border-border',
      icon: Compass,
    },
    {
      id: 'hangar',
      name: 'PaperMC Hangar',
      subtitle: 'Paper / Purpur / Folia',
      color: 'sky',
      activeClass: 'bg-accent text-accent-foreground border-border',
      icon: Layers,
      badge: !isPluginServer ? 'Paper only' : undefined,
    },
    {
      id: 'spiget',
      name: 'SpigotMC',
      subtitle: '100k+ Plugins',
      color: 'amber',
      activeClass: 'bg-accent text-accent-foreground border-border',
      icon: Package,
      badge: !isPluginServer ? 'Bukkit only' : undefined,
    },
    {
      id: 'curseforge',
      name: 'CurseForge',
      subtitle: 'Mods & Plugins',
      color: 'orange',
      activeClass: 'bg-accent text-accent-foreground border-border',
      icon: Flame,
    },
  ];

  return (
    <div className="space-y-4">
      {/* Provider Selector Bar */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between rounded-lg border bg-card p-2 ">
        <div className="flex flex-wrap items-center gap-1.5">
          {providers.map((p) => {
            const Icon = p.icon;
            const isActive = activeProvider === p.id;

            return (
              <button
                key={p.id}
                type="button"
                onClick={() => setActiveProvider(p.id)}
                aria-pressed={isActive}
                className={`group flex items-center gap-2 rounded-lg border px-3 py-2 text-xs font-medium transition-colors ${
                  isActive
                    ? p.activeClass
                    : 'border-transparent text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                }`}
              >
                <Icon className={`h-4 w-4 ${isActive ? 'text-primary' : 'text-muted-foreground group-hover:text-foreground'}`} />
                <span className="font-semibold">{p.name}</span>
                {p.badge ? (
                  <span
                    className={`rounded-full px-1.5 py-0.2 text-[11px] font-medium ${
                      isActive
                        ? 'bg-card text-primary'
                        : 'bg-secondary text-muted-foreground'
                    }`}
                  >
                    {p.badge}
                  </span>
                ) : (
                  <span
                    className={`hidden md:inline text-[11px] font-normal opacity-80 ${
                      isActive ? 'text-accent-foreground' : 'text-muted-foreground'
                    }`}
                  >
                    · {p.subtitle}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground px-2">
          <span>Active catalog:</span>
          <Badge variant="outline" className="text-[11px] font-semibold uppercase px-1.5 py-0">
            {activeProvider === 'modrinth'
              ? 'Modrinth'
              : activeProvider === 'hangar'
              ? 'PaperMC Hangar'
              : activeProvider === 'spiget'
              ? 'SpigotMC'
              : 'CurseForge'}
          </Badge>
        </div>
      </div>

      {/* Provider Content */}
      {activeProvider === 'modrinth' && (
        <ModrinthBrowser
          server={server}
          installedMods={installedMods}
          updates={updates}
          onOpenPicker={onOpenPicker}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'hangar' && (
        <HangarBrowser
          server={server}
          installedMods={installedMods}
          updates={updates}
          onOpenPicker={onOpenPicker}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'spiget' && (
        <SpigetBrowser
          server={server}
          installedMods={installedMods}
          updates={updates}
          onOpenPicker={onOpenPicker}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'curseforge' && (
        <CurseForgeBrowser
          server={server}
          installedMods={installedMods}
          updates={updates}
          onOpenPicker={onOpenPicker}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}
    </div>
  );
}
