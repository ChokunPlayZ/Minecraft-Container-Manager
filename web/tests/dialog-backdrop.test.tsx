import { useState } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { DialogBackdrop } from '../src/components/ui/dialog-backdrop';

function Harness() {
  const [open, setOpen] = useState(false);
  const [nested, setNested] = useState(false);
  return <>
    <button onClick={() => setOpen(true)}>Open workflow</button>
    {open && <DialogBackdrop onDismiss={() => setOpen(false)}>
      <div><h2>Workflow</h2><button onClick={() => setNested(true)}>Open nested</button><button onClick={() => setOpen(false)}>Cancel workflow</button></div>
    </DialogBackdrop>}
    {nested && <DialogBackdrop onDismiss={() => setNested(false)}><div><h2>Nested workflow</h2><button onClick={() => setNested(false)}>Close nested</button></div></DialogBackdrop>}
  </>;
}

describe('Dialog workflow keyboard behavior', () => {
  it('names the panel, traps Tab in both directions, and returns focus on Escape', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open workflow' });
    await user.click(trigger);
    expect(screen.getByRole('dialog', { name: 'Workflow' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open nested' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByRole('button', { name: 'Cancel workflow' })).toHaveFocus();
    await user.tab();
    expect(screen.getByRole('button', { name: 'Open nested' })).toHaveFocus();
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it('only dismisses the top dialog and keeps scrolling locked until both close', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole('button', { name: 'Open workflow' }));
    await user.click(screen.getByRole('button', { name: 'Open nested' }));
    expect(document.body.style.overflow).toBe('hidden');
    await user.keyboard('{Escape}');
    expect(screen.queryByRole('dialog', { name: 'Nested workflow' })).not.toBeInTheDocument();
    expect(screen.getByRole('dialog', { name: 'Workflow' })).toBeInTheDocument();
    expect(document.body.style.overflow).toBe('hidden');
    expect(screen.getByRole('button', { name: 'Open nested' })).toHaveFocus();
    await user.keyboard('{Escape}');
    expect(document.body.style.overflow).not.toBe('hidden');
  });

  it('keeps focus inside when a background control tries to receive focus', async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'Open workflow' });
    await user.click(trigger);
    act(() => trigger.focus());
    expect(screen.getByRole('button', { name: 'Open nested' })).toHaveFocus();
  });

  it('uses the current dismissal callback and restores scrolling on unmount', () => {
    const first = vi.fn();
    const latest = vi.fn();
    const content = <div><h2>Install</h2><button>Cancel</button></div>;
    const { rerender, unmount } = render(<DialogBackdrop onDismiss={first}>{content}</DialogBackdrop>);
    rerender(<DialogBackdrop onDismiss={latest}>{content}</DialogBackdrop>);
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(first).not.toHaveBeenCalled();
    expect(latest).toHaveBeenCalledOnce();
    unmount();
    expect(document.body.style.overflow).not.toBe('hidden');
  });
});
