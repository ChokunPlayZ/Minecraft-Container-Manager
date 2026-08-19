import type { ReactNode } from 'react';
import { Boxes } from 'lucide-react';
import { ThemeToggle } from './theme-toggle';

export function AuthScreen({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  return (
    <div className="relative flex min-h-screen flex-col items-center justify-center bg-background px-4 text-foreground">
      <div className="absolute right-4 top-4">
        <ThemeToggle />
      </div>
      <div className="mb-6 flex items-center gap-2 text-lg font-semibold">
        <Boxes className="h-6 w-6 text-primary" />
        MCM
      </div>
      <div className="w-full max-w-sm rounded-lg border bg-card p-6 shadow-sm">
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p>
        <div className="mt-5">{children}</div>
      </div>
    </div>
  );
}
