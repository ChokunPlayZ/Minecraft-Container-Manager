import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import type { Server, UserRole } from '../api/types';
import { Button } from './ui/button';
import { Input } from './ui/input';

const roleOptions: { value: UserRole; label: string; description: string }[] = [
  { value: 'member', label: 'Member', description: 'Manages only the servers you select.' },
  { value: 'admin', label: 'Admin', description: 'Full access to every server, users, and panel settings.' },
];

/** Role choice plus, for members, the servers they may manage. */
export function AccessFields({ idPrefix, role, onRoleChange, serverIds, onServerIdsChange, servers, roleLocked }: {
  idPrefix: string;
  role: UserRole;
  onRoleChange: (role: UserRole) => void;
  serverIds: string[];
  onServerIdsChange: (ids: string[]) => void;
  servers: Server[];
  roleLocked?: boolean;
}) {
  return (
    <div className="space-y-4">
      <fieldset className="space-y-2" disabled={roleLocked}>
        <legend className="mb-2 text-sm font-medium">Role</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {roleOptions.map((option) => (
            <label key={option.value} className="flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-primary/5 has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60">
              <input type="radio" name={`${idPrefix}-role`} value={option.value} checked={role === option.value}
                onChange={() => onRoleChange(option.value)} className="mt-0.5 h-4 w-4 accent-primary" />
              <span className="min-w-0">
                <span className="block text-sm font-medium">{option.label}</span>
                <span className="block text-xs text-muted-foreground">{option.description}</span>
              </span>
            </label>
          ))}
        </div>
        {roleLocked && <p className="text-xs text-muted-foreground">You cannot change your own role.</p>}
      </fieldset>
      {role === 'member' && (
        <ServerAccessPicker id={`${idPrefix}-servers`} servers={servers} value={serverIds} onChange={onServerIdsChange} />
      )}
    </div>
  );
}

export function ServerAccessPicker({ id, servers, value, onChange }: {
  id: string;
  servers: Server[];
  value: string[];
  onChange: (ids: string[]) => void;
}) {
  const [query, setQuery] = useState('');
  const selected = new Set(value);
  const visible = useMemo(() => {
    const q = query.toLowerCase().trim();
    return servers.filter((s) => `${s.name} ${s.server_type} ${s.version}`.toLowerCase().includes(q));
  }, [servers, query]);
  const allVisibleSelected = visible.length > 0 && visible.every((s) => selected.has(s.id));

  function toggle(serverId: string) {
    onChange(selected.has(serverId) ? value.filter((v) => v !== serverId) : [...value, serverId]);
  }

  return (
    <fieldset className="space-y-2">
      <legend className="mb-2 text-sm font-medium">Server access</legend>
      <div className="overflow-hidden rounded-lg border">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-muted/40 px-3 py-2">
          <p className="text-xs text-muted-foreground" aria-live="polite">{value.length} of {servers.length} selected</p>
          <div className="flex gap-1">
            <Button type="button" variant="ghost" size="sm" disabled={allVisibleSelected}
              onClick={() => onChange([...value, ...visible.map((s) => s.id).filter((sid) => !selected.has(sid))])}>
              Select all
            </Button>
            <Button type="button" variant="ghost" size="sm" disabled={value.length === 0} onClick={() => onChange([])}>Clear</Button>
          </div>
        </div>
        {servers.length > 6 && (
          <div className="relative border-b p-2">
            <Search className="pointer-events-none absolute left-5 top-5 h-4 w-4 text-muted-foreground" />
            <Input aria-label="Search servers to assign" placeholder="Search servers…" value={query} onChange={(e) => setQuery(e.target.value)} className="pl-9" />
          </div>
        )}
        {servers.length === 0 ? (
          <p className="px-3 py-4 text-sm text-muted-foreground">No servers yet. Create a server, then assign it here.</p>
        ) : visible.length === 0 ? (
          <p className="px-3 py-4 text-sm text-muted-foreground">No matching servers.</p>
        ) : (
          <ul className="max-h-64 overflow-y-auto p-1.5" aria-label="Servers">
            {visible.map((s) => (
              <li key={s.id}>
                <label htmlFor={`${id}-${s.id}`} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-2.5 py-2 transition-colors hover:bg-accent has-[:checked]:bg-accent/60">
                  <input id={`${id}-${s.id}`} type="checkbox" checked={selected.has(s.id)} onChange={() => toggle(s.id)} className="h-4 w-4 shrink-0 accent-primary" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{s.name}</span>
                    <span className="block truncate text-xs text-muted-foreground"><span className="capitalize">{s.server_type}</span> · {s.version}</span>
                  </span>
                </label>
              </li>
            ))}
          </ul>
        )}
      </div>
      {value.length === 0 && servers.length > 0 && (
        <p className="text-xs text-muted-foreground">This member will not see any servers until you select at least one.</p>
      )}
    </fieldset>
  );
}
