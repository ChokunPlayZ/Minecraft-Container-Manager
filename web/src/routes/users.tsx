import { useCallback, useEffect, useState } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { ShieldCheck, Trash2, UserPlus } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { User, UserAccess, PermissionCatalog } from '../api/types';
import { useAuth, usePermissions } from '../lib/auth';
import { UserAccessEditor, defaultAccess } from '../components/user-access-editor';
import { RequirePermission } from '../components/require-permission';
import { Badge } from '../components/ui/badge';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { useModal } from '../components/ui/modal';

export const Route = createFileRoute('/users')({
  component: UsersRoute,
});

function UsersRoute() {
  const { user: currentUser, refresh } = useAuth();
  const can = usePermissions();
  const [catalog, setCatalog] = useState<PermissionCatalog | null>(null);
  const [createAccess, setCreateAccess] = useState<UserAccess>(defaultAccess);
  const [editAccess, setEditAccess] = useState<UserAccess>(defaultAccess);
  const [query, setQuery] = useState('');
  const [notice, setNotice] = useState<string | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [createEmail, setCreateEmail] = useState('');
  const [createPassword, setCreatePassword] = useState('');
  const [creating, setCreating] = useState(false);

  // Editing state keyed by user id.
  const [editEmail, setEditEmail] = useState<Record<string, string>>({});
  const [editPassword, setEditPassword] = useState<Record<string, string>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const { confirm, dialog } = useModal();

  const reload = useCallback(async () => {
    try {
      const [data, permissions] = await Promise.all([api.listUsers(), api.permissionCatalog()]);
      setCatalog(permissions);
      setUsers(data);
      setError(null);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (can('users.manage')) void reload();
  }, [reload, can]);

  const handleCreate = async () => {
    if (creating) return;
    setCreating(true);
    setError(null);
    try {
      await api.createUser(createEmail, createPassword, createAccess);
      setCreateAccess(defaultAccess());
      setNotice('User created.');
      setCreateEmail('');
      setCreatePassword('');
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  };

  const startEdit = (u: User) => {
    setEditingId(u.id);
    setEditAccess({ role: u.role ?? 'viewer', permissions: u.permissions ?? [], all_servers: u.all_servers ?? false, server_ids: u.server_ids ?? [], disabled: u.disabled ?? false });
    setEditEmail((m) => ({ ...m, [u.id]: u.email }));
    setEditPassword((m) => ({ ...m, [u.id]: '' }));
  };

  const cancelEdit = () => {
    setEditingId(null);
  };

  const saveEdit = async (u: User) => {
    setBusy((b) => ({ ...b, [u.id]: true }));
    setError(null);
    const email = editEmail[u.id]?.trim() ?? u.email;
    const password = editPassword[u.id] ?? '';
    const input: { email?: string; password?: string; access: UserAccess } = { access: editAccess };
    if (email !== u.email) input.email = email;
    if (password) input.password = password;
    try {
      await api.updateUser(u.id, input);
      setEditingId(null);
      setNotice('Account and permissions saved. Other sessions were signed out.');
      if (u.id === currentUser?.id) await refresh();
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy((b) => ({ ...b, [u.id]: false }));
    }
  };

  const removeUser = async (u: User) => {
    if (!(await confirm(`Delete user ${u.email}? This cannot be undone.`, { title: 'Delete user', confirmLabel: 'Delete', destructive: true }))) return;
    setBusy((b) => ({ ...b, [u.id]: true }));
    setError(null);
    try {
      await api.deleteUser(u.id);
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy((b) => ({ ...b, [u.id]: false }));
    }
  };

  return (
    <>
      {dialog}
      <RequireAuth>
      <AppShell>
        <RequirePermission permission="users.manage">
        <div className="page-heading">
          <div>
            <h1 className="page-title">Users</h1>
            <p className="page-description">
              Manage accounts, roles, permissions, and server access.
            </p>
          </div>
        </div>

        {notice && <p role="status" className="mb-4 text-sm text-primary">{notice}</p>}
        {error && (
          <div role="alert" className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
            <Button variant="outline" size="sm" className="ml-3" onClick={() => void reload()}>Retry loading users</Button>
          </div>
        )}

        <Card className="mb-8 max-w-3xl">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-base"><UserPlus className="h-4 w-4 text-primary" />Add user</CardTitle>
            <CardDescription>New accounts start as viewers with no server access. Choose the servers and capabilities they need.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="new-email">Email</Label>
                <Input
                  id="new-email"
                  type="email"
                  value={createEmail}
                  onChange={(e) => setCreateEmail(e.target.value)}
                  placeholder="person@example.com"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="new-password">Password</Label>
                <Input
                  id="new-password"
                  type="password"
                  value={createPassword}
                  onChange={(e) => setCreatePassword(e.target.value)}
                  placeholder="At least 8 characters"
                />
              </div>
            </div>
            {catalog && <div className="mt-5"><UserAccessEditor value={createAccess} onChange={setCreateAccess} catalog={catalog} /></div>}
            <Button className="mt-4" onClick={() => void handleCreate()} disabled={creating || !catalog || !createEmail.trim() || createPassword.length < 8}>
              {creating ? 'Creating…' : 'Create user'}
            </Button>
          </CardContent>
        </Card>

        <div className="max-w-3xl space-y-3">
          <h2 className="section-heading mb-4 flex items-center gap-2"><ShieldCheck className="h-4 w-4 text-primary" />Panel accounts <span className="text-xs font-normal text-muted-foreground">({users.length})</span></h2>
          {loading && <p className="text-sm text-muted-foreground">Loading users…</p>}
          {!loading && users.length === 0 && (
            <p className="text-sm text-muted-foreground">No users yet.</p>
          )}
          <Input aria-label="Search users" placeholder="Search by email or role…" value={query} onChange={(event) => setQuery(event.target.value)} />
          {!loading && users.length > 0 && !users.some((u) => `${u.email} ${u.role}`.toLowerCase().includes(query.toLowerCase())) && <p className="py-4 text-sm text-muted-foreground">No matching accounts. Try another email or role.</p>}
          {users.filter((u) => `${u.email} ${u.role}`.toLowerCase().includes(query.toLowerCase())).map((u) => (
            <Card key={u.id}>
              <CardContent className="pt-5 sm:pt-6">
                {editingId === u.id ? (
                  <div className="space-y-3">
                    <div className="grid gap-3 sm:grid-cols-2">
                      <div className="space-y-1.5">
                        <Label htmlFor={`email-${u.id}`}>Email</Label>
                        <Input
                          id={`email-${u.id}`}
                          type="email"
                          value={editEmail[u.id] ?? ''}
                          onChange={(e) =>
                            setEditEmail((m) => ({ ...m, [u.id]: e.target.value }))
                          }
                        />
                      </div>
                      <div className="space-y-1.5">
                        <Label htmlFor={`pass-${u.id}`}>New password (optional)</Label>
                        <Input
                          id={`pass-${u.id}`}
                          type="password"
                          value={editPassword[u.id] ?? ''}
                          onChange={(e) =>
                            setEditPassword((m) => ({ ...m, [u.id]: e.target.value }))
                          }
                          placeholder="Leave blank to keep"
                        />
                      </div>
                    </div>
                    {catalog && <UserAccessEditor value={editAccess} onChange={setEditAccess} catalog={catalog} self={u.id === currentUser?.id} />}
                    <div className="flex gap-2">
                      <Button
                        size="sm"
                        onClick={() => void saveEdit(u)}
                        disabled={busy[u.id]}
                      >
                        {busy[u.id] ? 'Saving…' : 'Save'}
                      </Button>
                      <Button size="sm" variant="outline" onClick={cancelEdit}>
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0">
                      <div className="mb-2 flex flex-wrap items-center gap-2"><Badge variant="outline">{catalog?.roles.find((role) => role.id === u.role)?.label ?? u.role}</Badge>{u.disabled && <Badge variant="outline">Disabled</Badge>}{u.id === currentUser?.id && <span className="text-xs text-muted-foreground">You</span>}</div>
                      <p className="truncate font-medium">{u.email}</p>
                      <p className="text-sm text-muted-foreground">
                        Created {new Date(u.created_at).toLocaleDateString()}
                        {u.totp_enabled ? ' · TOTP enabled' : ''}
                        {' · '}{u.role === 'administrator' || u.all_servers ? 'All servers' : `${u.server_ids?.length ?? 0} selected servers`}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button size="sm" variant="outline" onClick={() => startEdit(u)}>
                        Edit
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                        onClick={() => void removeUser(u)}
                        disabled={busy[u.id] || u.id === currentUser?.id}
                        aria-label={`Delete ${u.email}`}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
        </RequirePermission>
      </AppShell>
      </RequireAuth>
    </>
  );
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return 'Something went wrong';
}
