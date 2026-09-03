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
});
