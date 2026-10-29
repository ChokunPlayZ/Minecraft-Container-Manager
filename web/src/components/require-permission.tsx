import type { ReactNode } from 'react';
import { usePermissions } from '../lib/auth';

export function RequirePermission({ permission, children }: { permission: string; children: ReactNode }) {
  const can = usePermissions();
  if (can(permission)) return <>{children}</>;
  return <div className="max-w-xl space-y-2 py-8" role="status">
    <h1 className="page-title">Access restricted</h1>
    <p className="text-sm text-muted-foreground">Your account does not have access to this section. An administrator can update your permissions in Users.</p>
  </div>;
}
