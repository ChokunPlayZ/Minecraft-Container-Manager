import { useCallback, useEffect, useState } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { ShieldCheck, Trash2, UserPlus, Users as UsersIcon } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { Server, User, UserInput, UserRole } from '../api/types';
import { useAuth, useIsAdmin } from '../lib/auth';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { AccessFields } from '../components/server-access-picker';
import { Badge } from '../components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { useModal } from '../components/ui/modal';

export const Route = createFileRoute('/users')({
  component: UsersRoute,
});

function UsersRoute() {
  return (
    <RequireAuth>
      <AppShell>
        <UsersPage />
      </AppShell>
    </RequireAuth>
  );
}

export function UsersPage() {
  const isAdmin = useIsAdmin();
  const { user: me } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [servers, setServers] = useState<Server[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [createEmail, setCreateEmail] = useState('');
  const [createPassword, setCreatePassword] = useState('');
  const [createRole, setCreateRole] = useState<UserRole>('member');
  const [createServerIds, setCreateServerIds] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);

  // Editing state for the one user being edited.
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editEmail, setEditEmail] = useState('');
  const [editPassword, setEditPassword] = useState('');
  const [editRole, setEditRole] = useState<UserRole>('member');
  const [editServerIds, setEditServerIds] = useState<string[]>([]);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const { confirm, dialog } = useModal();

  const reload = useCallback(async () => {
    try {
      const [userList, serverList] = await Promise.all([api.listUsers(), api.listServers()]);
      setUsers(userList);
      setServers(serverList);
      setError(null);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isAdmin) void reload();
  }, [isAdmin, reload]);

  if (!isAdmin) {
    return (
      <div className="empty-state rounded-lg border bg-card">
        <UsersIcon />
        <h1 className="section-heading">Users</h1>
        <p>Only administrators can manage panel accounts.</p>
      </div>
    );
  }

  const serverName = (id: string) => servers.find((s) => s.id === id)?.name;

  const handleCreate = async () => {
    if (creating) return;
    setCreating(true);
    setError(null);
    try {
      await api.createUser({
        email: createEmail,
        password: createPassword,
        role: createRole,
        server_ids: createRole === 'member' ? createServerIds : [],
      });
      setCreateEmail('');
      setCreatePassword('');
      setCreateServerIds([]);
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setCreating(false);
    }
  };

  const startEdit = (u: User) => {
    setEditingId(u.id);
    setEditEmail(u.email);
    setEditPassword('');
    setEditRole(u.role);
    setEditServerIds(u.server_ids);
  };

  const saveEdit = async (u: User) => {
    setBusy((b) => ({ ...b, [u.id]: true }));
    setError(null);
    const email = editEmail.trim();
    const input: UserInput = {};
    if (email !== u.email) input.email = email;
    if (editPassword) input.password = editPassword;
    if (editRole !== u.role) input.role = editRole;
    if (editRole === 'member') input.server_ids = editServerIds;
    try {
      await api.updateUser(u.id, input);
      setEditingId(null);
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
      <div className="page-heading">
        <div>
          <h1 className="page-title">Users</h1>
          <p className="page-description">
            Admins manage everything. Members manage only the servers you assign to them.
          </p>
        </div>
      </div>

      {error && (
        <div role="alert" className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <Card className="mb-8 max-w-3xl">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base"><UserPlus className="h-4 w-4 text-primary" />Add user</CardTitle>
          <CardDescription>Create a panel account and choose what it can manage.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
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
          <AccessFields
            idPrefix="new-user"
            role={createRole}
            onRoleChange={setCreateRole}
            serverIds={createServerIds}
            onServerIdsChange={setCreateServerIds}
            servers={servers}
          />
          <Button onClick={() => void handleCreate()} disabled={creating || !createEmail.trim() || createPassword.length < 8}>
            {creating ? 'Creating…' : 'Create user'}
          </Button>
        </CardContent>
      </Card>

      <div className="max-w-3xl space-y-3">
        <h2 className="section-heading mb-4 flex items-center gap-2"><ShieldCheck className="h-4 w-4 text-primary" />Accounts <span className="text-xs font-normal text-muted-foreground">({users.length})</span></h2>
        {loading && <p className="text-sm text-muted-foreground">Loading users…</p>}
        {!loading && users.length === 0 && (
          <p className="text-sm text-muted-foreground">No users yet.</p>
        )}
        {users.map((u) => (
          <Card key={u.id}>
            <CardContent className="pt-5 sm:pt-6">
              {editingId === u.id ? (
                <div className="space-y-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label htmlFor={`email-${u.id}`}>Email</Label>
                      <Input
                        id={`email-${u.id}`}
                        type="email"
                        value={editEmail}
                        onChange={(e) => setEditEmail(e.target.value)}
                      />
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor={`pass-${u.id}`}>New password (optional)</Label>
                      <Input
                        id={`pass-${u.id}`}
                        type="password"
                        value={editPassword}
                        onChange={(e) => setEditPassword(e.target.value)}
                        placeholder="Leave blank to keep"
                      />
                    </div>
                  </div>
                  <AccessFields
                    idPrefix={`edit-${u.id}`}
                    role={editRole}
                    onRoleChange={setEditRole}
                    serverIds={editServerIds}
                    onServerIdsChange={setEditServerIds}
                    servers={servers}
                    roleLocked={u.id === me?.id}
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      onClick={() => void saveEdit(u)}
                      disabled={busy[u.id]}
                    >
                      {busy[u.id] ? 'Saving…' : 'Save'}
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => setEditingId(null)}>
                      Cancel
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="flex items-start justify-between gap-4">
                  <div className="min-w-0 space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="truncate font-medium">{u.email}</p>
                      <Badge variant={u.role === 'admin' ? 'default' : 'secondary'}>{u.role === 'admin' ? 'Admin' : 'Member'}</Badge>
                      {u.id === me?.id && <span className="text-xs text-muted-foreground">You</span>}
                    </div>
                    <p className="text-sm text-muted-foreground">
                      Created {new Date(u.created_at).toLocaleDateString()}
                      {u.totp_enabled ? ' · TOTP enabled' : ''}
                    </p>
                    {u.role === 'member' && <AccessSummary names={u.server_ids.map(serverName).filter((n): n is string => !!n)} />}
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
                      disabled={busy[u.id]}
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
    </>
  );
}

const SUMMARY_LIMIT = 3;

function AccessSummary({ names }: { names: string[] }) {
  if (names.length === 0) {
    return <p className="text-sm text-amber-800 dark:text-amber-300">No servers assigned</p>;
  }
  const shown = names.slice(0, SUMMARY_LIMIT);
  return (
    <p className="text-sm">
      <span className="text-muted-foreground">Can manage: </span>
      {shown.join(', ')}
      {names.length > shown.length && <span className="text-muted-foreground"> +{names.length - shown.length} more</span>}
    </p>
  );
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return 'Something went wrong';
}
