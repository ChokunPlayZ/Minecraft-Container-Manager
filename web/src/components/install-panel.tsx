import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Loader2, Sparkles } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { InstallInfo, ServerType, VersionInfo, VersionMeta } from '../api/types';
import { Button } from './ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card';
import { Label } from './ui/label';
import { ProgressBar } from './ui/progress';
import { Select } from './ui/select';
import { useModal } from './ui/modal';

export function InstallPanel({
  serverId,
  serverType,
  onInstalled,
}: {
  serverId: string;
  serverType: ServerType;
  onInstalled: () => void;
}) {
  const [info, setInfo] = useState<InstallInfo | null>(null);
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [builds, setBuilds] = useState<VersionInfo[]>([]);
  const [version, setVersion] = useState('');
  const [build, setBuild] = useState('');
  const [loading, setLoading] = useState(true);
  const [loadingBuilds, setLoadingBuilds] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useModal();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [i, v] = await Promise.all([api.installInfo(serverId), api.jarVersions(serverType)]);
      setInfo(i);
      setVersions(v);
      const curVer = i.version ?? (i as any)?.server?.version ?? v[0]?.name ?? '';
      const curBuild = i.build ?? (i as any)?.server?.build ?? '';
      setVersion(curVer);
      setBuild(curBuild);
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to load install info');
    } finally {
      setLoading(false);
    }
  }, [serverId, serverType]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!version) return;
    let cancelled = false;
    setLoadingBuilds(true);
    api
      .jarBuilds(serverType, version)
      .then((b) => {
        if (cancelled) return;
        const sorted = [...b].sort((x, y) => {
          const nx = parseInt(x.build, 10);
          const ny = parseInt(y.build, 10);
          if (!isNaN(nx) && !isNaN(ny) && String(nx) === x.build && String(ny) === y.build) {
            return ny - nx;
          }
          return (y.build || '').localeCompare(x.build || '', undefined, { numeric: true });
        });
        setBuilds(sorted);
        const curBuild = info?.build ?? (info as any)?.server?.build;
        if (!curBuild || !sorted.some((x) => x.build === curBuild)) {
          setBuild(sorted[0]?.build ?? '');
        }
      })
      .catch((err) => !cancelled && setError(err instanceof ApiError ? err.detail : 'Failed to load builds'))
      .finally(() => !cancelled && setLoadingBuilds(false));
    return () => {
      cancelled = true;
    };
  }, [version, serverType, info]);

  const isInstalled = Boolean(info?.installed || (info as any)?.server?.container_id);
  const installedVer = info?.version ?? (info as any)?.server?.version;
  const installedBuild = info?.build ?? (info as any)?.server?.build;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!version || !build) return;

    const action = isInstalled ? 'upgrade' : 'install';
    const confirmed = await confirm(
      `Are you sure you want to ${action} the server jar ${version} (build ${build})? ` +
        (isInstalled
          ? 'This will replace the currently installed jar and may restart the server.'
          : 'This will create the server container and download the jar.'),
      { title: `${action === 'upgrade' ? 'Upgrade' : 'Install'} server jar`, confirmLabel: action === 'upgrade' ? 'Upgrade' : 'Install', destructive: action === 'upgrade' },
    );
    if (!confirmed) return;

    setBusy(true);
    setError(null);
    try {
      setInfo(await api.installServer(serverId, { version, build }));
      onInstalled();
    } catch (err) {
      setError(err instanceof ApiError ? err.detail : 'Failed to install');
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {dialog}
      <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Install</CardTitle>
        <CardDescription>Choose a jar version and build to install or upgrade.</CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="flex h-10 items-center text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
          </div>
        ) : (
          <>
            {!isInstalled && (
              <p className="mb-4 text-sm text-muted-foreground">
                No jar installed yet for this server.
              </p>
            )}
            {isInstalled && (
              <p className="mb-4 text-sm text-muted-foreground">
                Installed: {installedVer} {installedBuild ? `(build ${installedBuild})` : ''}
              </p>
            )}
            {serverType !== 'vanilla' && isInstalled && (
              <div className="mb-4 flex items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-3 text-xs">
                <div className="flex items-center gap-2 text-muted-foreground">
                  <Sparkles className="h-4 w-4 text-primary shrink-0" />
                  <span>
                    Planning a version upgrade? Check mod compatibility first in the{' '}
                    <strong className="text-foreground">Version Update Helper</strong> under Mods &amp; Plugins.
                  </span>
                </div>
              </div>
            )}
            <form onSubmit={onSubmit} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1.5">
                  <Label htmlFor="install-version">Version</Label>
                  <Select
                    id="install-version"
                    value={version}
                    onChange={(e) => setVersion(e.target.value)}
                    disabled={versions.length === 0}
                  >
                    {versions.map((v) => (
                      <option key={v.name} value={v.name}>
                        {v.latest ?? v.name}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="install-build">Build</Label>
                  {loadingBuilds ? (
                    <div className="flex h-9 items-center text-muted-foreground">
                      <Loader2 className="h-4 w-4 animate-spin" />
                    </div>
                  ) : (
                    <Select
                      id="install-build"
                      value={build}
                      onChange={(e) => setBuild(e.target.value)}
                      disabled={builds.length === 0}
                    >
                      {builds.map((b) => (
                        <option key={b.build} value={b.build}>
                          {b.build}
                        </option>
                      ))}
                    </Select>
                  )}
                </div>
              </div>
              {error && <p className="text-sm text-destructive">{error}</p>}
              {busy && (
                <div className="rounded-xl border border-primary/20 bg-primary/5 p-3.5 animate-fadeIn">
                  <ProgressBar
                    label={
                      <span className="flex items-center gap-2 font-medium">
                        <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
                        <span>{isInstalled ? 'Upgrading server jar...' : 'Downloading jar & setting up server...'}</span>
                      </span>
                    }
                    subtext="Fetching binaries and configuring container"
                    showPercent={false}
                    size="md"
                  />
                </div>
              )}
              <Button
                type="submit"
                variant="destructive"
                disabled={busy || loadingBuilds || !version || !build}
              >
                {busy ? 'Installing...' : isInstalled ? 'Upgrade' : 'Install'}
              </Button>
            </form>
          </>
        )}
      </CardContent>
      </Card>
    </>
  );
}
