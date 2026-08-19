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

const AuthContext = createContext<AuthContextValue | null>(null);

function statusFromError(err: unknown): AuthStatus {
  if (err instanceof ApiError) {
    if (err.status === 404) {
      const detail = err.detail.toLowerCase();
      if (detail.includes('onboard') || detail.includes('admin') || detail.includes('no admin')) {
        return 'onboarding';
      }
    }
    if (err.status === 401) return 'unauthenticated';
    if (err.status === 403) return 'unauthenticated';
  }
  return 'unauthenticated';
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading');
  const [user, setUser] = useState<Me | null>(null);

  const refresh = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
      setStatus('authenticated');
    } catch (err) {
      setUser(null);
      setStatus(statusFromError(err));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

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
