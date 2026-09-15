import { useId, useState } from 'react';
import {
  Check,
  CheckCircle2,
  Download,
  Info,
  Layers,
  Loader2,
  Package,
  ShieldAlert,
  Sparkles,
  X,
} from 'lucide-react';
import type { ResolvedDependency } from '../api/mod-dependencies';
import { formatFileSize } from '../api/modrinth';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { ProgressBar } from './ui/progress';

export interface ModDependenciesDialogProps {
  isOpen: boolean;
  primaryMod: {
    title: string;
    versionNumber?: string;
    filename?: string;
    iconUrl?: string | null;
  };
  dependencies: ResolvedDependency[];
  isInstalling?: boolean;
  installProgress?: {
    current: number;
    total: number;
    currentName: string;
  } | null;
  onConfirmInstall: (selectedDeps: ResolvedDependency[]) => void;
  onSkipAndInstallPrimaryOnly: () => void;
  onCancel: () => void;
}

export function ModDependenciesDialog({
  isOpen,
  primaryMod,
  dependencies,
  isInstalling = false,
  installProgress,
  onConfirmInstall,
  onSkipAndInstallPrimaryOnly,
  onCancel,
}: ModDependenciesDialogProps) {
  const missingDeps = dependencies.filter((d) => !d.alreadyInstalled);
  const satisfiedDeps = dependencies.filter((d) => d.alreadyInstalled);

  // By default, select all required missing dependencies + libraries
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    for (const d of missingDeps) {
      if (d.dependencyType === 'required' || d.isLibrary) {
        initial.add(d.id);
      }
    }
    // If no required deps, select all missing by default
    if (initial.size === 0) {
      for (const d of missingDeps) {
        initial.add(d.id);
      }
    }
    return initial;
  });

  const [showSatisfied, setShowSatisfied] = useState(false);

  const titleId = useId();
  const descId = useId();

  if (!isOpen) return null;

  const toggleDep = (id: string) => {
    if (isInstalling) return;
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const selectAll = () => {
    if (isInstalling) return;
    setSelectedIds(new Set(missingDeps.map((d) => d.id)));
  };

  const deselectAll = () => {
    if (isInstalling) return;
    setSelectedIds(new Set());
  };

  const selectedDependencies = missingDeps.filter((d) => selectedIds.has(d.id));
  const totalToInstall = 1 + selectedDependencies.length; // primary mod + chosen dependencies
  const requiredCount = missingDeps.filter((d) => d.dependencyType === 'required').length;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/65 p-4 backdrop-blur-xs"
      role="presentation"
      onMouseDown={(e) => {
        if (!isInstalling && e.target === e.currentTarget) {
          onCancel();
        }
      }}
    >
      <div
        className="relative flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-border/80 bg-card text-card-foreground shadow-2xl animate-in fade-in zoom-in-95"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descId}
      >
        {/* Header */}
        <div className="flex items-start justify-between border-b border-border/70 p-5 bg-gradient-to-r from-primary/10 via-background to-secondary/30">
          <div className="flex items-start gap-3.5">
            <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-primary/20 bg-primary/15 text-primary shadow-xs">
              <Layers className="h-5 w-5" />
            </div>
            <div>
              <div className="flex items-center gap-2 flex-wrap">
                <h3 id={titleId} className="text-base font-bold text-foreground">
                  Mod Dependencies Detected
                </h3>
                <Badge
                  variant="outline"
                  className="border-primary/30 bg-primary/10 text-primary text-[10px] uppercase font-mono"
                >
                  Auto-Resolved
                </Badge>
              </div>
              <p id={descId} className="mt-1 text-xs text-muted-foreground leading-relaxed">
                Installing <span className="font-semibold text-foreground">{primaryMod.title}</span> requires additional library mods to function properly on your server.
              </p>
            </div>
          </div>

          {!isInstalling && (
            <button
              type="button"
              aria-label="Close dialog"
              onClick={onCancel}
              className="rounded-lg p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors"
            >
              <X className="h-5 w-5" />
            </button>
          )}
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-5 space-y-4">
          {/* Installation Progress Overlay */}
          {isInstalling ? (
            <div className="py-8 text-center space-y-4">
              <div className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl border border-primary/20 bg-primary/10 text-primary shadow-inner">
                <Loader2 className="h-7 w-7 animate-spin text-primary" />
              </div>
              <div>
                <h4 className="text-sm font-semibold text-foreground">
                  {installProgress
                    ? `Installing Mods (${installProgress.current} of ${installProgress.total})...`
                    : 'Installing selected mods...'}
                </h4>
                {installProgress && (
                  <p className="mt-1 text-xs text-muted-foreground font-mono truncate max-w-md mx-auto">
                    Downloading: {installProgress.currentName}
                  </p>
                )}
              </div>
              {installProgress && (
                <div className="max-w-md mx-auto pt-2">
                  <ProgressBar
                    progress={Math.round((installProgress.current / installProgress.total) * 100)}
                    variant="primary"
                    size="sm"
                  />
                </div>
              )}
            </div>
          ) : (
            <>
              {/* Primary Mod Summary Card */}
              <div className="flex items-center justify-between gap-3 rounded-xl border border-border/70 bg-secondary/30 p-3 text-xs">
                <div className="flex items-center gap-3 min-w-0">
                  <div className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-background/80 shadow-2xs">
                    {primaryMod.iconUrl ? (
                      <img
                        src={primaryMod.iconUrl}
                        alt={primaryMod.title}
                        className="h-full w-full object-cover"
                      />
                    ) : (
                      <Package className="h-4 w-4 text-muted-foreground" />
                    )}
                  </div>
                  <div className="min-w-0">
                    <span className="font-semibold text-foreground truncate block">
                      {primaryMod.title}
                    </span>
                    <span className="text-[11px] text-muted-foreground font-mono truncate block">
                      {primaryMod.filename || primaryMod.versionNumber || 'Primary Mod'}
                    </span>
                  </div>
                </div>
                <Badge variant="default" className="shrink-0 text-[10px] px-2 py-0.5 font-medium">
                  Primary Mod
                </Badge>
              </div>

              {/* Missing Dependencies Checklist */}
              <div className="space-y-2">
                <div className="flex items-center justify-between text-xs">
                  <div className="flex items-center gap-2">
                    <span className="font-semibold uppercase tracking-wider text-muted-foreground text-[11px]">
                      Required & Recommended ({missingDeps.length})
                    </span>
                    {requiredCount > 0 && (
                      <span className="text-[10px] text-amber-600 dark:text-amber-400 font-medium flex items-center gap-1">
                        <ShieldAlert className="h-3 w-3" />
                        {requiredCount} required
                      </span>
                    )}
                  </div>

                  <div className="flex items-center gap-2 text-[11px]">
                    <button
                      type="button"
                      onClick={selectAll}
                      className="text-primary hover:underline font-medium"
                    >
                      Select all
                    </button>
                    <span className="text-muted-foreground">·</span>
                    <button
                      type="button"
                      onClick={deselectAll}
                      className="text-muted-foreground hover:text-foreground font-medium"
                    >
                      Deselect all
                    </button>
                  </div>
                </div>

                <div className="space-y-2">
                  {missingDeps.map((dep) => {
                    const isSelected = selectedIds.has(dep.id);

                    return (
                      <div
                        key={dep.id}
                        role="checkbox"
                        aria-checked={isSelected}
                        tabIndex={0}
                        onClick={() => toggleDep(dep.id)}
                        onKeyDown={(e) => {
                          if (e.key === ' ' || e.key === 'Enter') {
                            e.preventDefault();
                            toggleDep(dep.id);
                          }
                        }}
                        className={`group flex items-start gap-3 rounded-xl border p-3 text-xs transition-all cursor-pointer ${
                          isSelected
                            ? 'border-primary/50 bg-primary/5 shadow-2xs'
                            : 'border-border/60 bg-background/50 hover:border-border hover:bg-secondary/20'
                        }`}
                      >
                        <div className="pt-0.5 shrink-0">
                          <input
                            type="checkbox"
                            checked={isSelected}
                            onClick={(e) => e.stopPropagation()}
                            onChange={() => toggleDep(dep.id)}
                            aria-label={`Install ${dep.title}`}
                            className="h-4 w-4 rounded border-input text-primary focus:ring-primary cursor-pointer"
                          />
                        </div>

                        <div className="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-lg border bg-secondary/40 shadow-2xs">
                          {dep.iconUrl ? (
                            <img
                              src={dep.iconUrl}
                              alt={dep.title}
                              className="h-full w-full object-cover"
                            />
                          ) : (
                            <Package className="h-4 w-4 text-muted-foreground" />
                          )}
                        </div>

                        <div className="flex-1 min-w-0 space-y-1">
                          <div className="flex items-center gap-2 flex-wrap">
                            <span className="font-semibold text-foreground truncate">
                              {dep.title}
                            </span>
                            {dep.dependencyType === 'required' ? (
                              <Badge
                                variant="outline"
                                className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400 text-[10px] px-1.5 py-0"
                              >
                                Required
                              </Badge>
                            ) : (
                              <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                                Optional
                              </Badge>
                            )}
                            {dep.isLibrary && (
                              <Badge
                                variant="outline"
                                className="border-purple-500/30 bg-purple-500/10 text-purple-700 dark:text-purple-400 text-[10px] px-1.5 py-0"
                              >
                                Library
                              </Badge>
                            )}
                          </div>

                          {dep.description && (
                            <p className="text-muted-foreground text-[11px] line-clamp-1">
                              {dep.description}
                            </p>
                          )}

                          <div className="flex items-center gap-2 text-[10px] font-mono text-muted-foreground">
                            {dep.filename && <span className="truncate">{dep.filename}</span>}
                            {dep.fileSize !== undefined && (
                              <span>· {formatFileSize(dep.fileSize)}</span>
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* Already Satisfied Dependencies */}
              {satisfiedDeps.length > 0 && (
                <div className="pt-2 border-t border-border/60">
                  <button
                    type="button"
                    onClick={() => setShowSatisfied(!showSatisfied)}
                    className="flex w-full items-center justify-between text-xs font-semibold text-muted-foreground hover:text-foreground transition-colors"
                  >
                    <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
                      <CheckCircle2 className="h-3.5 w-3.5" />
                      Already Satisfied ({satisfiedDeps.length})
                    </span>
                    <span className="text-[11px] text-muted-foreground">
                      {showSatisfied ? 'Hide' : 'Show details'}
                    </span>
                  </button>

                  {showSatisfied && (
                    <div className="mt-2 space-y-1.5 animate-in fade-in">
                      {satisfiedDeps.map((dep) => (
                        <div
                          key={dep.id}
                          className="flex items-center justify-between gap-2 rounded-lg border border-emerald-500/20 bg-emerald-500/5 px-3 py-2 text-xs"
                        >
                          <div className="flex items-center gap-2 min-w-0">
                            <Check className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400 shrink-0" />
                            <span className="font-medium text-foreground truncate">
                              {dep.title}
                            </span>
                            {dep.installedFile && (
                              <span className="font-mono text-muted-foreground truncate text-[11px]">
                                ({dep.installedFile})
                              </span>
                            )}
                          </div>
                          <Badge
                            variant="outline"
                            className="border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[10px] px-1.5 py-0 shrink-0"
                          >
                            Installed
                          </Badge>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </>
          )}
        </div>

        {/* Footer Actions */}
        <div className="flex flex-col sm:flex-row items-center justify-between gap-2.5 border-t border-border/70 bg-secondary/15 px-5 py-3.5">
          <div className="flex items-center gap-2 text-xs text-muted-foreground w-full sm:w-auto">
            <Info className="h-3.5 w-3.5 text-primary shrink-0" />
            <span>
              {selectedDependencies.length === 0
                ? 'Only primary mod selected'
                : `${totalToInstall} mods will be downloaded to server`}
            </span>
          </div>

          <div className="flex items-center gap-2 w-full sm:w-auto justify-end">
            <Button
              variant="outline"
              size="sm"
              disabled={isInstalling}
              onClick={onCancel}
              className="text-xs"
            >
              Cancel
            </Button>

            {missingDeps.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                disabled={isInstalling}
                onClick={onSkipAndInstallPrimaryOnly}
                className="text-xs text-muted-foreground hover:text-foreground"
                title="Install only the main mod without any dependencies"
              >
                Skip & Install Only {primaryMod.title}
              </Button>
            )}

            <Button
              size="sm"
              disabled={isInstalling}
              onClick={() => onConfirmInstall(selectedDependencies)}
              className="gap-1.5 text-xs font-semibold shadow-xs"
            >
              <Download className="h-3.5 w-3.5" />
              {selectedDependencies.length > 0
                ? `Install ${totalToInstall} Mods`
                : `Install ${primaryMod.title}`}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
