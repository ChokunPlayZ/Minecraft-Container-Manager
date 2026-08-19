import { createFileRoute, useNavigate } from '@tanstack/react-router';
import { useEffect } from 'react';
import { useAuth } from '../lib/auth';

export const Route = createFileRoute('/')({
  component: HomeRoute,
});

function HomeRoute() {
  const { status } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (status === 'onboarding') void navigate({ to: '/onboarding', replace: true });
    else if (status === 'unauthenticated') void navigate({ to: '/login', replace: true });
    else if (status === 'authenticated') void navigate({ to: '/dashboard', replace: true });
  }, [status, navigate]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="h-8 w-8 animate-spin rounded-full border-2 border-border border-t-primary" />
    </div>
  );
}
