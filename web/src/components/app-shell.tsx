import { useState, type ReactNode } from 'react';
import { Link, useRouterState } from '@tanstack/react-router';
import { Archive, Box, ChevronRight, LogOut, Menu, Server, Settings2, Users, X } from 'lucide-react';
import { useAuth } from '../lib/auth';
import { ThemeToggle } from './theme-toggle';
import { Button } from './ui/button';

const destinations = [
  { to: '/dashboard', label: 'Servers', icon: Server },
  { to: '/backups', label: 'Backups', icon: Archive },
  { to: '/settings', label: 'Settings', icon: Settings2 },
  { to: '/users', label: 'Users', icon: Users },
] as const;

export function AppShell({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const pathname = useRouterState({ select: (state) => state.location.pathname });
  const [menuOpen, setMenuOpen] = useState(false);
  const isServer = pathname.startsWith('/servers/');
  const current = destinations.find((item) => item.to === pathname) ?? destinations[0];
  const isPreview = typeof window !== 'undefined' &&
    (window.location.search.includes('mock=true') || window.localStorage?.getItem('mcm-mock') === 'true');

  function navigation(mobile = false) {
    return (
      <nav aria-label={mobile ? 'Mobile navigation' : 'Main navigation'} className="space-y-1.5">
        {destinations.map(({ to, label, icon: Icon }) => {
          const active = to === current.to;
          return (
            <Link key={to} to={to} onClick={() => setMenuOpen(false)}
              aria-current={active ? 'page' : undefined}
              className={`flex items-center gap-3 rounded-lg px-3 py-3 transition-colors ${active ? 'bg-card text-foreground ring-1 ring-border' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}`}>
              <Icon className={`h-[18px] w-[18px] shrink-0 ${active ? 'text-primary' : ''}`} strokeWidth={1.7} />
              <span className="min-w-0 text-sm font-medium">{label}</span>
              {active && <ChevronRight className="ml-auto h-3.5 w-3.5 text-muted-foreground" />}
            </Link>
          );
        })}
      </nav>
    );
  }

  return (
    <div className="min-h-screen bg-background text-foreground">
      <a href="#main-content" className="sr-only fixed left-4 top-4 z-[200] rounded-md bg-primary px-4 py-3 text-primary-foreground focus:not-sr-only">Skip to content</a>
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-56 flex-col border-r bg-sidebar lg:flex">
        <Link to="/dashboard" className="flex h-24 items-center gap-3 px-6" aria-label="MCM home">
          <Box className="h-8 w-8 text-primary" strokeWidth={1.5} />
          <div><span className="text-xl font-semibold tracking-tight">MCM<span className="text-primary">.</span></span><span className="mt-0.5 block text-[11px] text-muted-foreground">Minecraft Container Manager</span></div>
        </Link>
        <div className="px-4 pt-4">{navigation()}</div>
        <div className="mt-auto border-t p-4">
          <Link to="/settings" search={{ tab: 'account' }} aria-label="Account settings" className="mb-3 flex min-w-0 items-center gap-2.5 rounded-md px-1 py-2 transition-colors hover:bg-accent">
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full border bg-card text-xs font-semibold text-primary" aria-hidden="true">{user?.email?.[0]?.toUpperCase() ?? 'A'}</span>
            <div className="min-w-0"><p className="text-xs font-medium">Administrator</p><p className="truncate text-xs text-muted-foreground" title={user?.email}>{user?.email}</p></div>
            <ChevronRight className="ml-auto h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          </Link>
          <Button variant="ghost" size="sm" onClick={() => void logout()} className="w-full justify-start"><LogOut />Log out</Button>
        </div>
      </aside>
      <div className="lg:pl-56">
        <header className="relative z-20 border-b bg-card">
          <div className="mx-auto flex h-16 max-w-[1480px] items-center gap-3 px-5 sm:px-8 lg:px-10">
            <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setMenuOpen(!menuOpen)} aria-label={menuOpen ? 'Close navigation' : 'Open navigation'} aria-expanded={menuOpen} aria-controls="mobile-navigation">{menuOpen ? <X /> : <Menu />}</Button>
            <Link to="/dashboard" className="flex items-center gap-2 text-base font-semibold lg:hidden"><Box className="h-5 w-5 text-primary" /> MCM</Link>
            <div className="hidden items-center gap-3 text-sm sm:flex"><span className="text-muted-foreground">Workspace</span><ChevronRight className="h-3 w-3 text-muted-foreground" /><span className="font-medium">{isServer ? 'Server management' : current.label}</span></div>
            <div className="ml-auto flex items-center gap-3">
              {isPreview && <span className="rounded-md border px-2 py-1 text-[11px] text-muted-foreground">Demo data</span>}
              <ThemeToggle />
              <Button variant="ghost" size="icon" className="lg:hidden" asChild><Link to="/settings" search={{ tab: 'account' }} aria-label="Account settings" onClick={() => setMenuOpen(false)}><span className="flex h-8 w-8 items-center justify-center rounded-full border bg-secondary text-xs font-semibold text-primary">{user?.email?.[0]?.toUpperCase() ?? 'A'}</span></Link></Button>
            </div>
          </div>
          {menuOpen && <div id="mobile-navigation" className="border-t p-4 lg:hidden">{navigation(true)}<div className="mt-4 flex min-w-0 items-center justify-between gap-3 border-t pt-3"><Link to="/settings" search={{ tab: 'account' }} onClick={() => setMenuOpen(false)} className="min-w-0 rounded-md px-3 py-2 text-sm hover:bg-accent"><span className="block font-medium">Account settings</span><span className="block truncate text-xs text-muted-foreground">{user?.email}</span></Link><Button variant="ghost" size="sm" onClick={() => void logout()}><LogOut />Log out</Button></div></div>}
        </header>
        <main id="main-content" tabIndex={-1} className="mx-auto w-full max-w-[1480px] px-5 py-8 outline-none sm:px-8 lg:px-10 lg:py-10">{children}</main>
        <footer className="mx-auto flex max-w-[1480px] flex-wrap items-center justify-between gap-2 px-5 py-6 text-xs text-muted-foreground sm:px-8 lg:px-10"><span>Minecraft Container Manager</span></footer>
      </div>
    </div>
  );
}
