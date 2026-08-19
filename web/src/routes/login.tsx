import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect, useState, type FormEvent } from 'react';
import { useAuth } from '../lib/auth';
import { api, ApiError } from '../api/client';
import { AuthScreen } from '../components/auth-screen';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Label } from '../components/ui/label';

export const Route = createFileRoute('/login')({
  component: LoginRoute,
});

function LoginRoute() {
  const { status, setAuthenticated } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (status === 'authenticated') void navigate({ to: '/dashboard', replace: true });
    if (status === 'onboarding') void navigate({ to: '/onboarding', replace: true });
  }, [status, navigate]);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const me = await api.login(email, password);
      setAuthenticated(me);
      void navigate({ to: '/dashboard', replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        void navigate({ to: '/onboarding', replace: true });
        return;
      }
      setError(err instanceof ApiError ? err.detail : 'Failed to sign in');
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthScreen title="Sign in" subtitle="Log in to manage your Minecraft servers.">
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            type="password"
            required
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <Button type="submit" className="w-full" disabled={busy}>
          {busy ? 'Signing in...' : 'Sign in'}
        </Button>
      </form>
    </AuthScreen>
  );
}
