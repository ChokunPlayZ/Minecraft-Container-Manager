import { api } from './src/api/client';
import type { Server } from './src/api/types';
localStorage.setItem('mcm-theme', window.location.hash.includes('dark') ? 'dark' : 'light');
const server: Server = { id: 'demo', name: 'Survival SMP', server_type: 'paper', version: '1.21.4', build: '145', ram_mb: 4096, cpu_limit: 2, memory_limit_mb: 4096, host_port: 25565, extra_ports: [], container_id: 'demo', state: 'stopped', backup_enabled: true, backup_interval_minutes: 60, created_at: '2026-10-01T00:00:00Z', updated_at: '2026-10-26T12:00:00Z' };
api.me = async () => ({ id: 'demo', email: 'admin@example.com', totp_enabled: false });
api.getServer = async () => server;
api.serverStatus = async () => ({ state: 'stopped' });
api.getServerDNS = async () => ({ record: null, join_address: '' });
api.serverStats = async () => ({ server_id: 'demo', online: false, disk_bytes: 2831155200 });
api.openConsoleStream = () => () => {};
const file = (path: string, archive = false) => ({ path, name: path.split('/').pop()!, size_bytes: 148889, modified_at: '2026-10-26T12:00:00Z', archive });
api.listServerLogs = async (_id, archive) => ({ files: archive ? [file('logs/2026-10-25-1.log'), file('logs/2026-10-25-2.log')] : [file('logs/latest.log'), file('logs/2026-10-25-1.log.gz'), file('logs/2026-10-archive.tar.gz', true)] });
api.readServerLog = async (_id, path, entry) => ({ path, entry, truncated: false, limit_bytes: 2097152, content: ['[19:32:01] [Server thread/INFO]: Starting minecraft server version 1.21.4', '[19:32:01] [Server thread/INFO]: Loading properties', '[19:32:02] [Server thread/INFO]: Preparing level "world"', '[19:32:05] [Server thread/INFO]: Done (3.247s)! For help, type "help"', '[19:34:12] [Server thread/INFO]: Alex joined the game', '[19:34:14] [Server thread/INFO]: Alex[/192.168.1.20:58411] logged in with entity id 127 at ([world]124.5, 64.0, -392.2)', '[19:36:30] [Server thread/WARN]: Can\'t keep up! Is the server overloaded? Running 2100ms or 42 ticks behind', '[19:36:32] [Server thread/INFO]: Saving the game (this may take a moment!)', '[19:36:32] [Server thread/INFO]: Saved the game', '[19:40:00] [Server thread/INFO]: Stopping the server', '[19:40:00] [Server thread/INFO]: Saving players', '[19:40:01] [Server thread/INFO]: Saving worlds', '[19:40:01] [Server thread/INFO]: All dimensions are saved'].join('\n') });
window.history.replaceState({}, '', '/servers/demo?mock=true');
await import('./src/client');
let archiveSelected = false;
const timer = setInterval(() => {
  const logs = document.getElementById('server-tab-logs');
  if (logs?.getAttribute('aria-selected') !== 'true') logs?.click();
  const select = document.getElementById('server-log-file') as HTMLSelectElement | null;
  if (select?.options.length === 3 && !archiveSelected) {
    archiveSelected = true;
    select.value = 'logs/2026-10-archive.tar.gz';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    clearInterval(timer);
  }
}, 100);
