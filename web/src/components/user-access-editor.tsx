import { useId, useState } from 'react';
import type { PermissionCatalog, UserAccess } from '../api/types';
import { Label } from './ui/label';
import { Select } from './ui/select';
import { Input } from './ui/input';

export const defaultAccess = (): UserAccess => ({ role: 'viewer', permissions: [], all_servers: false, server_ids: [], disabled: false });

export function UserAccessEditor({ value, onChange, catalog, self = false }: {
  value: UserAccess;
  onChange: (access: UserAccess) => void;
  catalog: PermissionCatalog;
  self?: boolean;
}) {
  const id = useId();
  const [query, setQuery] = useState('');
  const role = catalog.roles.find((entry) => entry.id === value.role);
  const effective = value.role === 'custom' ? value.permissions : role?.permissions ?? [];
  const filtered = catalog.permissions.filter((p) => `${p.label} ${p.description} ${p.group}`.toLowerCase().includes(query.toLowerCase()));
  const groups = [...new Set(filtered.map((p) => p.group))];

  function changeRole(next: UserAccess['role']) {
    onChange({ ...value, role: next, permissions: next === 'custom' ? [...effective] : [],
      all_servers: next === 'administrator' ? true : value.role === 'administrator' ? false : value.all_servers,
      server_ids: next === 'administrator' ? [] : value.server_ids });
  }

  function togglePermission(permission: string, checked: boolean) {
    const readPermissions: Record<string, string> = {
      'files.write': 'files.read', 'files.delete': 'files.read', 'players.manage': 'players.read',
      'ops.manage': 'players.read', 'whitelist.manage': 'players.read', 'mods.manage': 'mods.read',
      'worlds.manage': 'worlds.read', 'worlds.export': 'worlds.read', 'backups.create': 'backups.read',
      'backups.restore': 'backups.read', 'backups.delete': 'backups.read', 'backups.download': 'backups.read',
      'schedules.manage': 'schedules.read', 'performance.manage': 'performance.read', 'dns.manage': 'dns.read',
    };
    let permissions = checked ? [...value.permissions, permission] : value.permissions.filter((p) => p !== permission);
    if (checked && (permission === 'servers.create' || catalog.permissions.find((p) => p.id === permission)?.server_scoped)) permissions.push('servers.view');
    if (checked && readPermissions[permission]) permissions.push(readPermissions[permission]);
    if (checked && permission === 'servers.copy') permissions.push('servers.create', 'files.read');
    if (!checked && ['servers.create', 'files.read'].includes(permission)) permissions = permissions.filter((p) => p !== 'servers.copy');
    if (checked && permission === 'schedules.manage') permissions.push('servers.power', 'console.write', 'backups.create');
    if (!checked && ['servers.power', 'console.write', 'backups.create'].includes(permission)) permissions = permissions.filter((p) => p !== 'schedules.manage');
    if (!checked && permission === 'servers.view') permissions = permissions.filter((p) => p !== 'servers.create' && !catalog.permissions.find((entry) => entry.id === p)?.server_scoped);
    onChange({ ...value, permissions: [...new Set(permissions)], all_servers: checked && ['servers.create', 'servers.copy'].includes(permission) ? true : value.all_servers });
  }

  return <div className="space-y-5">
    <div className="space-y-2">
      <Label htmlFor={`${id}-role`}>Role</Label>
      <Select id={`${id}-role`} value={value.role} onValueChange={(next) => changeRole(next as UserAccess['role'])}>
        {catalog.roles.map((entry) => <option key={entry.id} value={entry.id}>{entry.label}</option>)}
      </Select>
      <p className="text-sm text-muted-foreground">{role?.description}</p>
    </div>
    {value.role !== 'administrator' && <fieldset className="space-y-3">
      <legend className="mb-2 text-sm font-medium">Server access</legend>
      <label className="flex min-h-10 cursor-pointer items-center gap-3 text-sm">
        <input type="checkbox" className="h-4 w-4 shrink-0 accent-primary" checked={value.all_servers}
          disabled={effective.includes('servers.create')}
          onChange={(event) => onChange({ ...value, all_servers: event.target.checked, server_ids: [] })} />
        All current and future servers
      </label>
      {!value.all_servers && <div className="max-h-52 overflow-y-auto rounded-md border p-3">
        {catalog.servers.length === 0 && <p className="text-sm text-muted-foreground">No servers available. This account will have no server access.</p>}
        {[...catalog.servers, ...value.server_ids.filter((sid) => !catalog.servers.some((server) => server.id === sid)).map((sid) => ({ id: sid, name: 'Deleted server' }))].map((server) => <label key={server.id} className="flex min-h-10 cursor-pointer items-center gap-3 text-sm">
          <input type="checkbox" className="h-4 w-4 shrink-0 accent-primary" checked={value.server_ids.includes(server.id)}
            onChange={(event) => onChange({ ...value, server_ids: event.target.checked ? [...value.server_ids, server.id] : value.server_ids.filter((sid) => sid !== server.id) })} />
          <span className="min-w-0 break-words">{server.name}</span>
        </label>)}
      </div>}
      <p className="text-sm text-muted-foreground">{value.all_servers ? 'Permissions apply to every server.' : `${value.server_ids.length} selected. Unselected servers are hidden and cannot be accessed.`}</p>
    </fieldset>}
    <details key={value.role} className="rounded-md border p-4" open={value.role === 'custom' ? true : undefined}>
      <summary className="cursor-pointer text-sm font-medium">{value.role === 'custom' ? 'Choose permissions' : 'Included permissions'} <span className="text-muted-foreground">({effective.length})</span></summary>
      <div className="mt-4 space-y-5">
        <Input aria-label="Filter permissions" placeholder="Filter permissions…" value={query} onChange={(event) => setQuery(event.target.value)} />
        {groups.length === 0 && <p className="text-sm text-muted-foreground">No matching permissions.</p>}
        {groups.map((group) => <fieldset key={group}>
          <legend className="mb-2 text-sm font-semibold">{group}</legend>
          <div className="divide-y">{filtered.filter((p) => p.group === group).map((permission) => <label key={permission.id} className="flex cursor-pointer items-start gap-3 py-3">
            <input type="checkbox" className="mt-1 h-4 w-4 shrink-0 accent-primary" checked={effective.includes(permission.id)} disabled={value.role !== 'custom'}
              onChange={(event) => togglePermission(permission.id, event.target.checked)} />
            <span className="min-w-0"><span className="block text-sm font-medium">{permission.label}</span><span className="mt-1 block text-sm text-muted-foreground">{permission.description}</span></span>
          </label>)}</div>
        </fieldset>)}
      </div>
    </details>
    <label className="flex min-h-10 cursor-pointer items-start gap-3 text-sm">
      <input type="checkbox" className="mt-1 h-4 w-4 shrink-0 accent-primary" checked={value.disabled} disabled={self}
        onChange={(event) => onChange({ ...value, disabled: event.target.checked })} />
      <span><span className="block font-medium">Disable account</span><span className="mt-1 block text-muted-foreground">{self ? 'You cannot disable your own account.' : 'Blocks password and passkey sign-in and revokes existing sessions.'}</span></span>
    </label>
  </div>;
}
