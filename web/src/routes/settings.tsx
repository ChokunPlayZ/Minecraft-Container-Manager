import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { createFileRoute } from '@tanstack/react-router';
import { KeyRound, Plus, ShieldCheck, Trash2 } from 'lucide-react';
import { api, ApiError } from '../api/client';
import type { PasskeyMeta } from '../api/types';
import { AppShell } from '../components/app-shell';
import { RequireAuth } from '../components/require-auth';
import { useAuth } from '../lib/auth';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';
import { useModal } from '../components/ui/modal';

export const Route = createFileRoute('/settings')({
  component: SettingsRoute,
});

function SettingsRoute() {
  const { user, refresh } = useAuth();

  return (
    <RequireAuth>
      <AppShell>
        <div className="mb-6">
          <h1 className="text-2xl font-semibold">Account</h1>
          <p className="text-sm text-muted-foreground">
            Manage your profile, sign-in security, and authenticators.
          </p>
        </div>
        <div className="max-w-2xl space-y-6">
          <ProfileCard email={user?.email ?? ''} userId={user?.id ?? ''} onSaved={() => void refresh()} />
          <TOTPCard />
          <PasskeyCard />
        </div>
      </AppShell>
    </RequireAuth>
  );
}

function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.detail || e.message;
  if (e instanceof Error) return e.message;
  return 'Something went wrong';
}

function ProfileCard({ email, userId, onSaved }: { email: string; userId: string; onSaved: () => void }) {
  const [newEmail, setNewEmail] = useState(email);
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    setSaved(false);
    const input: { email?: string; password?: string } = {};
    const trimmed = newEmail.trim();
    if (trimmed && trimmed !== email) input.email = trimmed;
    if (password) input.password = password;
    try {
      if (Object.keys(input).length === 0) {
        setError('Nothing to change.');
        return;
      }
      await api.updateUser(userId, input);
      setPassword('');
      setSaved(true);
      onSaved();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Profile</CardTitle>
        <CardDescription>Update your email address and password.</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="acc-email">Email</Label>
            <Input
              id="acc-email"
              type="email"
              value={newEmail}
              onChange={(e) => setNewEmail(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="acc-password">New password (optional)</Label>
            <Input
              id="acc-password"
              type="password"
              autoComplete="new-password"
              value={password}
              placeholder="At least 8 characters"
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          {saved && <p className="text-sm text-emerald-600">Saved.</p>}
          <Button type="submit" disabled={busy}>
            {busy ? 'Saving…' : 'Save changes'}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function TOTPCard() {
  const [enabled, setEnabled] = useState(false);
  const [loading, setLoading] = useState(true);
  const [enrolling, setEnrolling] = useState(false);
  const [secret, setSecret] = useState('');
  const [qrUri, setQrUri] = useState('');
  const [confirmCode, setConfirmCode] = useState('');
  const [disableCode, setDisableCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const status = await api.totpStatus();
      setEnabled(status.totp_enabled);
      if (!status.totp_enabled) {
        setSecret('');
        setQrUri('');
        setConfirmCode('');
      }
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

  async function startEnroll() {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      const res = await api.totpEnroll();
      setSecret(res.secret);
      setQrUri(res.qr_uri);
      setEnrolling(true);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function confirm() {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.totpConfirm(confirmCode);
      setEnrolling(false);
      setConfirmCode('');
      setSecret('');
      setQrUri('');
      setEnabled(true);
      setSaved('Two-factor authentication enabled.');
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function disable() {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.totpDisable(disableCode);
      setDisableCode('');
      setEnabled(false);
      setSaved('Two-factor authentication disabled.');
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Two-Factor Authentication</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Loading…</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base flex items-center gap-2">
          <ShieldCheck className="h-4 w-4" />
          Two-Factor Authentication
        </CardTitle>
        <CardDescription>
          Add a time-based one-time password for an extra layer of security.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        {saved && <p className="text-sm text-emerald-600">{saved}</p>}

        {enabled && !enrolling ? (
          <div className="space-y-3">
            <p className="text-sm text-emerald-600">Two-factor authentication is enabled.</p>
            <div className="space-y-1.5">
              <Label htmlFor="totp-disable-code">Current 6-digit code</Label>
              <Input
                id="totp-disable-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                value={disableCode}
                onChange={(e) => setDisableCode(e.target.value)}
                placeholder="000000"
              />
            </div>
            <Button type="button" variant="destructive" onClick={() => void disable()} disabled={busy}>
              {busy ? 'Working…' : 'Disable 2FA'}
            </Button>
          </div>
        ) : enrolling ? (
          <div className="space-y-4">
            <div className="rounded-md border p-3">
              <p className="text-sm font-medium">Scan or enter this recovery secret in your authenticator app.</p>
              <p className="mt-2 font-mono text-sm break-all">{secret}</p>
              <p className="mt-3 text-xs text-muted-foreground">Setup URI (manual entry):</p>
              <p className="font-mono text-xs break-all text-muted-foreground">{qrUri}</p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="totp-confirm-code">Enter the 6-digit code from your app</Label>
              <Input
                id="totp-confirm-code"
                inputMode="numeric"
                autoComplete="one-time-code"
                value={confirmCode}
                onChange={(e) => setConfirmCode(e.target.value)}
                placeholder="000000"
              />
            </div>
            <div className="flex gap-2">
              <Button type="button" onClick={() => void confirm()} disabled={busy}>
                {busy ? 'Working…' : 'Enable 2FA'}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setEnrolling(false);
                  void reload();
                }}
                disabled={busy}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">Two-factor authentication is currently off.</p>
            <Button type="button" onClick={() => void startEnroll()} disabled={busy}>
              {busy ? 'Working…' : 'Set up 2FA'}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PasskeyCard() {
  const [passkeys, setPasskeys] = useState<PasskeyMeta[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const { confirm, dialog } = useModal();

  const reload = useCallback(async () => {
    try {
      const list = await api.passkeyList();
      setPasskeys(list);
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

  async function register() {
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      const begin = await api.passkeyRegisterBegin();
      if (!navigator.credentials || typeof navigator.credentials.create !== 'function') {
        setError('This browser does not support passkeys.');
        return;
      }
      const credential = (await navigator.credentials.create({
        publicKey: begin.options as PublicKeyCredentialCreationOptions,
      })) as PublicKeyCredential | null;
      if (!credential) {
        setError('Passkey registration was cancelled.');
        return;
      }
      await api.passkeyRegisterFinish(begin.registration_id, credential.toJSON());
      setSaved('Passkey added.');
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(p: PasskeyMeta) {
    if (!(await confirm(`Delete passkey ${p.name || p.id}?`, { title: 'Delete passkey', confirmLabel: 'Delete', destructive: true }))) return;
    setBusy(true);
    setError(null);
    setSaved(null);
    try {
      await api.passkeyDelete(p.id);
      setSaved('Passkey removed.');
      await reload();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      {dialog}
      <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base flex items-center gap-2">
          <KeyRound className="h-4 w-4" />
          Passkeys
        </CardTitle>
        <CardDescription>Sign in quickly with a passkey on this device.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        {saved && <p className="text-sm text-emerald-600">{saved}</p>}

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : passkeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No passkeys registered.</p>
        ) : (
          <div className="space-y-2">
            {passkeys.map((p) => (
              <div key={p.id} className="flex items-center justify-between gap-3 rounded-md border p-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{p.name || 'Passkey'}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{p.id}</p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`Delete passkey ${p.name || p.id}`}
                  onClick={() => void remove(p)}
                  disabled={busy}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}

        <Button type="button" onClick={() => void register()} disabled={busy}>
          <Plus className="h-4 w-4" />
          {busy ? 'Working…' : 'Add passkey'}
        </Button>
      </CardContent>
      </Card>
    </>
  );
}
