import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '../src/api/client';
import type { ServerLogContent, ServerLogFile } from '../src/api/types';
import { LogBrowser } from '../src/components/log-browser';

const logFile = (path: string, archive = false): ServerLogFile => ({
  path, name: path.split('/').pop()!, archive, size_bytes: 4096, modified_at: '2026-10-26T12:00:00Z',
});
const content = (path: string, text: string): ServerLogContent => ({
  path, content: text, truncated: false, limit_bytes: 2 * 1024 * 1024,
});

afterEach(() => vi.restoreAllMocks());

describe('LogBrowser', () => {
  it('opens the current log, filters case-insensitively, and preserves original line numbers', async () => {
    vi.spyOn(api, 'listServerLogs').mockResolvedValue({ files: [logFile('logs/latest.log')] });
    const read = vi.spyOn(api, 'readServerLog').mockResolvedValue(content('logs/latest.log', 'Server ready\nWARN: overloaded\nPlayer joined\n'));
    const user = userEvent.setup();
    render(<LogBrowser serverId="server-1" />);
    expect(await screen.findByText('Server ready')).toBeInTheDocument();
    expect(read).toHaveBeenCalledWith('server-1', 'logs/latest.log', undefined);
    await user.type(screen.getByRole('textbox', { name: 'Search saved log' }), 'warn');
    expect(screen.queryByText('Server ready')).not.toBeInTheDocument();
    expect(screen.getByText('WARN: overloaded').previousElementSibling).toHaveTextContent('2');
    expect(screen.getByText(/1 matching \/ 3 lines/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Clear log search' }));
    expect(screen.getByText('Server ready')).toBeInTheDocument();
  });

  it('reads compressed logs and lets the user select members of a tarball', async () => {
    const user = userEvent.setup();
    const list = vi.spyOn(api, 'listServerLogs').mockImplementation(async (_id, archive) => ({ files: archive
      ? [logFile('old/first.log'), logFile('old/second.log.gz')]
      : [logFile('logs/2026-10-25-1.log.gz'), logFile('logs/history.tar.gz', true)],
    }));
    const read = vi.spyOn(api, 'readServerLog').mockImplementation(async (_id, path, entry) => content(path, `Contents of ${entry ?? path}`));
    render(<LogBrowser serverId="server-1" />);
    expect(await screen.findByText('Contents of logs/2026-10-25-1.log.gz')).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Log file'), 'logs/history.tar.gz');
    expect(await screen.findByText('Contents of old/first.log')).toBeInTheDocument();
    expect(list).toHaveBeenCalledWith('server-1', 'logs/history.tar.gz');
    await user.selectOptions(screen.getByLabelText('Log inside archive'), 'old/second.log.gz');
    expect(await screen.findByText('Contents of old/second.log.gz')).toBeInTheDocument();
    expect(read).toHaveBeenLastCalledWith('server-1', 'logs/history.tar.gz', 'old/second.log.gz');
  });

  it('ignores responses from a previously selected log', async () => {
    vi.spyOn(api, 'listServerLogs').mockResolvedValue({ files: [logFile('logs/latest.log'), logFile('logs/old.log.gz')] });
    let resolveOld: (value: ServerLogContent) => void = () => {};
    vi.spyOn(api, 'readServerLog').mockImplementation(async (_id, path) => path === 'logs/latest.log'
      ? new Promise<ServerLogContent>((resolve) => { resolveOld = resolve; })
      : content(path, 'Correct selected log'));
    render(<LogBrowser serverId="server-1" />);
    await waitFor(() => expect(screen.getByLabelText('Log file')).toHaveValue('logs/latest.log'));
    fireEvent.change(screen.getByLabelText('Log file'), { target: { value: 'logs/old.log.gz' } });
    expect(await screen.findByText('Correct selected log')).toBeInTheDocument();
    await act(async () => resolveOld(content('logs/latest.log', 'Stale log content')));
    expect(screen.queryByText('Stale log content')).not.toBeInTheDocument();
    expect(screen.getByText('Correct selected log')).toBeInTheDocument();
  });

  it('pages large previews and labels the search limit', async () => {
    vi.spyOn(api, 'listServerLogs').mockResolvedValue({ files: [logFile('logs/latest.log')] });
    vi.spyOn(api, 'readServerLog').mockResolvedValue({ ...content('logs/latest.log', Array.from({ length: 300 }, (_, i) => `Log line ${i + 1}`).join('\n')), truncated: true });
    const user = userEvent.setup();
    render(<LogBrowser serverId="server-1" />);
    expect(await screen.findByText('Log line 250')).toBeInTheDocument();
    expect(screen.queryByText('Log line 251')).not.toBeInTheDocument();
    expect(screen.getByText(/Showing the first 2 MB. Search covers this preview./)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Next log page' }));
    expect(screen.getByText('Log line 251')).toBeInTheDocument();
    expect(screen.queryByText('Log line 250')).not.toBeInTheDocument();
    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument();
  });

  it('shows empty directories and archives clearly', async () => {
    const list = vi.spyOn(api, 'listServerLogs').mockResolvedValue({ files: [] });
    const { unmount } = render(<LogBrowser serverId="server-1" />);
    expect(await screen.findByText('No saved logs yet')).toBeInTheDocument();
    unmount();
    list.mockImplementation(async (_id, path) => ({ files: path ? [] : [logFile('logs/empty.tar.gz', true)] }));
    render(<LogBrowser serverId="server-2" />);
    expect(await screen.findByText('This archive contains no readable .log or .txt files.')).toBeInTheDocument();
  });

  it('recovers from a damaged archive when refreshed', async () => {
    vi.spyOn(api, 'listServerLogs').mockResolvedValue({ files: [logFile('logs/old.log.gz')] });
    const read = vi.spyOn(api, 'readServerLog').mockRejectedValueOnce(new ApiError(400, 'This compressed log is damaged.'))
      .mockResolvedValue(content('logs/old.log.gz', 'Recovered log'));
    const user = userEvent.setup();
    render(<LogBrowser serverId="server-1" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('This compressed log is damaged.');
    await user.click(screen.getByRole('button', { name: 'Refresh logs' }));
    expect(await screen.findByText('Recovered log')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(read).toHaveBeenCalledTimes(2);
  });
});
