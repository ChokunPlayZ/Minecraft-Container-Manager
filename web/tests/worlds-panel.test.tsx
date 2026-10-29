import { renderWithAdmin as render } from '../src/test/auth';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { WorldsPanel } from '../src/components/worlds-panel';
import { worldApi, type World } from '../src/api/worlds';
import { ApiError } from '../src/api/client';

const saved: World = { name: 'survival', active: true, generated: true, seed: '-9223372036854775807', level_type: 'minecraft:normal', generate_structures: true, size_bytes: 16384, modified_at: '2026-10-27T12:00:00Z', metadata_error: false, version: '1.21.4', dimensions: [{ id: 'survival', name: 'Overworld', regions: 1 }] };
function setup(worlds: World[] = [saved], editable = true) {
  vi.spyOn(worldApi, 'list').mockResolvedValue({ worlds, active_world: 'survival', editable });
  return userEvent.setup();
}
afterEach(() => vi.restoreAllMocks());
describe('WorldsPanel', () => {
  it('creates a world with a full precision seed and keeps input after failure', async () => {
    const user = setup([]);
    const create = vi.spyOn(worldApi, 'create').mockRejectedValueOnce(new ApiError(409, 'This folder already exists.')).mockResolvedValue({ ...saved, name: 'new-world', generated: false, active: false });
    render(<WorldsPanel serverId="one" state="stopped" serverType="paper" />);
    await user.click(await screen.findByRole('button', { name: 'Create world' }));
    await user.type(screen.getByLabelText('World folder name'), 'new-world');
    await user.type(screen.getByLabelText('Seed'), '-9223372036854775807');
    await user.click(screen.getByLabelText('World generator'));
    await user.click(screen.getByRole('option', { name: 'Amplified' }));
    const form = screen.getByRole('form', { name: 'Create world' });
    await user.click(within(form).getByRole('button', { name: 'Create world' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('This folder already exists.');
    expect(screen.getByLabelText('Seed')).toHaveValue('-9223372036854775807');
    await user.click(within(form).getByRole('button', { name: 'Create world' }));
    await waitFor(() => expect(create).toHaveBeenLastCalledWith('one', { name: 'new-world', seed: '-9223372036854775807', level_type: 'minecraft:amplified', generate_structures: true }));
    expect(await screen.findByRole('status')).toHaveTextContent('Created “new-world”');
  });
  it('uploads a selected ZIP under its own world name', async () => {
    const user = setup(); const upload = vi.spyOn(worldApi, 'import').mockResolvedValue({ ...saved, name: 'imported', active: false });
    render(<WorldsPanel serverId="one" state="stopped" serverType="paper" />);
    await user.click(await screen.findByRole('button', { name: 'Import ZIP' }));
    await user.type(screen.getByLabelText('World folder name'), 'imported');
    const file = new File(['zip'], 'my-world.zip', { type: 'application/zip' });
    await user.upload(screen.getByLabelText('World ZIP file'), file);
    expect(screen.getByLabelText('World folder name')).toHaveValue('imported');
    expect(screen.getByLabelText('World folder name')).toBeValid();
    expect((screen.getByLabelText('World ZIP file') as HTMLInputElement).files).toHaveLength(1);
    await user.click(screen.getByRole('button', { name: 'Import world' }));
    await waitFor(() => expect(upload).toHaveBeenCalledWith('one', 'imported', file));
    expect(await screen.findByRole('status')).toHaveTextContent('Imported “imported”');
  });
  it('confirms switching worlds and prevents deleting the active world', async () => {
    const other = { ...saved, name: 'creative', active: false };
    const user = setup([saved, other]); const activate = vi.spyOn(worldApi, 'activate').mockResolvedValue({ ...other, active: true });
    render(<WorldsPanel serverId="one" state="stopped" serverType="paper" />);
    expect(await screen.findByText('-9223372036854775807')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete world' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: /creative/ }));
    await user.click(screen.getByRole('button', { name: 'Make active' }));
    expect(activate).not.toHaveBeenCalled();
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Make active' }));
    await waitFor(() => expect(activate).toHaveBeenCalledWith('one', 'creative'));
    expect(await screen.findByRole('status')).toHaveTextContent('next server start');
  });
  it('selects a negative-coordinate chunk and sends only that chunk after confirmation', async () => {
    const user = setup(); vi.spyOn(worldApi, 'regions').mockResolvedValue({ regions: [{ x: -1, z: 0, chunks: 2, size_bytes: 16384 }], total: 1, offset: 0 });
    vi.spyOn(worldApi, 'chunks').mockResolvedValue({ chunks: [{ x: -32, z: 0, timestamp: 123 }, { x: -31, z: 0, timestamp: 456 }] });
    const reset = vi.spyOn(worldApi, 'reset').mockResolvedValue({ reset: 1, recovery_path: '.mcm-world-backups/recovery' });
    render(<WorldsPanel serverId="one" state="stopped" serverType="paper" />);
    await user.click(await screen.findByRole('button', { name: 'Manage chunks' }));
    const chunk = await screen.findByRole('gridcell', { name: 'Chunk -32, 0' });
    await user.click(screen.getByText('Select an area by chunk coordinates'));
    await user.click(screen.getByRole('button', { name: 'Select area' }));
    expect(screen.getByText('(-32, 0)', { exact: true })).toBeInTheDocument();
    expect(chunk).toHaveAttribute('aria-selected', 'true');
    await user.click(chunk);
    expect(chunk).toHaveAttribute('aria-selected', 'false');
    await user.keyboard(' ');
    expect(chunk).toHaveAttribute('aria-selected', 'true');
    await user.keyboard('{ArrowRight}'); expect(screen.getByRole('gridcell', { name: 'Chunk -31, 0' })).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Reset selected (1)' }));
    expect(reset).not.toHaveBeenCalled();
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Reset 1 chunk' }));
    await waitFor(() => expect(reset).toHaveBeenCalledWith('one', 'survival', { dimension: 'survival', region_x: -1, region_z: 0, chunks: [0] }));
    expect(await screen.findByRole('status')).toHaveTextContent('.mcm-world-backups/recovery');
  });
  it('gates mutation on server state and keeps generation settings off existing terrain', async () => {
    setup([saved], false); render(<WorldsPanel serverId="one" state="running" serverType="paper" />);
    await screen.findByText('-9223372036854775807');
    expect(screen.getByRole('button', { name: 'Create world' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Export ZIP' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Generation settings' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Manage chunks' })).toBeEnabled();
  });
  it('offers a retry after a failed load and explains unsupported server types', async () => {
    const list = vi.spyOn(worldApi, 'list').mockRejectedValueOnce(new ApiError(500, 'Storage unavailable')).mockResolvedValue({ worlds: [saved], active_world: 'survival', editable: true });
    const user = userEvent.setup(); const view = render(<WorldsPanel serverId="one" state="stopped" serverType="paper" />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Storage unavailable');
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    await screen.findByText('-9223372036854775807'); expect(list).toHaveBeenCalledTimes(2);
    view.unmount(); render(<WorldsPanel serverId="proxy" state="stopped" serverType="velocity" />);
    expect(screen.getByText(/does not store Java worlds/)).toBeInTheDocument(); expect(list).toHaveBeenCalledTimes(2);
  });
});
