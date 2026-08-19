import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { api } from '../api/client';
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
