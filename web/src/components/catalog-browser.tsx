import { useState } from 'react';
import { Compass, Flame, Layers, Package } from 'lucide-react';
import type { Mod, ModProvider, Server } from '../api/types';
import { Badge } from './ui/badge';
import { CurseForgeBrowser } from './curseforge-browser';
import { HangarBrowser } from './hangar-browser';
import { ModrinthBrowser } from './modrinth-browser';
import { SpigetBrowser } from './spiget-browser';

interface CatalogBrowserProps {
  server: Server;
  installedMods: Mod[];
  onModInstalled: (mod: Mod) => void;
  onModDeleted?: (modName: string) => void;
  defaultProvider?: ModProvider;
}

export function CatalogBrowser({
  server,
  installedMods,
  onModInstalled,
  onModDeleted,
  defaultProvider = 'modrinth',
}: CatalogBrowserProps) {
  const [activeProvider, setActiveProvider] = useState<ModProvider>(defaultProvider);

  const isPluginServer = server.server_type === 'paper' || server.server_type === 'spigot';

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
      activeClass: 'bg-emerald-600 text-white shadow-sm border-emerald-600',
      icon: Compass,
    },
    {
      id: 'hangar',
      name: 'PaperMC Hangar',
      subtitle: 'Paper / Purpur / Folia',
      color: 'sky',
      activeClass: 'bg-sky-600 text-white shadow-sm border-sky-600',
      icon: Layers,
      badge: !isPluginServer ? 'Paper only' : undefined,
    },
    {
      id: 'spiget',
      name: 'SpigotMC',
      subtitle: '100k+ Plugins',
      color: 'amber',
      activeClass: 'bg-amber-600 text-white shadow-sm border-amber-600',
      icon: Package,
      badge: !isPluginServer ? 'Bukkit only' : undefined,
    },
    {
      id: 'curseforge',
      name: 'CurseForge',
      subtitle: 'Mods & Plugins',
      color: 'orange',
      activeClass: 'bg-orange-600 text-white shadow-sm border-orange-600',
      icon: Flame,
    },
  ];

  return (
    <div className="space-y-4">
      {/* Provider Selector Bar */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between rounded-xl border border-border/80 bg-card/60 p-2 shadow-2xs">
        <div className="flex flex-wrap items-center gap-1.5">
          {providers.map((p) => {
            const Icon = p.icon;
            const isActive = activeProvider === p.id;

            return (
              <button
                key={p.id}
                type="button"
                onClick={() => setActiveProvider(p.id)}
                className={`group flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs font-semibold transition-all ${
                  isActive
                    ? p.activeClass
                    : 'border-transparent text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                }`}
              >
                <Icon className={`h-4 w-4 ${isActive ? 'text-white' : 'text-muted-foreground group-hover:text-foreground'}`} />
                <span className="font-semibold">{p.name}</span>
                {p.badge ? (
                  <span
                    className={`rounded-full px-1.5 py-0.2 text-[9px] font-medium ${
                      isActive
                        ? 'bg-white/20 text-white'
                        : 'bg-secondary text-muted-foreground'
                    }`}
                  >
                    {p.badge}
                  </span>
                ) : (
                  <span
                    className={`hidden md:inline text-[10px] font-normal opacity-80 ${
                      isActive ? 'text-white/90' : 'text-muted-foreground'
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
          <Badge variant="outline" className="text-[10px] font-semibold uppercase px-1.5 py-0">
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
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'hangar' && (
        <HangarBrowser
          server={server}
          installedMods={installedMods}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'spiget' && (
        <SpigetBrowser
          server={server}
          installedMods={installedMods}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}

      {activeProvider === 'curseforge' && (
        <CurseForgeBrowser
          server={server}
          installedMods={installedMods}
          onModInstalled={onModInstalled}
          onModDeleted={onModDeleted}
        />
      )}
    </div>
  );
}
