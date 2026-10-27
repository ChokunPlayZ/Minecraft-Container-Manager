import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Select } from '../src/components/ui/select';
import { DialogBackdrop } from '../src/components/ui/dialog-backdrop';

describe('custom select', () => {
  it('supports grouped options, keyboard selection and disabled options', async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(<><label htmlFor="platform">Platform</label><Select id="platform" defaultValue="paper" onValueChange={onValueChange}>
      <optgroup label="Servers"><option value="paper">Paper</option><option value="retired" disabled>Retired</option><option value="fabric">Fabric</option></optgroup>
    </Select></>);
    const trigger = screen.getByRole('combobox', { name: 'Platform' });
    await user.tab();
    await user.keyboard('{Enter}');
    expect(screen.getByRole('listbox')).toBeInTheDocument();
    expect(screen.getByRole('group', { name: 'Servers' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Retired' })).toHaveAttribute('aria-disabled', 'true');
    await user.keyboard('{ArrowDown}{Enter}');
    expect(onValueChange).toHaveBeenCalledWith('fabric');
    expect(trigger).toHaveTextContent('Fabric');
    expect(trigger).toHaveFocus();
  });

  it('preserves numeric values and posts the actual value through a form', async () => {
    const user = userEvent.setup();
    function Form() {
      const [value, setValue] = useState(12);
      return <form aria-label="Page size form"><Select name="pageSize" aria-label="Page size" value={value} onValueChange={(next) => setValue(Number(next))}><option value={12}>12 players</option><option value={24}>24 players</option></Select></form>;
    }
    render(<Form />);
    await user.click(screen.getByRole('combobox', { name: 'Page size' }));
    await user.click(screen.getByRole('option', { name: '24 players' }));
    expect(new FormData(screen.getByRole('form') as HTMLFormElement).get('pageSize')).toBe('24');
    expect(screen.getByRole('combobox')).toHaveTextContent('24 players');
  });

  it('shows empty options as placeholders and accepts options loaded later', async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    const view = render(<Select aria-label="Server" value="" onValueChange={onValueChange}><option value="" disabled>Choose a server</option></Select>);
    expect(screen.getByRole('combobox')).toHaveTextContent('Choose a server');
    view.rerender(<Select aria-label="Server" value="" onValueChange={onValueChange}><option value="" disabled>Choose a server</option><><option value="survival">Survival</option></></Select>);
    await user.click(screen.getByRole('combobox'));
    await user.click(screen.getByRole('option', { name: 'Survival' }));
    expect(onValueChange).toHaveBeenCalledWith('survival');
  });

  it('keeps disabled fields closed', async () => {
    const user = userEvent.setup();
    render(<Select aria-label="Loading versions" disabled><option value="">Loading…</option></Select>);
    await user.click(screen.getByRole('combobox'));
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });

  it('lets a portaled listbox receive focus inside a dialog and handles Escape one layer at a time', async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();
    render(<DialogBackdrop onDismiss={onDismiss}><div><h2>Create server</h2><Select aria-label="Software" defaultValue="paper"><option value="paper">Paper</option><option value="fabric">Fabric</option></Select></div></DialogBackdrop>);
    await user.click(screen.getByRole('combobox'));
    expect(screen.getByRole('listbox')).toContainElement(document.activeElement as HTMLElement);
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('listbox')).not.toBeInTheDocument());
    expect(onDismiss).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toHaveFocus();
    await user.keyboard('{Escape}');
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('closes on an outside click without changing the value', async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(<><Select aria-label="Storage" defaultValue="local" onValueChange={onValueChange}><option value="local">Local</option><option value="s3">S3</option></Select><button>Outside</button></>);
    await user.click(screen.getByRole('combobox'));
    await user.click(document.documentElement);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(onValueChange).not.toHaveBeenCalled();
  });
});
