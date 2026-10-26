import type { ReactNode } from 'react';
import { Box } from 'lucide-react';
import { ThemeToggle } from './theme-toggle';

export function AuthScreen({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  return (
    <div className="grid min-h-screen bg-card text-foreground lg:grid-cols-[1fr_1.1fr]">
      <aside className="hidden flex-col justify-between bg-[#214c42] p-12 text-[#f0f7f3] lg:flex xl:p-16">
        <div className="flex items-center gap-3"><Box className="h-8 w-8" strokeWidth={1.5} /><span className="text-2xl font-semibold tracking-tight">MCM.</span></div>
        <div className="max-w-md py-16">
          <h2 className="text-4xl font-medium leading-[1.2] tracking-tight">Minecraft<br />Container Manager</h2>
          <p className="mt-5 text-sm text-[#c2d9d0]">Self-hosted server management.</p>
        </div>
        <p className="text-xs text-[#c2d9d0]">Minecraft Container Manager</p>
      </aside>
      <div className="relative flex flex-col items-center justify-center px-6 py-20 sm:px-12">
        <div className="absolute right-5 top-5"><ThemeToggle /></div>
        <div className="w-full max-w-sm">
          <div className="mb-10 flex items-center gap-2 text-lg font-semibold lg:hidden"><Box className="h-6 w-6 text-primary" />MCM.</div>
          <h1 className="text-3xl font-semibold tracking-tight">{title}</h1>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">{subtitle}</p>
          <div className="mt-8">{children}</div>
          <p className="mt-8 border-t pt-6 text-xs leading-5 text-muted-foreground">Minecraft Container Manager</p>
        </div>
      </div>
    </div>
  );
}
