import { ModChannelBadge } from './mod-channel-filter';
import { useEffect, useState } from 'react';
import { ExternalLink, Loader2, Package, X } from 'lucide-react';
import {
  fetchInstalledModDetails,
  getInstalledModProjectUrl,
  type InstalledModDetails,
} from '../api/mod-details';
import { MOD_PROVIDER_LABELS } from '../api/mod-icons';
import type { Mod, ModList } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { DialogBackdrop } from './ui/dialog-backdrop';

interface InstalledModDetailsDialogProps {
  mod: Mod;
  type: ModList['type'];
  onClose: () => void;
}

function safeExternalUrl(url?: string | null): string | undefined {
  if (url && /^https?:\/\//i.test(url)) return url;
}

export function InstalledModDetailsDialog({ mod, type, onClose }: InstalledModDetailsDialogProps) {
  const [details, setDetails] = useState<InstalledModDetails | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    setDetails(null);
    setLoading(true);
    setError(false);
    void fetchInstalledModDetails(mod, type, controller.signal)
      .then((result) => { if (!controller.signal.aborted) setDetails(result); })
      .catch(() => { if (!controller.signal.aborted) setError(true); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [mod, type]);

  const providerLabel = mod.provider ? MOD_PROVIDER_LABELS[mod.provider] : undefined;
  const hasCatalogProject = Boolean(mod.provider && (mod.project_id || mod.project_slug));
  const projectUrl = safeExternalUrl(details?.projectUrl) || getInstalledModProjectUrl(mod, type);
  const description = details?.description || mod.description;
  const links = [
    { label: 'Source code', url: safeExternalUrl(details?.sourceUrl) },
    { label: 'Wiki', url: safeExternalUrl(details?.wikiUrl) },
    { label: 'Issue tracker', url: safeExternalUrl(details?.issuesUrl) },
  ].filter((link) => link.url);

  return (
    <DialogBackdrop
      onDismiss={onClose}
      onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4"
    >
      <div className="flex max-h-[85vh] w-full max-w-xl flex-col rounded-xl border bg-card text-card-foreground shadow-2xl">
        <div className="flex items-start gap-3 border-b p-5">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-secondary text-muted-foreground">
            <Package aria-hidden="true" className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 className="break-words text-lg font-semibold">{details?.title || mod.title || mod.name}</h2>
            <p className="mt-1 text-xs text-muted-foreground">Installed {type === 'plugins' ? 'plugin' : 'mod'} details</p>
          </div>
          <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" aria-label="Close mod details" onClick={onClose}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="min-h-0 space-y-5 overflow-y-auto p-5">
          <div className="flex flex-wrap gap-2">
            <Badge variant="outline">{mod.enabled ? 'Enabled' : 'Disabled'}</Badge>
            {providerLabel && <Badge variant="secondary">{providerLabel}</Badge>}
            <ModChannelBadge channel={mod.release_type} />
            {details?.categories?.map((category) => <Badge key={category} variant="outline">{category}</Badge>)}
          </div>

          {description && <p className="whitespace-pre-wrap break-words text-sm leading-relaxed text-muted-foreground">{description}</p>}

          <dl className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
            <div className="sm:col-span-2">
              <dt className="text-xs text-muted-foreground">Installed file</dt>
              <dd className="mt-1 break-all font-mono text-xs">{mod.file}</dd>
            </div>
            <div>
              <dt className="text-xs text-muted-foreground">Installed version</dt>
              <dd className="mt-1 break-words">{mod.version || 'Unknown'}</dd>
            </div>
            {mod.mod_id && <div><dt className="text-xs text-muted-foreground">Mod ID</dt><dd className="mt-1 break-all">{mod.mod_id}</dd></div>}
            {mod.project_id && <div><dt className="text-xs text-muted-foreground">Project ID</dt><dd className="mt-1 break-all">{mod.project_id}</dd></div>}
            {mod.project_slug && <div><dt className="text-xs text-muted-foreground">Project slug</dt><dd className="mt-1 break-all">{mod.project_slug}</dd></div>}
            {details?.author && <div><dt className="text-xs text-muted-foreground">Author</dt><dd className="mt-1 break-words">{details.author}</dd></div>}
            {details && <div><dt className="text-xs text-muted-foreground">Project downloads</dt><dd className="mt-1">{details.downloads.toLocaleString()}</dd></div>}
            {details?.license && <div><dt className="text-xs text-muted-foreground">License</dt><dd className="mt-1 break-words">{details.license}</dd></div>}
            {mod.sha1 && <div className="sm:col-span-2"><dt className="text-xs text-muted-foreground">SHA-1</dt><dd className="mt-1 break-all font-mono text-xs">{mod.sha1}</dd></div>}
          </dl>

          {loading && <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />Loading catalog details…</p>}
          {error && <p role="status" className="text-sm text-muted-foreground">Catalog details are unavailable right now. Installed file information is still shown.</p>}
          {!loading && !error && !details && (
            <p className="text-sm text-muted-foreground">
              {hasCatalogProject
                ? 'Extended catalog details are unavailable for this project. Installed file information is still shown.'
                : 'No catalog project is linked to this file. Only installed file details are available.'}
            </p>
          )}

          {links.length > 0 && (
            <div className="flex flex-wrap gap-4">
              {links.map((link) => (
                <a key={link.label} href={link.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-sm text-primary hover:underline">
                  {link.label}<ExternalLink aria-hidden="true" className="h-3.5 w-3.5" />
                </a>
              ))}
            </div>
          )}
        </div>

        <div className="flex flex-wrap justify-end gap-2 border-t p-4">
          <Button variant="outline" onClick={onClose}>Close</Button>
          {projectUrl && providerLabel && (
            <Button asChild className="gap-2">
              <a href={projectUrl} target="_blank" rel="noopener noreferrer">
                Open in {providerLabel}<ExternalLink aria-hidden="true" className="h-4 w-4" />
              </a>
            </Button>
          )}
        </div>
      </div>
    </DialogBackdrop>
  );
}
