import { render, type RenderOptions } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { AuthContext } from '../lib/auth';
import type { Me } from '../api/types';

export const adminUser: Me = { id: 'admin', email: 'admin@example.test', role: 'administrator', all_servers: true, permissions: [], server_ids: [], disabled: false };

export function renderWithUser(ui: ReactElement, user: Me | null, options?: RenderOptions) {
  return render(ui, {
    wrapper: ({ children }: { children: ReactNode }) => <AuthContext.Provider value={{
      user, status: user ? 'authenticated' : 'unauthenticated', refresh: async () => {}, logout: async () => {}, setAuthenticated: () => {},
    }}>{children}</AuthContext.Provider>, ...options,
  });
}

export function renderWithAdmin(ui: ReactElement, options?: RenderOptions) {
  return renderWithUser(ui, adminUser, options);
}
