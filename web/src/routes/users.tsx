import { useCallback, useEffect, useState } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { User } from '../api/types';
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
      const data = await api.listUsers();
      setUsers(data);
      setError(null);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const handleCreate = async () => {
    if (creating) return;
    setCreating(true);
    setError(null);
    try {
      await api.createUser(createEmail, createPassword);
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
    const input: { email?: string; password?: string } = {};
    if (email !== u.email) input.email = email;
    if (password) input.password = password;
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
      <RequireAuth>
      <AppShell>
        <div className="mb-6 flex items-end justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold">Users</h1>
            <p className="text-sm text-muted-foreground">
              Manage panel accounts. Everyone with an account is an admin.
            </p>
          </div>
        </div>

        {error && (
          <div className="mb-4 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        )}

        <Card className="mb-6 max-w-2xl">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Add user</CardTitle>
            <CardDescription>Create a new panel account.</CardDescription>
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
            <Button className="mt-4" onClick={() => void handleCreate()} disabled={creating}>
              {creating ? 'Creating…' : 'Create user'}
            </Button>
          </CardContent>
        </Card>

        <div className="space-y-3">
          {loading && <p className="text-sm text-muted-foreground">Loading users…</p>}
          {!loading && users.length === 0 && (
            <p className="text-sm text-muted-foreground">No users yet.</p>
          )}
          {users.map((u) => (
            <Card key={u.id}>
              <CardContent className="pt-6">
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
                      <p className="truncate font-medium">{u.email}</p>
                      <p className="text-sm text-muted-foreground">
                        Created {new Date(u.created_at).toLocaleDateString()}
                        {u.totp_enabled ? ' · TOTP enabled' : ''}
                      </p>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button size="sm" variant="outline" onClick={() => startEdit(u)}>
                        Edit
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
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
