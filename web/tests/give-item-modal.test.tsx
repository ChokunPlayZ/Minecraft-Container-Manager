import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi } from 'vitest';
import { GiveItemModal } from '../src/components/give-item-modal';

describe('GiveItemModal Component', () => {
  it('renders modal with player name and default item', () => {
    render(
      <GiveItemModal
        open={true}
        onClose={vi.fn()}
        player="Steve"
        serverVersion="1.21.4"
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText(/Give Items/i)).toBeInTheDocument();
    expect(screen.getByText('Steve')).toBeInTheDocument();
    expect(screen.getByDisplayValue('minecraft:diamond_sword')).toBeInTheDocument();
    expect(screen.getByText(/\/give Steve minecraft:diamond_sword 1/)).toBeInTheDocument();
  });

  it('autocompletes items when user types query and allows selection', async () => {
    const user = userEvent.setup();
    render(
      <GiveItemModal
        open={true}
        onClose={vi.fn()}
        player="Alex"
        serverVersion="1.21.4"
        onSubmit={vi.fn()}
      />,
    );

    const input = screen.getByPlaceholderText(/e.g. diamond_sword/i);
    await user.clear(input);
    await user.type(input, 'elytra');

    // Autocomplete dropdown should show Elytra
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Elytra/i })).toBeInTheDocument();
    });

    // Click to select Elytra
    const elytraOption = screen.getByRole('button', { name: /Elytra/i });
    await user.click(elytraOption);

    expect(input).toHaveValue('minecraft:elytra');
    expect(screen.getByText(/\/give Alex minecraft:elytra 1/)).toBeInTheDocument();
  });

  it('toggles raw NBT editor and populates with preset', async () => {
    const user = userEvent.setup();
    const handleSubmit = vi.fn().mockResolvedValue(undefined);

    render(
      <GiveItemModal
        open={true}
        onClose={vi.fn()}
        player="Steve"
        serverVersion="1.21.4"
        onSubmit={handleSubmit}
      />,
    );

    // Toggle NBT Switch
    const nbtSwitch = screen.getByRole('switch');
    await user.click(nbtSwitch);

    // Presets and textarea should now be visible
    expect(screen.getByText(/Quick Presets:/i)).toBeInTheDocument();
    const godSwordPreset = screen.getByRole('button', { name: /God Sword/i });
    expect(godSwordPreset).toBeInTheDocument();

    // Click the God Sword preset
    await user.click(godSwordPreset);

    // Textarea should be populated with Modern Component format (since 1.21.4)
    const textarea = screen.getByPlaceholderText(/enchantments=/i);
    expect(textarea).toHaveValue(
      "[enchantments={levels:{'minecraft:sharpness':5,'minecraft:fire_aspect':2,'minecraft:looting':3,'minecraft:sweeping_edge':3,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    );

    // Live preview and textarea should update
    expect(screen.getAllByText(/sharpness':5/).length).toBeGreaterThan(0);

    // Change amount to 2 using fireEvent
    const amountInput = screen.getByLabelText(/Amount/i);
    fireEvent.change(amountInput, { target: { value: '2' } });

    // Click submit
    const submitBtn = screen.getByRole('button', { name: /Give to Steve/i });
    await user.click(submitBtn);

    expect(handleSubmit).toHaveBeenCalledWith({
      item: 'minecraft:diamond_sword',
      amount: 2,
      nbt: "[enchantments={levels:{'minecraft:sharpness':5,'minecraft:fire_aspect':2,'minecraft:looting':3,'minecraft:sweeping_edge':3,'minecraft:unbreaking':3,'minecraft:mending':1}},unbreakable={}]",
    });
  });

  it('switches to legacy SNBT format and inserts legacy preset', async () => {
    const user = userEvent.setup();
    render(
      <GiveItemModal
        open={true}
        onClose={vi.fn()}
        player="Steve"
        serverVersion="1.20.1" // Legacy version
        onSubmit={vi.fn()}
      />,
    );

    // Toggle NBT
    const nbtSwitch = screen.getByRole('switch');
    await user.click(nbtSwitch);

    // Should default to Legacy SNBT because version is 1.20.1
    expect(screen.getByText('Legacy SNBT')).toBeInTheDocument();

    // Click God Sword preset
    const godSwordPreset = screen.getByRole('button', { name: /God Sword/i });
    await user.click(godSwordPreset);

    // Check textarea has legacy syntax
    const textarea = screen.getByRole('textbox', { name: '' });
    expect(textarea).toHaveValue(
      '{Enchantments:[{id:"minecraft:sharpness",lvl:5s},{id:"minecraft:fire_aspect",lvl:2s},{id:"minecraft:looting",lvl:3s},{id:"minecraft:sweeping_edge",lvl:3s},{id:"minecraft:unbreaking",lvl:3s},{id:"minecraft:mending",lvl:1s}],Unbreakable:1b}',
    );
  });

  it('shows validation error on invalid NBT', async () => {
    const user = userEvent.setup();
    render(
      <GiveItemModal
        open={true}
        onClose={vi.fn()}
        player="Steve"
        serverVersion="1.21.4"
        onSubmit={vi.fn()}
      />,
    );

    // Toggle NBT
    await user.click(screen.getByRole('switch'));

    const textarea = screen.getByPlaceholderText(/enchantments=/i);
    fireEvent.change(textarea, { target: { value: '[unclosed' } });

    expect(screen.getByText(/Must be enclosed in \[\]/i)).toBeInTheDocument();

    const submitBtn = screen.getByRole('button', { name: /Give to Steve/i });
    expect(submitBtn).toBeDisabled();
  });
});
