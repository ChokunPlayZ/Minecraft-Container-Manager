import { useState, type ReactNode, type ComponentType } from 'react';
import {
  Check,
  Compass,
  Download,
  ExternalLink,
  Flame,
  Layers,
  Loader2,
  Package,
  Star,
  Trash2,
} from 'lucide-react';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { formatCount } from '../api/modrinth';

export type PluginProvider = 'modrinth' | 'hangar' | 'spiget' | 'curseforge';

interface ProviderTheme {
  borderHover: string;
  titleHover: string;
  iconBox: string;
  installButton: string;
  defaultIcon: ComponentType<{ className?: string }>;
}

const PROVIDER_THEMES: Record<PluginProvider, ProviderTheme> = {
  modrinth: {
    borderHover: 'hover:border-emerald-500/40',
    titleHover: 'group-hover:text-emerald-600 dark:group-hover:text-emerald-400',
    iconBox: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
    installButton: 'bg-emerald-600 hover:bg-emerald-700 text-white',
    defaultIcon: Compass,
  },
  hangar: {
    borderHover: 'hover:border-sky-500/40',
    titleHover: 'group-hover:text-sky-600 dark:group-hover:text-sky-400',
    iconBox: 'bg-sky-500/10 text-sky-600 dark:text-sky-400',
    installButton: 'bg-sky-600 hover:bg-sky-700 text-white',
    defaultIcon: Layers,
  },
  spiget: {
    borderHover: 'hover:border-amber-500/40',
    titleHover: 'group-hover:text-amber-600 dark:group-hover:text-amber-400',
    iconBox: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
    installButton: 'bg-amber-600 hover:bg-amber-700 text-white',
    defaultIcon: Package,
  },
  curseforge: {
    borderHover: 'hover:border-orange-500/40',
    titleHover: 'group-hover:text-orange-600 dark:group-hover:text-orange-400',
    iconBox: 'bg-orange-500/10 text-orange-600 dark:text-orange-400',
    installButton: 'bg-orange-600 hover:bg-orange-700 text-white',
    defaultIcon: Flame,
  },
};

export interface PluginCardProps {
  id: string | number;
  title: string;
  author?: string;
  description?: string;
  iconUrl?: string | null;
  provider?: PluginProvider;
  fallbackIcon?: ComponentType<{ className?: string }>;

  // Status
  isInstalled?: boolean;
  installedJar?: string;
  isInstalling?: boolean;

  // Stats
  downloads?: number;
  rating?: number | null;
  stars?: number;
  follows?: number;

  // Badges & metadata
  category?: string;
  badges?: ReactNode;

  // External link
  externalUrl?: string;
  externalLabel?: string;

  // Actions
  installLabel?: string;
  onInstall?: () => void | Promise<void>;
  installDisabled?: boolean;

  onDelete?: () => void | Promise<void>;
  deleteTitle?: string;
  deleteAriaLabel?: string;

  onViewDetails?: () => void;
  detailsLabel?: string;
  detailsTitle?: string;

  onClickTitle?: () => void;
}

export function PluginCard({
  title,
  author,
  description,
  iconUrl,
  provider = 'modrinth',
  fallbackIcon,
  isInstalled = false,
  installedJar,
  isInstalling = false,
  downloads,
  rating,
  stars,
  follows,
  category,
  badges,
  externalUrl,
  externalLabel,
  installLabel = 'Install',
  onInstall,
  installDisabled = false,
  onDelete,
  deleteTitle,
  deleteAriaLabel,
  onViewDetails,
  detailsLabel = 'Details',
  detailsTitle,
  onClickTitle,
}: PluginCardProps) {
  const [imgError, setImgError] = useState(false);
  const theme = PROVIDER_THEMES[provider] || PROVIDER_THEMES.modrinth;
  const FallbackIcon = fallbackIcon || theme.defaultIcon;

  return (
    <div
      className={`group relative flex flex-col justify-between rounded-xl border border-border/80 bg-card/60 p-4 shadow-2xs transition-all ${theme.borderHover} hover:shadow-md hover:bg-card`}
    >
      <div>
        <div className="flex items-start gap-3">
          {iconUrl && !imgError ? (
            <img
              src={iconUrl}
              alt={title}
              className="h-11 w-11 shrink-0 rounded-lg object-contain bg-secondary/30 p-1 border border-border/40"
              loading="lazy"
              onError={() => setImgError(true)}
            />
          ) : (
            <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-lg ${theme.iconBox}`}>
              <FallbackIcon className="h-6 w-6" />
            </div>
          )}

          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-1.5">
              <h4
                className={`truncate font-semibold text-foreground text-sm transition-colors ${theme.titleHover} ${
                  onClickTitle || onViewDetails ? 'cursor-pointer' : ''
                }`}
                onClick={onClickTitle || onViewDetails}
                title={title}
              >
                {title}
              </h4>
              {isInstalled && (
                <Badge
                  variant="secondary"
                  className="bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 text-[10px] px-1.5 py-0 shrink-0 font-medium"
                >
                  Installed
                </Badge>
              )}
            </div>

            {author && (
              <p className="truncate text-xs text-muted-foreground">
                by <span className="font-medium text-foreground/80">{author}</span>
              </p>
            )}
          </div>
        </div>

        <p className="mt-2.5 line-clamp-2 text-xs text-muted-foreground leading-relaxed">
          {description || 'No description provided.'}
        </p>

        {installedJar && (
          <div className="mt-2 flex items-center gap-1.5 rounded bg-emerald-500/10 px-2 py-0.5 text-[11px] font-mono text-emerald-700 dark:text-emerald-300">
            <Check className="h-3 w-3 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <span className="truncate">Installed: {installedJar}</span>
          </div>
        )}

        {badges && (
          <div className="mt-2.5 flex flex-wrap items-center gap-1.5 text-xs">
            {badges}
          </div>
        )}
      </div>

      <div className="mt-4 space-y-3 pt-2 border-t border-border/40">
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <div className="flex items-center gap-3">
            {downloads !== undefined && (
              <span className="inline-flex items-center gap-1" title="Downloads">
                <Download className="h-3 w-3 text-muted-foreground/80" />
                {formatCount(downloads)}
              </span>
            )}
            {stars !== undefined && (
              <span className="inline-flex items-center gap-1" title="Stars">
                <Star className="h-3 w-3 text-amber-500" />
                {formatCount(stars)}
              </span>
            )}
            {rating !== undefined && rating !== null && rating > 0 && (
              <span className="inline-flex items-center gap-1" title="Rating">
                <Star className="h-3 w-3 text-amber-500 fill-amber-500" />
                {rating.toFixed(1)}
              </span>
            )}
            {follows !== undefined && (
              <span className="inline-flex items-center gap-1" title="Followers">
                <Flame className="h-3 w-3 text-amber-500" />
                {formatCount(follows)}
              </span>
            )}
          </div>

          <div className="flex items-center gap-1.5">
            {category && (
              <Badge variant="outline" className="text-[10px] px-1.5 py-0 uppercase">
                {category}
              </Badge>
            )}
            {externalUrl && (
              <a
                href={externalUrl}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
              >
                {externalLabel || 'View'}
                <ExternalLink className="h-2.5 w-2.5" />
              </a>
            )}
          </div>
        </div>

        <div className="flex items-center gap-1.5 pt-1">
          <Button
            size="sm"
            onClick={onInstall}
            disabled={isInstalling || (isInstalled && !onInstall) || installDisabled}
            className={`flex-1 h-8 text-xs gap-1.5 ${
              isInstalled
                ? 'border border-emerald-500/40 bg-emerald-500/10 text-emerald-700 hover:bg-emerald-500/20 dark:text-emerald-300'
                : theme.installButton
            }`}
            title={
              isInstalled
                ? installDisabled
                  ? 'Installed'
                  : 'Click to reinstall or update'
                : installLabel
            }
          >
            {isInstalling ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Installing...
              </>
            ) : isInstalled ? (
              <>
                <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
                Installed
              </>
            ) : (
              <>
                <Download className="h-3.5 w-3.5" />
                {installLabel}
              </>
            )}
          </Button>

          {isInstalled && onDelete && (
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:bg-destructive/10 hover:text-destructive transition-colors shrink-0"
              title={deleteTitle || (installedJar ? `Delete ${installedJar} from server` : 'Delete jar from server')}
              aria-label={deleteAriaLabel || deleteTitle || (installedJar ? `Delete ${installedJar}` : 'Delete jar')}
              onClick={onDelete}
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}

          {onViewDetails && (
            <Button
              variant="outline"
              size="sm"
              onClick={onViewDetails}
              className="h-8 px-2.5 text-xs text-muted-foreground hover:text-foreground shrink-0 gap-1"
              title={detailsTitle || 'View versions and details'}
              aria-label={detailsLabel}
            >
              <Layers className="h-3.5 w-3.5" />
              <span className="sr-only">{detailsLabel}</span>
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

export function PluginCardSkeleton() {
  return (
    <div className="flex h-48 flex-col justify-between rounded-xl border border-border/60 bg-card/40 p-4 animate-pulse">
      <div>
        <div className="flex items-start gap-3">
          <div className="h-11 w-11 shrink-0 rounded-lg bg-secondary/80" />
          <div className="space-y-1.5 flex-1 min-w-0">
            <div className="h-4 w-28 rounded-sm bg-secondary/80" />
            <div className="h-3 w-16 rounded-sm bg-secondary/60" />
          </div>
        </div>
        <div className="mt-3 space-y-2">
          <div className="h-3 w-full rounded-sm bg-secondary/50" />
          <div className="h-3 w-3/4 rounded-sm bg-secondary/40" />
        </div>
      </div>
      <div className="mt-4 pt-2 border-t border-border/40 space-y-2.5">
        <div className="flex items-center justify-between">
          <div className="h-3 w-20 rounded-sm bg-secondary/50" />
          <div className="h-3 w-14 rounded-sm bg-secondary/50" />
        </div>
        <div className="h-8 w-full rounded-md bg-secondary/60" />
      </div>
    </div>
  );
}

export function PluginGridSkeleton({ count = 6 }: { count?: number }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: count }).map((_, i) => (
        <PluginCardSkeleton key={i} />
      ))}
    </div>
  );
}

export function PluginEmptyState({
  title,
  description,
  action,
  icon: Icon = Package,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  icon?: ComponentType<{ className?: string }>;
}) {
  return (
    <div className="rounded-xl border border-dashed border-border/80 py-12 px-4 text-center">
      <Icon className="mx-auto h-8 w-8 text-muted-foreground/60" />
      <p className="mt-3 text-sm font-medium text-foreground">{title}</p>
      {description && (
        <p className="mt-1 text-xs text-muted-foreground max-w-md mx-auto">{description}</p>
      )}
      {action && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}
