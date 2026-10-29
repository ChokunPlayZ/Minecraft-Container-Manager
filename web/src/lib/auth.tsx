import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { api, ApiError } from '../api/client';
import type { Me } from '../api/types';

export type AuthStatus = 'loading' | 'onboarding' | 'unauthenticated' | 'authenticated';

interface AuthContextValue {
  status: AuthStatus;
  user: Me | null;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  setAuthenticated: (user: Me) => void;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading');
  const [user, setUser] = useState<Me | null>(null);

  const refresh = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
      setStatus('authenticated');
      return;
    } catch {
      setUser(null);
    }

    // No valid session. If the instance still needs its first admin account,
    // route straight to onboarding so a fresh install never dead-ends on login.
    // A set-up instance is never sent here, so this only fires the first time.
    try {
      const onboard = await api.onboardingStatus();
      if (onboard.onboarding_required) {
        setStatus('onboarding');
        return;
      }
    } catch {
      /* endpoint unavailable; fall through to unauthenticated */
    }
    setStatus('unauthenticated');
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (status !== 'authenticated') return;
    const update = async () => {
      if (document.hidden) return;
      try { setUser(await api.me()); }
      catch (error) { if (error instanceof ApiError && error.status === 401) await refresh(); }
    };
    const timer = setInterval(update, 60000);
    window.addEventListener('focus', update);
    return () => { clearInterval(timer); window.removeEventListener('focus', update); };
  }, [status, refresh]);

  const logout = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      /* ignore logout errors */
    }
    setUser(null);
    setStatus('unauthenticated');
  }, []);

  const setAuthenticated = useCallback((me: Me) => {
    setUser(me);
    setStatus('authenticated');
  }, []);

  const value = useMemo(
    () => ({ status, user, refresh, logout, setAuthenticated }),
    [status, user, refresh, logout, setAuthenticated],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within an AuthProvider');
  return ctx;
}

// Client checks control presentation; the API independently authorizes every request.
export function can(user: Me | null | undefined, permission: string, serverId?: string): boolean {
  if (!user || user.disabled || !user.role) return false;
  const has = (value: string) => user.role === 'administrator' || !!user.permissions?.includes(value);
  if (!has(permission)) return false;
  if (serverId && (!has('servers.view') || !(user.role === 'administrator' || user.all_servers || user.server_ids?.includes(serverId)))) return false;
  if (permission === 'servers.copy' && (!has('servers.create') || !has('files.read') || !user.all_servers)) return false;
  return true;
}

export function usePermissions() {
  const context = useContext(AuthContext);
  return useCallback((permission: string, serverId?: string) => can(context?.user, permission, serverId), [context?.user]);
}
