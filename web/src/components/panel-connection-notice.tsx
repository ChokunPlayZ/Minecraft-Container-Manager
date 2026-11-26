import { useEffect, useState } from 'react';
import { WifiOff } from 'lucide-react';
import { api } from '../api/client';

const CHECK_INTERVAL_MS = 15_000;
const CHECK_TIMEOUT_MS = 5_000;

export function PanelConnectionNotice() {
  const [disconnected, setDisconnected] = useState(false);

  useEffect(() => {
    let active = true;
    let probe: AbortController | undefined;
    let timeout: ReturnType<typeof setTimeout> | undefined;

    async function check() {
      if (probe) return;
      const controller = new AbortController();
      probe = controller;
      timeout = setTimeout(() => controller.abort(), CHECK_TIMEOUT_MS);
      const reachable = await api.panelHealth(controller.signal).then(
        (health) => health?.status === 'ok',
        () => false,
      );
      clearTimeout(timeout);
      probe = undefined;
      if (active) setDisconnected(!reachable);
    }

    const interval = setInterval(() => void check(), CHECK_INTERVAL_MS);
    return () => {
      active = false;
      clearInterval(interval);
      clearTimeout(timeout);
      probe?.abort();
    };
  }, []);

  if (!disconnected) return null;

  return (
    <div role="alert" className="pointer-events-none fixed right-4 bottom-4 left-4 z-[120] flex items-start gap-3 rounded-lg border border-destructive/40 bg-popover px-4 py-3 text-sm text-popover-foreground shadow-lg sm:left-auto">
      <WifiOff aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
      <div>
        <p className="font-semibold">MCM Not reachable</p>
        <p className="mt-0.5 text-muted-foreground">check internet connection or if your VPN is connected</p>
      </div>
    </div>
  );
}
