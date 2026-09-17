import { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import {
  Sparkles,
  CheckCircle2,
  AlertTriangle,
  AlertCircle,
  ArrowRight,
  DownloadCloud,
  ShieldCheck,
  Copy,
  Check,
  ExternalLink,
  RefreshCw,
  Search,
  Server as ServerIcon,
  Loader2,
  RotateCw,
} from 'lucide-react';
import { api, ApiError } from '../api/client';
import type {
  Server,
  Mod,
  VersionMeta,
  VersionUpdateReport,
  ModTargetCompatibility,
  ModUpdateInfo,
} from '../api/types';
import { Button } from './ui/button';
import { Badge } from './ui/badge';
import { Select } from './ui/select';
import { Input } from './ui/input';
import { ProgressBar } from './ui/progress';
import { useModal } from './ui/modal';
import { ModJarPickerDialog } from './mod-jar-picker-dialog';

interface VersionUpdateHelperProps {
  server: Server;
  installedMods: Mod[];
  onRefreshInstalled?: () => void;
  onNavigateToSettings?: () => void;
}

export function VersionUpdateHelper({
  server,
  installedMods,
  onRefreshInstalled,
  onNavigateToSettings,
}: VersionUpdateHelperProps) {
  const { confirm, dialog } = useModal();

  // Versions and Target
  const [availableVersions, setAvailableVersions] = useState<VersionMeta[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [targetVersion, setTargetVersion] = useState<string>('');
  const [customVersion, setCustomVersion] = useState('');
  const [isCustom, setIsCustom] = useState(false);

  // Scan & Report state
  const [scanning, setScanning] = useState(false);
  const [scanProgress, setScanProgress] = useState(0);
  const [scanStage, setScanStage] = useState('');
  const [report, setReport] = useState<VersionUpdateReport | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lastScannedVersion, setLastScannedVersion] = useState<string | null>(null);
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  // Clear timers on unmount
  useEffect(() => {
    return () => {
      timersRef.current.forEach((t) => clearTimeout(t));
    };
  }, []);

  // Filters & search
  const [filterStatus, setFilterStatus] = useState<
    'all' | 'update_available' | 'already_compatible' | 'not_available' | 'untracked'
  >('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Mod Jar Picker dialog for individual manual selection
  const [pickerMod, setPickerMod] = useState<{ mod: Mod; updateInfo?: ModUpdateInfo } | null>(null);

  // Actions state
  const [updatingBatch, setUpdatingBatch] = useState(false);
  const [batchProgress, setBatchProgress] = useState<{
    current: number;
    total: number;
    message: string;
    percent: number;
  } | null>(null);
  const [creatingBackup, setCreatingBackup] = useState(false);
  const [backupSuccess, setBackupSuccess] = useState<string | null>(null);
  const [copiedReport, setCopiedReport] = useState(false);
  const [upgradingServerJar, setUpgradingServerJar] = useState(false);
  const [serverUpgradeSuccess, setServerUpgradeSuccess] = useState<string | null>(null);

  // Load available server jar versions on mount
  useEffect(() => {
    let cancelled = false;
    setLoadingVersions(true);
    api
      .jarVersions(server.server_type)
      .then((versions) => {
        if (cancelled) return;
        setAvailableVersions(versions);
        // Default target to the latest version or first version different from current
        const current = server.version?.trim();
        const newer = versions.find((v) => v.name !== current);
        if (newer) {
          setTargetVersion(newer.name);
        } else if (versions[0]) {
          setTargetVersion(versions[0].name);
        }
      })
      .catch(() => {
        // Jar versions fetch failed; fallback to current version
        if (server.version) {
          setTargetVersion(server.version);
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingVersions(false);
      });

    return () => {
      cancelled = true;
    };
  }, [server.server_type, server.version]);

  const effectiveTargetVersion = isCustom ? customVersion.trim() : targetVersion.trim();

  // Scan compatibility for effective target version
  const scanCompatibility = useCallback(
    async (force = false) => {
      const v = effectiveTargetVersion;
      if (!v) return;

      timersRef.current.forEach((t) => clearTimeout(t));
      timersRef.current = [];

      setScanning(true);
      setScanProgress(15);
      setScanStage(`Connecting to mod repositories for Minecraft ${v}...`);
      setError(null);
      setBackupSuccess(null);
      setServerUpgradeSuccess(null);

      const t1 = setTimeout(() => {
        setScanProgress(45);
        setScanStage('Analyzing file hashes & target version constraints...');
      }, 300);

      const t2 = setTimeout(() => {
        setScanProgress(75);
        setScanStage('Checking Modrinth, CurseForge & catalog indexes...');
      }, 700);

      const t3 = setTimeout(() => {
        setScanProgress(90);
        setScanStage('Compiling compatibility report...');
      }, 1100);

      timersRef.current = [t1, t2, t3];

      try {
        const res = await api.checkVersionUpgradeCompatibility(server.id, v, force);
        timersRef.current.forEach((t) => clearTimeout(t));
        timersRef.current = [];
        setScanProgress(100);
        setScanStage('Compatibility check complete!');
        await new Promise((resolve) => setTimeout(resolve, 150));
        setReport(res);
        setLastScannedVersion(v);
      } catch (err) {
        timersRef.current.forEach((t) => clearTimeout(t));
        timersRef.current = [];
        setError(err instanceof ApiError ? err.detail : 'Failed to check version compatibility.');
      } finally {
        setScanning(false);
      }
    },
    [effectiveTargetVersion, server.id],
  );

  // Create pre-upgrade server backup
  async function handleCreateBackup() {
    setCreatingBackup(true);
    setError(null);
    setBackupSuccess(null);
    try {
      const backupName = `Pre-Upgrade-${effectiveTargetVersion || 'update'}-${new Date().toISOString().slice(0, 10)}`;
      await api.createBackup(server.id, backupName);
      setBackupSuccess(`Backup "${backupName}" created successfully!`);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to create backup.');
    } finally {
      setCreatingBackup(false);
    }
  }

  // Batch update all compatible mods
  async function handleBatchUpdate() {
    if (!report || report.update_count === 0) return;

    const confirmed = await confirm(
      `Download and apply target version jars for all ${report.update_count} compatible mods for Minecraft ${effectiveTargetVersion}? Old jars will be replaced with compatible builds.`,
      {
        title: `Update ${report.update_count} mods for MC ${effectiveTargetVersion}`,
        confirmLabel: `Update ${report.update_count} Mods`,
      },
    );
    if (!confirmed) return;

    setUpdatingBatch(true);
    setError(null);
    setBatchProgress({
      current: 0,
      total: report.update_count,
      message: 'Preparing mod downloads...',
      percent: 0,
    });

    try {
      const res = await api.applyVersionUpgradeMods(server.id, effectiveTargetVersion);
      onRefreshInstalled?.();
      // Rescan after update to reflect fresh state
      await scanCompatibility(true);
      if (res.failed > 0) {
        setError(
          `Updated ${res.updated} mods, but ${res.failed} failed. Check individual mods below.`,
        );
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Batch update failed.');
    } finally {
      setUpdatingBatch(false);
      setBatchProgress(null);
    }
  }

  // Update a single mod
  async function handleUpdateSingleMod(item: ModTargetCompatibility) {
    if (!item.download_url || !item.compatible_jar) return;

    const confirmed = await confirm(
      `Upgrade ${item.title || item.mod_name} to ${item.compatible_version} (${item.compatible_jar})? The previous file (${item.mod_file}) will be removed.`,
      {
        title: `Update ${item.title || item.mod_name}`,
        confirmLabel: 'Update',
      },
    );
    if (!confirmed) return;

    setError(null);
    try {
      await api.updateModVersion(
        server.id,
        item.mod_name,
        item.download_url,
        item.compatible_jar,
        {
          deleteOld: true,
          projectId: item.project_id,
          projectSlug: item.project_slug,
          provider: item.provider,
        },
      );
      onRefreshInstalled?.();
      await scanCompatibility(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : `Failed to update ${item.title}.`);
    }
  }

  // Upgrade Server Jar directly to target version
  async function handleUpgradeServerJar() {
    const v = effectiveTargetVersion;
    if (!v) return;

    const confirmed = await confirm(
      `Upgrade this ${server.server_type} server to Minecraft ${v}? This will download the latest server jar build for ${v} and configure the container.`,
      {
        title: `Upgrade server software to MC ${v}`,
        confirmLabel: `Upgrade Server to ${v}`,
        destructive: true,
      },
    );
    if (!confirmed) return;

    setUpgradingServerJar(true);
    setError(null);
    setServerUpgradeSuccess(null);

    try {
      // Find latest build for target version
      const builds = await api.jarBuilds(server.server_type, v);
      const latestBuild = builds[0]?.build ?? '';
      await api.installServer(server.id, { version: v, build: latestBuild });
      setServerUpgradeSuccess(
        `Server jar upgraded to ${server.server_type} ${v}${latestBuild ? ` (build ${latestBuild})` : ''}! Restart the server to run the new version.`,
      );
      onRefreshInstalled?.();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to upgrade server jar.');
    } finally {
      setUpgradingServerJar(false);
    }
  }

  // Copy Markdown compatibility report to clipboard
  function handleCopyReport() {
    if (!report) return;

    const v = effectiveTargetVersion;
    const dateStr = new Date(report.last_checked).toLocaleDateString();

    const readyList = Object.values(report.mods).filter(
      (m) => m.status === 'already_compatible' || m.status === 'update_available',
    );
    const missingList = Object.values(report.mods).filter((m) => m.status === 'not_available');
    const untrackedList = Object.values(report.mods).filter((m) => m.status === 'untracked');

    const lines = [
      `# Minecraft ${v} Upgrade Report — ${server.name}`,
      `*Generated on ${dateStr} via Minecraft Container Manager*`,
      '',
      `**Overall Readiness**: ${report.compatible_count}/${report.total_mods} mods ready (${Math.round(
        (report.compatible_count / (report.total_mods || 1)) * 100,
      )}%)`,
      `**Current Server**: ${server.server_type} ${server.version || 'Unknown'}`,
      `**Target Version**: Minecraft ${v}`,
      '',
      `### 🟢 Ready for ${v} (${readyList.length})`,
      ...readyList.map(
        (m) =>
          `- **${m.title || m.mod_name}**: ${
            m.status === 'update_available'
              ? `Update available -> \`${m.compatible_version}\` (\`${m.compatible_jar}\`)`
              : `Already compatible (\`${m.current_version || m.mod_file}\`)`
          }`,
      ),
      '',
    ];

    if (missingList.length > 0) {
      lines.push(
        `### 🔴 Incompatible / Missing for ${v} (${missingList.length})`,
        ...missingList.map(
          (m) =>
            `- **${m.title || m.mod_name}** (\`${m.mod_file}\`): No ${v} release published yet by author`,
        ),
        '',
      );
    }

    if (untrackedList.length > 0) {
      lines.push(
        `### ⚪ Untracked / Custom Mods (${untrackedList.length})`,
        ...untrackedList.map((m) => `- **${m.title || m.mod_name}** (\`${m.mod_file}\`)`),
        '',
      );
    }

    void navigator.clipboard.writeText(lines.join('\n'));
    setCopiedReport(true);
    setTimeout(() => setCopiedReport(false), 3000);
  }

  // Filter and search mods
  const filteredMods = useMemo(() => {
    if (!report?.mods) return [];
    let list = Object.values(report.mods);

    if (filterStatus !== 'all') {
      list = list.filter((m) => m.status === filterStatus);
    }

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      list = list.filter(
        (m) =>
          m.mod_name.toLowerCase().includes(q) ||
          m.title.toLowerCase().includes(q) ||
          m.mod_file.toLowerCase().includes(q) ||
          (m.compatible_jar && m.compatible_jar.toLowerCase().includes(q)),
      );
    }

    // Sort order: update_available, not_available, untracked, already_compatible
    const rank: Record<string, number> = {
      update_available: 1,
      not_available: 2,
      untracked: 3,
      already_compatible: 4,
    };
    return list.sort((a, b) => {
      const rA = rank[a.status] || 99;
      const rB = rank[b.status] || 99;
      if (rA !== rB) return rA - rB;
      return (a.title || a.mod_name).localeCompare(b.title || b.mod_name);
    });
  }, [report, filterStatus, searchQuery]);

  const readinessPercent = useMemo(() => {
    if (!report || report.total_mods === 0) return 100;
    return Math.round((report.compatible_count / report.total_mods) * 100);
  }, [report]);

  return (
    <>
      {dialog}

      <div className="space-y-6">
        {/* Helper Banner / Controls Card */}
        <div className="rounded-2xl border border-primary/20 bg-gradient-to-br from-card via-card to-primary/5 p-5 shadow-sm transition-all sm:p-6">
          <div className="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
            <div className="space-y-1.5 min-w-0 flex-1">
              <div className="flex items-center gap-2.5 flex-wrap">
                <span className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary shadow-2xs">
                  <Sparkles className="h-4 w-4" />
                </span>
                <h2 className="text-xl font-bold tracking-tight text-foreground">
                  Version Update Helper
                </h2>
              </div>
              <p className="text-sm text-muted-foreground max-w-2xl">
                Check whether your installed mods &amp; plugins have compatible releases before
                updating your server to a newer Minecraft version. Avoid crashes and plan safe
                upgrades with one click.
              </p>
            </div>

            {/* Target Version Selector Pill */}
            <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 bg-secondary/40 p-2.5 rounded-xl border border-border/70 shrink-0">
              {/* Current Version */}
              <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-background/80 border border-border/50 text-xs shadow-2xs shrink-0">
                <span className="text-muted-foreground font-medium">Current:</span>
                <Badge variant="outline" className="font-mono text-xs px-1.5 py-0">
                  {server.server_type} {server.version || 'unknown'}
                </Badge>
              </div>

              <ArrowRight className="hidden sm:block h-4 w-4 text-muted-foreground shrink-0" />

              {/* Target Version Dropdown */}
              <div className="flex items-center gap-2 min-w-[200px]">
                {isCustom ? (
                  <Input
                    placeholder="e.g. 1.21.1"
                    value={customVersion}
                    onChange={(e) => setCustomVersion(e.target.value)}
                    className="h-9 text-xs font-mono"
                  />
                ) : (
                  <Select
                    id="target-version-select"
                    value={targetVersion}
                    onChange={(e) => {
                      if (e.target.value === '__custom__') {
                        setIsCustom(true);
                      } else {
                        setTargetVersion(e.target.value);
                      }
                    }}
                    disabled={loadingVersions || availableVersions.length === 0}
                    className="h-9 text-xs font-mono"
                  >
                    {availableVersions.map((v) => (
                      <option key={v.name} value={v.name}>
                        MC {v.name} {v.name === server.version ? '(Current)' : ''}
                      </option>
                    ))}
                    <option value="__custom__">Custom Version...</option>
                  </Select>
                )}

                {isCustom && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setIsCustom(false)}
                    className="h-9 px-2 text-xs"
                    title="Switch back to list"
                  >
                    Cancel
                  </Button>
                )}
              </div>

              {/* Scan / Refresh Button */}
              <Button
                size="sm"
                onClick={() => void scanCompatibility(true)}
                disabled={scanning || !effectiveTargetVersion}
                className="gap-1.5 text-xs font-semibold shadow-xs shrink-0"
              >
                {scanning ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCw className="h-3.5 w-3.5" />
                )}
                <span>
                  {scanning
                    ? 'Checking...'
                    : report && report.target_version === effectiveTargetVersion
                    ? 'Re-Check'
                    : 'Check Compatibility'}
                </span>
              </Button>
            </div>
          </div>
        </div>

        {/* Feedback alerts */}
        {error && (
          <div className="flex items-center gap-2.5 rounded-xl border border-destructive/40 bg-destructive/10 p-3.5 text-sm text-destructive animate-fadeIn">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        {backupSuccess && (
          <div className="flex items-center gap-2.5 rounded-xl border border-emerald-500/40 bg-emerald-500/10 p-3.5 text-sm text-emerald-800 dark:text-emerald-300 animate-fadeIn">
            <CheckCircle2 className="h-4 w-4 shrink-0" />
            <span>{backupSuccess}</span>
          </div>
        )}

        {serverUpgradeSuccess && (
          <div className="flex items-center gap-2.5 rounded-xl border border-emerald-500/40 bg-emerald-500/10 p-3.5 text-sm text-emerald-800 dark:text-emerald-300 animate-fadeIn">
            <CheckCircle2 className="h-4 w-4 shrink-0" />
            <span>{serverUpgradeSuccess}</span>
          </div>
        )}

        {/* In-progress batch update banner */}
        {updatingBatch && batchProgress && (
          <div className="rounded-xl border border-primary/30 bg-primary/5 p-4 space-y-2 animate-fadeIn">
            <ProgressBar
              label={
                <span className="flex items-center gap-2 font-semibold text-foreground text-sm">
                  <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
                  <span>{batchProgress.message}</span>
                </span>
              }
              subtext={`Updating compatible mod jars (${batchProgress.current}/${batchProgress.total})`}
              value={batchProgress.percent}
              size="md"
            />
          </div>
        )}

        {/* Scanning in-progress progress bar card */}
        {scanning && (
          <div className="rounded-2xl border border-primary/30 bg-gradient-to-br from-card via-card to-primary/5 p-5 sm:p-6 space-y-4 shadow-sm animate-fadeIn">
            <div className="flex items-center justify-between gap-4">
              <div className="flex items-center gap-2.5 min-w-0">
                <span className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary/10 text-primary shrink-0">
                  <Loader2 className="h-4 w-4 animate-spin" />
                </span>
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold text-foreground truncate">
                    Checking Compatibility for Minecraft {effectiveTargetVersion}
                  </h3>
                  <p className="text-xs text-muted-foreground truncate">
                    Scanning {installedMods.length} installed {server.server_type === 'paper' ? 'plugins' : 'mods'} against repository indexes
                  </p>
                </div>
              </div>
              <Badge variant="outline" className="font-mono text-xs px-2 py-0.5 border-primary/40 text-primary shrink-0">
                {scanProgress}%
              </Badge>
            </div>

            <ProgressBar
              value={scanProgress}
              max={100}
              label={
                <span className="text-xs font-medium text-muted-foreground truncate">
                  {scanStage || 'Scanning repositories...'}
                </span>
              }
              showPercent={true}
              variant="default"
              size="md"
              animated
            />

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-2 pt-1 text-xs text-muted-foreground border-t border-border/50">
              <div className="flex items-center gap-1.5 truncate">
                <span className="font-medium text-foreground">Current:</span>
                <span className="font-mono">{server.server_type} {server.version || 'unknown'}</span>
              </div>
              <div className="flex items-center gap-1.5 truncate">
                <span className="font-medium text-foreground">Target:</span>
                <span className="font-mono text-primary font-semibold">Minecraft {effectiveTargetVersion}</span>
              </div>
              <div className="flex items-center gap-1.5 truncate">
                <span className="font-medium text-foreground">Mods/Plugins:</span>
                <span>{installedMods.length} installed</span>
              </div>
            </div>
          </div>
        )}

        {/* Target version changed notice if report exists for a different version */}
        {!scanning && report && report.target_version !== effectiveTargetVersion && (
          <div className="flex items-center justify-between gap-3 rounded-xl border border-amber-500/40 bg-amber-500/10 p-3.5 text-xs text-amber-900 dark:text-amber-200 animate-fadeIn">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
              <span>
                Target version changed to <strong>Minecraft {effectiveTargetVersion}</strong>. The results below are for <strong>Minecraft {report.target_version}</strong>.
              </span>
            </div>
            <Button
              size="sm"
              onClick={() => void scanCompatibility(true)}
              className="h-7 text-xs bg-amber-600 hover:bg-amber-700 text-white shrink-0"
            >
              Scan {effectiveTargetVersion}
            </Button>
          </div>
        )}

        {/* Readiness Dashboard & Overview */}
        {!scanning && report && (
          <div className="space-y-4 animate-fadeIn">
            {/* Verdict Card */}
            <div
              className={`rounded-xl border p-4.5 sm:p-5 flex flex-col md:flex-row items-start md:items-center justify-between gap-4 ${
                report.ready_to_update
                  ? 'border-emerald-500/40 bg-emerald-500/5 text-emerald-950 dark:text-emerald-100'
                  : 'border-amber-500/40 bg-amber-500/5 text-amber-950 dark:text-amber-100'
              }`}
            >
              <div className="flex items-start gap-3.5">
                <div
                  className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${
                    report.ready_to_update
                      ? 'bg-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                      : 'bg-amber-500/20 text-amber-600 dark:text-amber-400'
                  }`}
                >
                  {report.ready_to_update ? (
                    <CheckCircle2 className="h-5 w-5" />
                  ) : (
                    <AlertTriangle className="h-5 w-5" />
                  )}
                </div>
                <div className="space-y-1">
                  <h3 className="font-bold text-base tracking-tight">
                    {report.ready_to_update
                      ? `All mods are compatible with Minecraft ${effectiveTargetVersion}!`
                      : `${report.not_available_count} ${
                          report.not_available_count === 1 ? 'mod is' : 'mods are'
                        } not yet ready for Minecraft ${effectiveTargetVersion}`}
                  </h3>
                  <p className="text-xs text-muted-foreground leading-relaxed">
                    {report.ready_to_update
                      ? `Your server has ${report.total_mods} installed ${
                          server.server_type === 'paper' ? 'plugins' : 'mods'
                        }, and all of them are ready for MC ${effectiveTargetVersion}. You can safely update your mods and upgrade the server jar.`
                      : `Upgrading your server now may cause crashes or disable the ${report.not_available_count} missing mods. We recommend waiting for mod authors to release updates or disabling these mods before upgrading.`}
                  </p>
                </div>
              </div>

              {/* Quick Jump / Actions in Verdict */}
              <div className="flex items-center gap-2 shrink-0 self-stretch sm:self-auto flex-wrap">
                {report.update_count > 0 && (
                  <Button
                    size="sm"
                    onClick={() => void handleBatchUpdate()}
                    disabled={updatingBatch || scanning}
                    className="h-9 gap-2 text-xs font-semibold bg-emerald-600 hover:bg-emerald-700 text-white shadow-xs"
                  >
                    <DownloadCloud className="h-4 w-4" />
                    <span>Update All Compatible Mods ({report.update_count})</span>
                  </Button>
                )}

                {report.ready_to_update && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void handleUpgradeServerJar()}
                    disabled={upgradingServerJar || scanning}
                    className="h-9 gap-1.5 text-xs font-semibold border-primary/40 bg-primary/10 text-primary hover:bg-primary/20 shadow-xs"
                  >
                    <RotateCw className="h-3.5 w-3.5" />
                    <span>Upgrade Server Jar to {effectiveTargetVersion}</span>
                  </Button>
                )}
              </div>
            </div>

            {/* Metrics Grid */}
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              {/* Ready / Compatible */}
              <button
                type="button"
                onClick={() =>
                  setFilterStatus((prev) =>
                    prev === 'already_compatible' ? 'all' : 'already_compatible',
                  )
                }
                className={`flex flex-col items-start p-4 rounded-xl border text-left transition-all ${
                  filterStatus === 'already_compatible'
                    ? 'border-emerald-500 bg-emerald-500/10 shadow-xs'
                    : 'border-border/70 bg-card hover:border-border hover:bg-secondary/30'
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <span className="text-xs font-medium text-muted-foreground">
                    Already Compatible
                  </span>
                  <span className="h-2 w-2 rounded-full bg-emerald-500" />
                </div>
                <div className="mt-2 text-2xl font-bold tracking-tight text-foreground">
                  {report.compatible_count - report.update_count}
                </div>
                <span className="mt-1 text-[11px] text-muted-foreground">
                  Works on {effectiveTargetVersion} as-is
                </span>
              </button>

              {/* Updates Available */}
              <button
                type="button"
                onClick={() =>
                  setFilterStatus((prev) =>
                    prev === 'update_available' ? 'all' : 'update_available',
                  )
                }
                className={`flex flex-col items-start p-4 rounded-xl border text-left transition-all ${
                  filterStatus === 'update_available'
                    ? 'border-amber-500 bg-amber-500/10 shadow-xs'
                    : 'border-border/70 bg-card hover:border-border hover:bg-secondary/30'
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <span className="text-xs font-medium text-muted-foreground">
                    Updates Available
                  </span>
                  <span className="h-2 w-2 rounded-full bg-amber-500 animate-pulse" />
                </div>
                <div className="mt-2 text-2xl font-bold tracking-tight text-amber-600 dark:text-amber-400">
                  {report.update_count}
                </div>
                <span className="mt-1 text-[11px] text-muted-foreground">
                  New jars ready to install
                </span>
              </button>

              {/* Incompatible / Missing */}
              <button
                type="button"
                onClick={() =>
                  setFilterStatus((prev) =>
                    prev === 'not_available' ? 'all' : 'not_available',
                  )
                }
                className={`flex flex-col items-start p-4 rounded-xl border text-left transition-all ${
                  filterStatus === 'not_available'
                    ? 'border-destructive bg-destructive/10 shadow-xs'
                    : 'border-border/70 bg-card hover:border-border hover:bg-secondary/30'
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <span className="text-xs font-medium text-muted-foreground">
                    No Build Found
                  </span>
                  <span className="h-2 w-2 rounded-full bg-destructive" />
                </div>
                <div className="mt-2 text-2xl font-bold tracking-tight text-destructive">
                  {report.not_available_count}
                </div>
                <span className="mt-1 text-[11px] text-muted-foreground">
                  Blocking upgrade to {effectiveTargetVersion}
                </span>
              </button>

              {/* Untracked */}
              <button
                type="button"
                onClick={() =>
                  setFilterStatus((prev) =>
                    prev === 'untracked' ? 'all' : 'untracked',
                  )
                }
                className={`flex flex-col items-start p-4 rounded-xl border text-left transition-all ${
                  filterStatus === 'untracked'
                    ? 'border-slate-500 bg-slate-500/10 shadow-xs'
                    : 'border-border/70 bg-card hover:border-border hover:bg-secondary/30'
                }`}
              >
                <div className="flex items-center justify-between w-full">
                  <span className="text-xs font-medium text-muted-foreground">
                    Untracked / Custom
                  </span>
                  <span className="h-2 w-2 rounded-full bg-muted-foreground" />
                </div>
                <div className="mt-2 text-2xl font-bold tracking-tight text-foreground">
                  {report.untracked_count}
                </div>
                <span className="mt-1 text-[11px] text-muted-foreground">
                  Private or custom builds
                </span>
              </button>
            </div>

            {/* Readiness Progress Bar */}
            <div className="rounded-xl border border-border/80 bg-card p-4 space-y-2">
              <div className="flex items-center justify-between text-xs">
                <span className="font-semibold text-foreground">
                  Version Upgrade Readiness Score
                </span>
                <span className="font-mono font-bold text-primary">
                  {readinessPercent}% ({report.compatible_count}/{report.total_mods} mods ready)
                </span>
              </div>
              <div className="h-2.5 w-full overflow-hidden rounded-full bg-secondary">
                <div
                  className={`h-full transition-all duration-500 ${
                    readinessPercent === 100
                      ? 'bg-emerald-500'
                      : readinessPercent >= 75
                      ? 'bg-amber-500'
                      : 'bg-primary'
                  }`}
                  style={{ width: `${readinessPercent}%` }}
                />
              </div>
            </div>

            {/* Action Bar / Utility Toolbar */}
            <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pt-2">
              {/* Filter tabs */}
              <div className="flex items-center gap-1.5 overflow-x-auto pb-1 sm:pb-0">
                <button
                  type="button"
                  onClick={() => setFilterStatus('all')}
                  className={`px-3 py-1.5 text-xs font-semibold rounded-lg transition-all ${
                    filterStatus === 'all'
                      ? 'bg-primary text-primary-foreground shadow-2xs'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                  }`}
                >
                  All ({report.total_mods})
                </button>
                <button
                  type="button"
                  onClick={() => setFilterStatus('update_available')}
                  className={`px-3 py-1.5 text-xs font-semibold rounded-lg transition-all ${
                    filterStatus === 'update_available'
                      ? 'bg-amber-600 text-white shadow-2xs'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                  }`}
                >
                  Updates ({report.update_count})
                </button>
                <button
                  type="button"
                  onClick={() => setFilterStatus('not_available')}
                  className={`px-3 py-1.5 text-xs font-semibold rounded-lg transition-all ${
                    filterStatus === 'not_available'
                      ? 'bg-destructive text-destructive-foreground shadow-2xs'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                  }`}
                >
                  Missing ({report.not_available_count})
                </button>
                <button
                  type="button"
                  onClick={() => setFilterStatus('already_compatible')}
                  className={`px-3 py-1.5 text-xs font-semibold rounded-lg transition-all ${
                    filterStatus === 'already_compatible'
                      ? 'bg-emerald-600 text-white shadow-2xs'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                  }`}
                >
                  Compatible ({report.compatible_count - report.update_count})
                </button>
              </div>

              {/* Utility actions */}
              <div className="flex items-center gap-2 flex-wrap">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void handleCreateBackup()}
                  disabled={creatingBackup}
                  className="h-8 gap-1.5 text-xs"
                  title="Create a safety backup before applying updates"
                >
                  {creatingBackup ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
                  )}
                  <span>{creatingBackup ? 'Backing up...' : 'Create Backup'}</span>
                </Button>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleCopyReport}
                  className="h-8 gap-1.5 text-xs"
                  title="Copy formatted markdown report to clipboard"
                >
                  {copiedReport ? (
                    <Check className="h-3.5 w-3.5 text-emerald-500" />
                  ) : (
                    <Copy className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                  <span>{copiedReport ? 'Copied!' : 'Copy Report'}</span>
                </Button>

                {onNavigateToSettings && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onNavigateToSettings}
                    className="h-8 gap-1 text-xs text-muted-foreground hover:text-foreground"
                  >
                    <ServerIcon className="h-3.5 w-3.5" />
                    <span>Server Settings</span>
                  </Button>
                )}
              </div>
            </div>

            {/* Search Input */}
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <Input
                placeholder="Filter mods by name..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9 h-8 text-xs max-w-sm"
              />
            </div>

            {/* Mod List */}
            <div className="space-y-2.5">
              {filteredMods.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border/80 p-8 text-center text-muted-foreground">
                  <p className="text-sm font-medium">No mods match the selected filter.</p>
                </div>
              ) : (
                filteredMods.map((item) => {
                  const originalMod = installedMods.find(
                    (m) => m.name === item.mod_name || m.file === item.mod_file,
                  );

                  return (
                    <div
                      key={item.mod_name}
                      className={`flex flex-col md:flex-row md:items-center md:justify-between gap-3.5 rounded-xl border p-4 shadow-2xs transition-all ${
                        item.status === 'update_available'
                          ? 'border-amber-500/40 bg-card hover:border-amber-500/70 hover:shadow-xs'
                          : item.status === 'not_available'
                          ? 'border-destructive/30 bg-card hover:border-destructive/60'
                          : item.status === 'already_compatible'
                          ? 'border-emerald-500/30 bg-card/60 hover:border-emerald-500/50'
                          : 'border-border/70 bg-card'
                      }`}
                    >
                      {/* Left: Info & Versions */}
                      <div className="min-w-0 space-y-2 flex-1">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-bold text-foreground text-sm">
                            {item.title || item.mod_name}
                          </span>

                          {/* Status Badge */}
                          {item.status === 'update_available' && (
                            <Badge
                              variant="secondary"
                              className="border border-amber-500/40 bg-amber-500/15 text-amber-700 dark:text-amber-300 text-[11px] px-2 py-0 font-semibold"
                            >
                              Update Ready
                            </Badge>
                          )}
                          {item.status === 'already_compatible' && (
                            <Badge
                              variant="secondary"
                              className="border border-emerald-500/40 bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 text-[11px] px-2 py-0 font-semibold"
                            >
                              Already Compatible
                            </Badge>
                          )}
                          {item.status === 'not_available' && (
                            <Badge
                              variant="secondary"
                              className="border border-destructive/40 bg-destructive/15 text-destructive text-[11px] px-2 py-0 font-semibold"
                            >
                              No {effectiveTargetVersion} Build
                            </Badge>
                          )}
                          {item.status === 'untracked' && (
                            <Badge
                              variant="outline"
                              className="text-[11px] px-2 py-0 text-muted-foreground"
                            >
                              Untracked / Private
                            </Badge>
                          )}

                          {item.provider && (
                            <Badge
                              variant="outline"
                              className="text-[10px] px-1.5 py-0 uppercase font-mono text-muted-foreground"
                            >
                              {item.provider}
                            </Badge>
                          )}

                          {item.release_type && (
                            <Badge
                              variant="secondary"
                              className="text-[10px] px-1.5 py-0 uppercase"
                            >
                              {item.release_type}
                            </Badge>
                          )}
                        </div>

                        {/* Version Comparison Bar */}
                        <div className="flex items-center gap-2 text-xs flex-wrap rounded-lg bg-secondary/30 px-3 py-1.5 border border-border/50">
                          <div className="flex items-center gap-1.5 min-w-0">
                            <span className="text-muted-foreground font-medium shrink-0">
                              Current:
                            </span>
                            <span
                              className="font-mono text-muted-foreground truncate max-w-[200px]"
                              title={item.mod_file}
                            >
                              {item.mod_file}
                            </span>
                            {item.current_version && (
                              <Badge variant="outline" className="text-[10px] px-1 py-0">
                                {item.current_version}
                              </Badge>
                            )}
                          </div>

                          <ArrowRight className="h-3.5 w-3.5 text-muted-foreground shrink-0" />

                          <div className="flex items-center gap-1.5 min-w-0">
                            <span className="text-muted-foreground font-medium shrink-0">
                              Target:
                            </span>
                            {item.status === 'update_available' ? (
                              <>
                                <span
                                  className="font-mono font-semibold text-emerald-600 dark:text-emerald-400 truncate max-w-[240px]"
                                  title={item.compatible_jar}
                                >
                                  {item.compatible_jar}
                                </span>
                                <Badge
                                  variant="outline"
                                  className="border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 text-[10px] px-1 py-0 shrink-0 font-mono"
                                >
                                  {item.compatible_version}
                                </Badge>
                              </>
                            ) : item.status === 'already_compatible' ? (
                              <span className="text-emerald-600 dark:text-emerald-400 font-medium">
                                Compatible with MC {effectiveTargetVersion}
                              </span>
                            ) : item.status === 'not_available' ? (
                              <span className="text-destructive font-medium">
                                No release for MC {effectiveTargetVersion}
                              </span>
                            ) : (
                              <span className="text-muted-foreground">
                                Manual verification needed
                              </span>
                            )}
                          </div>
                        </div>

                        {/* Release date or changelog snippet */}
                        {item.release_date && (
                          <div className="text-[11px] text-muted-foreground">
                            Published:{' '}
                            {new Date(item.release_date).toLocaleDateString(undefined, {
                              year: 'numeric',
                              month: 'short',
                              day: 'numeric',
                            })}
                          </div>
                        )}
                      </div>

                      {/* Right: Actions */}
                      <div className="flex items-center gap-2 shrink-0 self-end md:self-center">
                        {item.status === 'update_available' && (
                          <Button
                            size="sm"
                            onClick={() => void handleUpdateSingleMod(item)}
                            className="h-8 gap-1.5 text-xs bg-amber-600 hover:bg-amber-700 text-white font-semibold shadow-xs"
                          >
                            <DownloadCloud className="h-3.5 w-3.5" />
                            <span>Update Jar</span>
                          </Button>
                        )}

                        {/* Pick Jar / View Releases */}
                        {originalMod && (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => {
                              const updateInfo: ModUpdateInfo | undefined =
                                item.status === 'update_available'
                                  ? {
                                      modName: item.mod_name,
                                      modFile: item.mod_file,
                                      provider: item.provider || 'modrinth',
                                      projectId: item.project_id,
                                      projectSlug: item.project_slug,
                                      title: item.title,
                                      currentVersion: item.current_version,
                                      latestVersion: item.compatible_version || '',
                                      latestJar: item.compatible_jar || '',
                                      latestDownloadUrl: item.download_url,
                                      latestReleaseType: item.release_type,
                                    }
                                  : undefined;
                              setPickerMod({ mod: originalMod, updateInfo });
                            }}
                            className="h-8 text-xs"
                            title="View all available jar versions"
                          >
                            <span>Pick Jar...</span>
                          </Button>
                        )}

                        {/* External Project Link */}
                        {item.provider === 'modrinth' && (item.project_slug || item.project_id) && (
                          <a
                            href={`https://modrinth.com/mod/${item.project_slug || item.project_id}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex h-8 w-8 items-center justify-center rounded-lg border border-border/80 text-muted-foreground hover:text-foreground hover:bg-secondary/60 transition-colors"
                            title="Open Modrinth project page"
                          >
                            <ExternalLink className="h-3.5 w-3.5" />
                          </a>
                        )}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </div>
        )}

        {/* Empty state when no report has loaded yet */}
        {!report && !scanning && (
          <div className="rounded-2xl border border-dashed border-border/80 bg-card/50 p-10 sm:p-12 text-center space-y-4">
            <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-primary/10 text-primary">
              <Sparkles className="h-6 w-6" />
            </div>
            <div className="space-y-1.5 max-w-md mx-auto">
              <h3 className="text-base font-semibold text-foreground">
                Ready to check version compatibility?
              </h3>
              <p className="text-xs text-muted-foreground">
                Select your target Minecraft version above and click &quot;Check Compatibility&quot; to scan all {installedMods.length} installed{' '}
                {server.server_type === 'paper' ? 'plugins' : 'mods'}.
              </p>
            </div>
            <Button
              size="sm"
              onClick={() => void scanCompatibility(true)}
              disabled={!effectiveTargetVersion}
              className="gap-1.5 text-xs font-semibold shadow-xs"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              <span>Check Compatibility Now</span>
            </Button>
          </div>
        )}
      </div>

      {/* Mod Jar Picker Dialog for manually choosing builds */}
      {pickerMod && (
        <ModJarPickerDialog
          server={server}
          mod={pickerMod.mod}
          updateInfo={pickerMod.updateInfo}
          isOpen={true}
          onClose={() => setPickerMod(null)}
          onUpdated={async () => {
            setPickerMod(null);
            onRefreshInstalled?.();
            await scanCompatibility(true);
          }}
        />
      )}
    </>
  );
}
