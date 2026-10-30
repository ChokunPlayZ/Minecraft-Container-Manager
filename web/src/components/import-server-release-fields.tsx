import { useEffect, useId, useState } from 'react';
import { api } from '../api/client';
import type { ServerType, VersionInfo, VersionMeta } from '../api/types';
import { sortServerBuilds, sortServerVersions } from '../lib/server-releases';
import { Button } from './ui/button';
import { Input } from './ui/input';
import { Label } from './ui/label';
import { Select } from './ui/select';

export function ImportServerReleaseFields({ serverType, version, build, onVersionChange, onBuildChange, disabled }: {
  serverType?: ServerType;
  version: string;
  build: string;
  onVersionChange: (version: string) => void;
  onBuildChange: (build: string) => void;
  disabled: boolean;
}) {
  const id = useId();
  const [versions, setVersions] = useState<VersionMeta[]>([]);
  const [builds, setBuilds] = useState<VersionInfo[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [loadingBuilds, setLoadingBuilds] = useState(false);
  const [versionsError, setVersionsError] = useState(false);
  const [buildsError, setBuildsError] = useState(false);
  const [manualVersion, setManualVersion] = useState(false);
  const [manualBuild, setManualBuild] = useState(false);
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    if (!serverType || serverType === 'custom') return;
    let canceled = false;
    setLoadingVersions(true);
    setVersionsError(false);
    api.jarVersions(serverType).then((items) => {
      if (!canceled) setVersions(sortServerVersions(items));
    }).catch(() => { if (!canceled) setVersionsError(true); })
      .finally(() => { if (!canceled) setLoadingVersions(false); });
    return () => { canceled = true; };
  }, [serverType, retry]);

  useEffect(() => {
    if (!serverType || serverType === 'custom' || !version || manualVersion) return;
    let canceled = false;
    setLoadingBuilds(true);
    setBuildsError(false);
    api.jarBuilds(serverType, version).then((items) => {
      if (!canceled) setBuilds(sortServerBuilds(items));
    }).catch(() => { if (!canceled) setBuildsError(true); })
      .finally(() => { if (!canceled) setLoadingBuilds(false); });
    return () => { canceled = true; };
  }, [serverType, version, manualVersion, retry]);

  function selectVersion(value: string) {
    setManualVersion(value === 'manual');
    setManualBuild(false);
    setBuilds([]);
    setBuildsError(false);
    setLoadingBuilds(false);
    onVersionChange(value === 'manual' || value === 'exported' ? '' : value);
  }

  return <>
    <div className="space-y-2">
      <Label htmlFor={`${id}-version`}>Version</Label>
      <Select id={`${id}-version`} value={manualVersion ? 'manual' : version || 'exported'} disabled={disabled || !serverType || loadingVersions}
        aria-describedby={!serverType ? `${id}-help` : undefined} onValueChange={selectVersion}>
        <option value="exported">{loadingVersions ? 'Loading versions…' : 'Use exported setting'}</option>
        {serverType === 'custom' && <option value="custom">Custom JAR</option>}
        {versions.filter((item) => item.name).map((item) => <option key={item.name} value={item.name}>{item.latest ?? item.name}</option>)}
        <option value="manual">Enter version manually…</option>
      </Select>
      {manualVersion && <Input aria-label="Version number" value={version} maxLength={128} placeholder="e.g. 1.21.1" disabled={disabled} onChange={(event) => onVersionChange(event.target.value)} />}
      {versionsError && <p role="alert" className="text-xs text-destructive">Could not load versions. Enter the original version manually or <Button type="button" variant="link" className="h-auto p-0 text-xs" disabled={disabled} onClick={() => setRetry((value) => value + 1)}>retry</Button>.</p>}
    </div>
    <div className="space-y-2">
      <Label htmlFor={`${id}-build`}>Build</Label>
      <Select id={`${id}-build`} value={manualBuild ? 'manual' : build || 'exported'} disabled={disabled || !serverType || !version || loadingBuilds} onValueChange={(value) => {
        setManualBuild(value === 'manual');
        onBuildChange(value === 'manual' || value === 'exported' ? '' : value);
      }}>
        <option value="exported">{loadingBuilds ? 'Loading builds…' : 'Use exported setting'}</option>
        {builds.filter((item) => item.build).map((item) => <option key={item.build} value={item.build}>{item.build}</option>)}
        <option value="manual">Enter build manually…</option>
      </Select>
      {manualBuild && <Input aria-label="Build number" value={build} maxLength={128} placeholder="Original build" disabled={disabled} onChange={(event) => onBuildChange(event.target.value)} />}
      {buildsError && <p role="alert" className="text-xs text-destructive">Could not load builds. Enter the original build manually or <Button type="button" variant="link" className="h-auto p-0 text-xs" disabled={disabled} onClick={() => setRetry((value) => value + 1)}>retry</Button>.</p>}
    </div>
    {!serverType && <p id={`${id}-help`} className="text-xs text-muted-foreground sm:col-span-2">Choose a server type to browse versions and builds, or keep the exported settings.</p>}
  </>;
}
