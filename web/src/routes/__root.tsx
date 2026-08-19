import { createRootRoute, Outlet } from '@tanstack/react-router';
import { ThemeProvider } from '../components/theme-provider';
import { AuthProvider } from '../lib/auth';
import '../styles/index.css';

export const Route = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFound,
});

function RootComponent() {
  return (
    <ThemeProvider>
      <AuthProvider>
        <Outlet />
      </AuthProvider>
    </ThemeProvider>
  );
}

function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-background text-foreground">
      <p className="text-muted-foreground">Not found</p>
    </div>
  );
}
