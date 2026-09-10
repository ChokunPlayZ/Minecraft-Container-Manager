import { useEffect } from 'react';
import {
  AlertTriangle,
  ArrowRight,
  Download,
  Loader2,
  Trash2,
  X,
} from 'lucide-react';
import { Badge } from './ui/badge';
import { Button } from './ui/button';

export interface ModUpdateDialogProps {
  isOpen: boolean;
  modTitle: string;
  installedJar: string;
  newJar: string;
  isInstalling?: boolean;
  onConfirmDeleteAndInstall: () => void | Promise<void>;
  onConfirmKeepAndInstall: () => void | Promise<void>;
  onDeleteOldOnly?: () => void | Promise<void>;
  onCancel: () => void;
}

export function ModUpdateDialog({
  isOpen,
  modTitle,
  installedJar,
  newJar,
  isInstalling = false,
  onConfirmDeleteAndInstall,
  onConfirmKeepAndInstall,
  onDeleteOldOnly,
  onCancel,
}: ModUpdateDialogProps) {
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !isInstalling) {
        onCancel();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, isInstalling, onCancel]);

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-[110] flex items-center justify-center bg-black/60 backdrop-blur-xs p-4"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !isInstalling) onCancel();
      }}
    >
      <div
        className="relative w-full max-w-lg rounded-xl border border-border bg-card p-6 text-card-foreground shadow-2xl transition-all animate-in fade-in zoom-in-95 duration-150"
        role="dialog"
        aria-modal="true"
        aria-labelledby="mod-update-title"
      >
        <button
          type="button"
          onClick={onCancel}
          disabled={isInstalling}
          className="absolute right-4 top-4 rounded-md p-1 text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors disabled:opacity-50"
          aria-label="Close dialog"
        >
          <X className="h-4 w-4" />
        </button>

        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-amber-500/15 text-amber-600 dark:text-amber-400">
            <AlertTriangle className="h-5 w-5" />
          </div>
          <div className="min-w-0 flex-1">
            <h3 id="mod-update-title" className="text-base font-semibold leading-tight text-foreground">
              Older Jar Version Detected
            </h3>
            <p className="mt-1 text-xs text-muted-foreground">
              An existing version of <span className="font-semibold text-foreground">{modTitle}</span> is currently installed on your server.
            </p>
          </div>
        </div>

        {/* Comparison card */}
        <div className="mt-4 space-y-2 rounded-lg border border-border/80 bg-secondary/30 p-3 text-xs">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <span className="font-medium text-muted-foreground shrink-0">Installed:</span>
              <span className="font-mono text-destructive/90 dark:text-red-400 font-semibold truncate" title={installedJar}>
                {installedJar}
              </span>
            </div>
            <Badge variant="outline" className="border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-300 shrink-0 text-[10px] px-1.5 py-0">
              Old Version
            </Badge>
          </div>

          <div className="flex items-center justify-center py-0.5 text-muted-foreground">
            <ArrowRight className="h-3.5 w-3.5 rotate-90 sm:rotate-0" />
          </div>

          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <span className="font-medium text-muted-foreground shrink-0">Incoming:</span>
              <span className="font-mono text-emerald-600 dark:text-emerald-400 font-semibold truncate" title={newJar}>
                {newJar}
              </span>
            </div>
            <Badge variant="outline" className="border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 shrink-0 text-[10px] px-1.5 py-0">
              New Version
            </Badge>
          </div>
        </div>

        <p className="mt-3 text-xs text-muted-foreground leading-relaxed">
          Minecraft server software loads all jar files in your mods/plugins directory. Keeping both versions can lead to duplicate class collisions, startup failures, or unexpected errors.
        </p>

        {/* Action buttons */}
        <div className="mt-6 flex flex-col gap-2 sm:flex-row-reverse sm:items-center sm:justify-between">
          <div className="flex flex-col sm:flex-row gap-2">
            <Button
              type="button"
              variant="default"
              disabled={isInstalling}
              onClick={() => void onConfirmDeleteAndInstall()}
              className="gap-1.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white shadow-xs"
            >
              {isInstalling ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  Updating...
                </>
              ) : (
                <>
                  <Trash2 className="h-3.5 w-3.5" />
                  Delete Old Jar &amp; Install
                </>
              )}
            </Button>

            <Button
              type="button"
              variant="outline"
              disabled={isInstalling}
              onClick={() => void onConfirmKeepAndInstall()}
              className="gap-1.5 text-xs"
              title="Install new jar without deleting the old version"
            >
              <Download className="h-3.5 w-3.5 text-muted-foreground" />
              Keep Both &amp; Install
            </Button>
          </div>

          <div className="flex items-center gap-1">
            {onDeleteOldOnly && (
              <Button
                type="button"
                variant="ghost"
                disabled={isInstalling}
                onClick={() => void onDeleteOldOnly()}
                className="text-xs text-destructive hover:bg-destructive/10 hover:text-destructive gap-1 px-2.5"
                title="Delete installed jar without installing new version"
              >
                <Trash2 className="h-3.5 w-3.5" />
                Delete Old Only
              </Button>
            )}

            <Button
              type="button"
              variant="ghost"
              disabled={isInstalling}
              onClick={onCancel}
              className="text-xs text-muted-foreground"
            >
              Cancel
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
