import type { ReactNode } from 'react';
import { Link } from '@tanstack/react-router';
import { Boxes, LogOut } from 'lucide-react';
import { useAuth } from '../lib/auth';
import { ThemeToggle } from './theme-toggle';
import { Button } from './ui/button';

export function AppShell({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      <header className="sticky top-0 z-20 border-b bg-background/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4">
          <Link to="/dashboard" className="flex items-center gap-2 font-semibold">
            <Boxes className="h-5 w-5 text-primary" />
            <span>MCM</span>
          </Link>
          <nav className="ml-4 flex items-center gap-1 text-sm">
            <Link
              to="/dashboard"
              className="rounded-md px-3 py-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              activeProps={{ className: 'bg-accent text-accent-foreground' }}
            >
              Servers
            </Link>
            <Link
              to="/settings"
              className="rounded-md px-3 py-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              activeProps={{ className: 'bg-accent text-accent-foreground' }}
            >
              Settings
            </Link>
            <Link
              to="/users"
              className="rounded-md px-3 py-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              activeProps={{ className: 'bg-accent text-accent-foreground' }}
            >
              Users
            </Link>
          </nav>
          <div className="ml-auto flex items-center gap-2">
            {user && (
              <span className="hidden text-sm text-muted-foreground sm:inline">{user.email}</span>
            )}
            <ThemeToggle />
            <Button variant="ghost" size="icon" onClick={() => void logout()} aria-label="Log out">
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">{children}</main>
    </div>
  );
}
