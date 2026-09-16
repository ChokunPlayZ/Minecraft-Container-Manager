import { act, fireEvent, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { ConsoleLine } from '../src/api/types';
import { ConsoleViewer } from '../src/components/console-viewer';

describe('ConsoleViewer', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('does not pull the viewer down when they have scrolled up', () => {
    let receiveLine: ((line: ConsoleLine) => void) | undefined;
    vi.spyOn(api, 'openConsoleStream').mockImplementation((_serverId, onLine) => {
      receiveLine = onLine;
      return vi.fn();
    });

    const { container } = render(<ConsoleViewer serverId="server-1" running />);
    const consoleElement = container.querySelector('.overflow-y-auto') as HTMLDivElement;

    Object.defineProperties(consoleElement, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 300 },
    });

    consoleElement.scrollTop = 200;
    fireEvent.scroll(consoleElement);

    act(() => receiveLine?.({ timestamp: '', message: 'A new log line' }));

    expect(consoleElement.scrollTop).toBe(200);
  });

  it('continues following new output while the viewer is at the bottom', () => {
    let receiveLine: ((line: ConsoleLine) => void) | undefined;
    vi.spyOn(api, 'openConsoleStream').mockImplementation((_serverId, onLine) => {
      receiveLine = onLine;
      return vi.fn();
    });

    const { container } = render(<ConsoleViewer serverId="server-1" running />);
    const consoleElement = container.querySelector('.overflow-y-auto') as HTMLDivElement;

    Object.defineProperties(consoleElement, {
      scrollHeight: { configurable: true, value: 1000 },
      clientHeight: { configurable: true, value: 300 },
    });

    consoleElement.scrollTop = 700;
    fireEvent.scroll(consoleElement);
    consoleElement.scrollTop = 650;

    act(() => receiveLine?.({ timestamp: '', message: 'A new log line' }));

    expect(consoleElement.scrollTop).toBe(1000);
  });

  it('preserves lines and does not close stream when server stops or errors out', () => {
    let receiveLine: ((line: ConsoleLine) => void) | undefined;
    const closeSpy = vi.fn();
    const openSpy = vi.spyOn(api, 'openConsoleStream').mockImplementation((_serverId, onLine) => {
      receiveLine = onLine;
      return closeSpy;
    });

    const { container, rerender } = render(<ConsoleViewer serverId="server-1" running />);
    expect(openSpy).toHaveBeenCalledTimes(1);

    act(() => {
      receiveLine?.({ timestamp: '', message: 'Line before crash' });
      receiveLine?.({ timestamp: '', message: 'Encountered an unexpected exception' });
    });

    expect(container.textContent).toContain('Line before crash');
    expect(container.textContent).toContain('Encountered an unexpected exception');

    // Server transitions from running -> stopped/error
    rerender(<ConsoleViewer serverId="server-1" running={false} />);

    // Stream should NOT have been closed or re-opened
    expect(closeSpy).not.toHaveBeenCalled();
    expect(openSpy).toHaveBeenCalledTimes(1);

    // Lines should still be present in the console viewer
    expect(container.textContent).toContain('Line before crash');
    expect(container.textContent).toContain('Encountered an unexpected exception');
  });

  it('renders repeated lines without suppressing them', () => {
    let receiveLine: ((line: ConsoleLine) => void) | undefined;
    vi.spyOn(api, 'openConsoleStream').mockImplementation((_serverId, onLine) => {
      receiveLine = onLine;
      return vi.fn();
    });

    const { container } = render(<ConsoleViewer serverId="server-1" running />);

    act(() => {
      receiveLine?.({ timestamp: '', message: 'Warning: overloaded' });
      receiveLine?.({ timestamp: '', message: 'Warning: overloaded' });
    });

    const allMatches = container.querySelectorAll('.whitespace-pre-wrap');
    expect(allMatches.length).toBe(2);
    expect(allMatches[0].textContent).toContain('Warning: overloaded');
    expect(allMatches[1].textContent).toContain('Warning: overloaded');
  });
});
