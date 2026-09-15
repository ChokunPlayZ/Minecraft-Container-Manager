import { useState } from 'react';
import {
  AlertTriangle,
  ArrowRight,
  Box,
  CheckCircle2,
  Download,
  Loader2,
  X,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { InstallModpackOptions, ModpackManifest, Server } from '../api/types';
import { Badge } from './ui/badge';
import { Button } from './ui/button';

interface ModpackInstallDialogProps {
  server: Server;
  manifest: ModpackManifest;
  installOptions: InstallModpackOptions;
  modpackFile?: File;
  isUpdate?: boolean;
  onClose: () => void;
  onInstalled: () => void;
}

export function ModpackInstallDialog({
  server,
  manifest,
  installOptions,
  modpackFile,
  isUpdate = false,
  onClose,
  onInstalled,
}: ModpackInstallDialogProps) {
  const [autoConfigure, setAutoConfigure] = useState(true);
  const [installing, setInstalling] = useState(false);
  const [progress, setProgress] = useState<number | null>(null);
  const [statusText, setStatusText] = useState<string>('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  // Check compatibility
  const serverTypeMatch =
    !manifest.loader ||
    server.server_type.toLowerCase() === manifest.loader.toLowerCase() ||
    (manifest.loader.toLowerCase() === 'quilt' && server.server_type === 'fabric');

  const versionMatch =
    !manifest.minecraft_version || server.version === manifest.minecraft_version;

  const needsReconfig = !serverTypeMatch || !versionMatch;

  async function handleInstall() {
    setInstalling(true);
    setError(null);
    setProgress(10);
    setStatusText(isUpdate ? 'Preparing modpack update...' : 'Preparing modpack installation...');

    try {
      if (modpackFile) {
        setStatusText(isUpdate ? 'Uploading & applying modpack update...' : 'Uploading & extracting modpack...');
        await api.installModpackFile(
          server.id,
          modpackFile,
          autoConfigure,
          false,
          (loaded, total) => {
            if (total > 0) {
              const pct = Math.round((loaded / total) * 60);
              setProgress(10 + pct);
              setStatusText(`Uploading modpack archive (${Math.round((loaded / (1024 * 1024)) * 10) / 10} MB)...`);
            }
          },
        );
      } else {
        setStatusText(isUpdate ? 'Downloading & updating modpack files...' : 'Downloading & installing modpack files...');
        setProgress(40);
        await api.installModpackRemote(server.id, {
          ...installOptions,
          auto_configure_server: autoConfigure,
        });
      }

      setProgress(100);
      setStatusText(isUpdate ? 'Update complete!' : 'Installation complete!');
      setSuccess(true);
      setTimeout(() => {
        onInstalled();
        onClose();
      }, 1200);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : (isUpdate ? 'Failed to update modpack' : 'Failed to install modpack'));
      setProgress(null);
    } finally {
      setInstalling(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-xs p-4"
      onClick={() => !installing && onClose()}
    >
      <div
        className="w-full max-w-lg rounded-2xl border border-border/90 bg-card p-6 shadow-2xl transition-all"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-start justify-between gap-3 border-b border-border/70 pb-4">
          <div className="flex items-center gap-3">
            {manifest.icon_url ? (
              <img
                src={manifest.icon_url}
                alt={manifest.name}
                className="h-12 w-12 rounded-xl object-cover border border-border shadow-xs"
              />
            ) : (
              <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-primary/10 border border-primary/20 text-primary">
                <Box className="h-6 w-6" />
              </div>
            )}
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-lg font-bold text-foreground leading-tight">
                  {manifest.name}
                </h2>
                <Badge variant="outline" className="text-[10px] font-semibold uppercase">
                  {manifest.format}
                </Badge>
                {isUpdate && (
                  <Badge variant="secondary" className="bg-primary/10 text-primary border-primary/20 text-[10px] font-semibold uppercase">
                    Update
                  </Badge>
                )}
              </div>
              <p className="text-xs text-muted-foreground mt-0.5">
                Version {manifest.version}
                {manifest.author ? ` by ${manifest.author}` : ''}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={installing}
            className="rounded-lg p-1 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body Content */}
        <div className="mt-4 space-y-4">
          {manifest.summary && (
            <p className="text-xs text-muted-foreground line-clamp-2 bg-secondary/30 p-2.5 rounded-lg border border-border/40">
              {manifest.summary}
            </p>
          )}

          {/* Specs Grid */}
          <div className="grid grid-cols-3 gap-2 text-center text-xs">
            <div className="rounded-xl border border-border/70 bg-secondary/20 p-2.5">
              <span className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider block">
                Loader
              </span>
              <span className="font-bold text-foreground capitalize mt-0.5 block">
                {manifest.loader || 'Fabric'}
              </span>
              {manifest.loader_version && (
                <span className="text-[10px] text-muted-foreground truncate block">
                  v{manifest.loader_version}
                </span>
              )}
            </div>

            <div className="rounded-xl border border-border/70 bg-secondary/20 p-2.5">
              <span className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider block">
                Game Version
              </span>
              <span className="font-bold text-foreground mt-0.5 block">
                {manifest.minecraft_version || 'Any'}
              </span>
            </div>

            <div className="rounded-xl border border-border/70 bg-secondary/20 p-2.5">
              <span className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider block">
                Mods to Install
              </span>
              <span className="font-bold text-foreground mt-0.5 block">
                {manifest.server_files > 0 ? manifest.server_files : manifest.total_files}
              </span>
              {manifest.client_only_files > 0 && (
                <span className="text-[10px] text-muted-foreground block">
                  ({manifest.client_only_files} client-only skipped)
                </span>
              )}
            </div>
          </div>

          {/* Software Reconfiguration Notice */}
          {needsReconfig ? (
            <div className="rounded-xl border border-amber-500/30 bg-amber-500/10 p-3.5 space-y-2">
              <div className="flex items-start gap-2.5">
                <AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 shrink-0 mt-0.5" />
                <div className="text-xs">
                  <span className="font-semibold text-amber-900 dark:text-amber-200">
                    Server Software Mismatch Detected
                  </span>
                  <div className="mt-1 flex items-center gap-1.5 text-muted-foreground">
                    <span className="font-medium text-foreground capitalize">
                      {server.server_type} {server.version}
                    </span>
                    <ArrowRight className="h-3 w-3" />
                    <span className="font-semibold text-amber-600 dark:text-amber-400 capitalize">
                      {manifest.loader} {manifest.minecraft_version}
                    </span>
                  </div>
                </div>
              </div>

              <label className="flex items-center gap-2 text-xs font-medium cursor-pointer pt-1 border-t border-amber-500/20">
                <input
                  type="checkbox"
                  checked={autoConfigure}
                  onChange={(e) => setAutoConfigure(e.target.checked)}
                  disabled={installing}
                  className="rounded border-amber-500/40 text-primary focus:ring-primary h-4 w-4"
                />
                <span>
                  Automatically reconfigure server to{' '}
                  <strong className="capitalize">{manifest.loader} {manifest.minecraft_version}</strong>
                </span>
              </label>
            </div>
          ) : (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-800 dark:text-emerald-300">
              <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
              <span>
                Compatible with this server software (<strong>{server.server_type} {server.version}</strong>).
              </span>
            </div>
          )}

          {/* Progress / Status */}
          {installing && (
            <div className="space-y-2 rounded-xl border border-primary/20 bg-primary/5 p-3.5">
              <div className="flex items-center justify-between text-xs">
                <span className="font-medium text-foreground flex items-center gap-2">
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-primary" />
                  {statusText}
                </span>
                {progress !== null && (
                  <span className="font-bold text-primary">{progress}%</span>
                )}
              </div>
              {progress !== null && (
                <div className="h-2 w-full overflow-hidden rounded-full bg-secondary">
                  <div
                    className="h-full bg-primary transition-all duration-300"
                    style={{ width: `${progress}%` }}
                  />
                </div>
              )}
            </div>
          )}

          {/* Success message */}
          {success && (
            <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 p-3 text-xs text-emerald-800 dark:text-emerald-300 font-medium">
              <CheckCircle2 className="h-4 w-4 text-emerald-600" />
              {isUpdate ? 'Modpack updated successfully! Refreshing server...' : 'Modpack installed successfully! Refreshing server...'}
            </div>
          )}

          {/* Error display */}
          {error && (
            <div className="rounded-xl border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">
              {error}
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="mt-6 flex items-center justify-end gap-2.5 border-t border-border/70 pt-4">
          <Button
            variant="outline"
            size="sm"
            onClick={onClose}
            disabled={installing}
          >
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={() => void handleInstall()}
            disabled={installing || success}
            className="gap-1.5 bg-primary text-primary-foreground shadow-xs font-semibold"
          >
            {installing ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                {isUpdate ? 'Updating...' : 'Installing...'}
              </>
            ) : (
              <>
                <Download className="h-4 w-4" />
                {isUpdate ? 'Update Modpack' : 'Install Modpack'}
              </>
            )}
          </Button>
        </div>
      </div>
    </div>
  );
}
