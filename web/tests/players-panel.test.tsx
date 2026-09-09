import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Op, Player, Server } from '../src/api/types';
import { PlayersPanel } from '../src/components/players-panel';

const mockServer: Server = {
  id: 'test-server-id',
  name: 'Mega SMP Server',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 8192,
  cpu_limit: 4,
  memory_limit_mb: 8192,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-123',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  spin_down_enabled: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

// Generate 100 mock players: Player_001 to Player_100
function generateManyPlayers(count: number): Player[] {
  const players: Player[] = [];
  for (let i = 1; i <= count; i++) {
    const pad = String(i).padStart(3, '0');
    players.push({ name: `Player_${pad}` });
  }
  return players;
}

const mockOps: Op[] = [
  { uuid: 'uuid-1', name: 'Player_001', level: 4, bypassesPlayerLimit: true },
  { uuid: 'uuid-2', name: 'Player_005', level: 4 },
];

describe('PlayersPanel with Large Player Counts', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders a server with 100 online players with pagination', async () => {
    const manyPlayers = generateManyPlayers(100);
    vi.spyOn(api, 'players').mockResolvedValue({
      players: manyPlayers,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: mockOps });

    render(<PlayersPanel server={mockServer} />);

    // Wait for players to load
    await waitFor(() => {
      expect(screen.getByText(/100 players online/i)).toBeInTheDocument();
    });

    // Check pagination status (24 per page by default: 1-24 of 100)
    expect(screen.getByText(/Showing 1–24 of 100/i)).toBeInTheDocument();
    expect(screen.getByText(/Page 1 of 5/i)).toBeInTheDocument();

    // Verify first page has Player_001 through Player_024
    expect(screen.getByText('Player_001')).toBeInTheDocument();
    expect(screen.getByText('Player_024')).toBeInTheDocument();
    // Player_025 should be on page 2, not page 1
    expect(screen.queryByText('Player_025')).not.toBeInTheDocument();
  });

  it('filters 100 players instantly using the search input', async () => {
    const user = userEvent.setup();
    const manyPlayers = generateManyPlayers(100);
    vi.spyOn(api, 'players').mockResolvedValue({
      players: manyPlayers,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: mockOps });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/100 players online/i)).toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/Search players.../i);
    await user.type(searchInput, 'Player_042');

    // Only Player_042 should be shown
    expect(screen.getByText('Player_042')).toBeInTheDocument();
    expect(screen.queryByText('Player_001')).not.toBeInTheDocument();
    expect(screen.getByText(/Showing 1 of 100 players/i)).toBeInTheDocument();

    // Clear search
    const clearBtn = screen.getByLabelText(/Clear search/i);
    await user.click(clearBtn);

    expect(screen.getByText('Player_001')).toBeInTheDocument();
  });

  it('sorts players ascending and descending', async () => {
    const user = userEvent.setup();
    const manyPlayers = generateManyPlayers(10);
    vi.spyOn(api, 'players').mockResolvedValue({
      players: manyPlayers,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Player_001')).toBeInTheDocument();
    });

    // Change sort to Z-A
    const sortSelect = screen.getByDisplayValue('Name (A-Z)');
    await user.selectOptions(sortSelect, 'desc');

    // Now Player_010 should appear before Player_001
    const names = screen.getAllByText(/Player_0/i).map((el) => el.textContent);
    expect(names[0]).toContain('Player_010');
  });

  it('navigates through pages with Next and Previous buttons', async () => {
    const user = userEvent.setup();
    const manyPlayers = generateManyPlayers(50);
    vi.spyOn(api, 'players').mockResolvedValue({
      players: manyPlayers,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/Showing 1–24 of 50/i)).toBeInTheDocument();
    });

    // Go to next page
    const nextBtn = screen.getByLabelText(/Next page/i);
    await user.click(nextBtn);

    expect(screen.getByText(/Showing 25–48 of 50/i)).toBeInTheDocument();
    expect(screen.getByText('Player_025')).toBeInTheDocument();
    expect(screen.queryByText('Player_001')).not.toBeInTheDocument();

    // Go back to previous page
    const prevBtn = screen.getByLabelText(/Previous page/i);
    await user.click(prevBtn);

    expect(screen.getByText(/Showing 1–24 of 50/i)).toBeInTheDocument();
    expect(screen.getByText('Player_001')).toBeInTheDocument();
  });

  it('changes page size to display all players', async () => {
    const user = userEvent.setup();
    const manyPlayers = generateManyPlayers(60);
    vi.spyOn(api, 'players').mockResolvedValue({
      players: manyPlayers,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText(/Showing 1–24 of 60/i)).toBeInTheDocument();
    });

    const pageSizeSelect = screen.getByDisplayValue('24');
    await user.selectOptions(pageSizeSelect, '9999');

    expect(screen.getByText(/Showing 1–60 of 60/i)).toBeInTheDocument();
    expect(screen.getByText('Player_060')).toBeInTheDocument();
  });

  it('toggles between Grid View and List View', async () => {
    const user = userEvent.setup();
    const players = generateManyPlayers(5);
    vi.spyOn(api, 'players').mockResolvedValue({
      players,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Player_001')).toBeInTheDocument();
    });

    // Click list view button
    const listViewBtn = screen.getByLabelText(/List view/i);
    await user.click(listViewBtn);

    // List view should be active
    const gridViewBtn = screen.getByLabelText(/Grid view/i);
    await user.click(gridViewBtn);
  });

  it('filters by Operator role', async () => {
    const user = userEvent.setup();
    const players = generateManyPlayers(10);
    vi.spyOn(api, 'players').mockResolvedValue({
      players,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: mockOps }); // Player_001 and Player_005 are ops

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Player_001')).toBeInTheDocument();
    });

    // Select Operators role filter
    const roleSelect = screen.getByDisplayValue('All Roles');
    await user.selectOptions(roleSelect, 'ops');

    expect(screen.getByText('Player_001')).toBeInTheDocument();
    expect(screen.getByText('Player_005')).toBeInTheDocument();
    expect(screen.queryByText('Player_002')).not.toBeInTheDocument();
  });

  it('opens command action modal and executes command', async () => {
    const user = userEvent.setup();
    const players = [{ name: 'Steve' }];
    vi.spyOn(api, 'players').mockResolvedValue({
      players,
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });
    const runCommandSpy = vi.spyOn(api, 'runPlayerCommand').mockResolvedValue({ ok: true, response: 'Success' });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Steve')).toBeInTheDocument();
    });

    // Click quick "Kick" button
    const kickBtn = screen.getByRole('button', { name: 'Kick' });
    await user.click(kickBtn);

    // Modal dialog should open
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText(/Target: Steve/i)).toBeInTheDocument();

    const reasonInput = screen.getByPlaceholderText(/e.g. Inappropriate behavior/i);
    await user.type(reasonInput, 'Testing kick command');

    const confirmBtn = screen.getByRole('button', { name: /Confirm Action/i });
    await user.click(confirmBtn);

    expect(runCommandSpy).toHaveBeenCalledWith(
      'test-server-id',
      'Steve',
      'kick',
      expect.objectContaining({ reason: 'Testing kick command' }),
    );

    // Modal closes and notice appears
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(screen.getByText(/Successfully ran Kick on Steve/i)).toBeInTheDocument();
    });
  });

  it('switches between Online Players, Operators, and Whitelist sub-tabs', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'players').mockResolvedValue({
      players: [{ name: 'Steve' }],
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: mockOps });
    vi.spyOn(api, 'whitelist').mockResolvedValue({ whitelist: [] });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Steve')).toBeInTheDocument();
    });

    // Click Operators sub-tab
    const opsTabBtn = screen.getByRole('button', { name: /Operators/i });
    await user.click(opsTabBtn);

    expect(screen.getByText('Grant or revoke server operator status.')).toBeInTheDocument();

    // Click Whitelist sub-tab
    const whitelistTabBtn = screen.getByRole('button', { name: /Whitelist/i });
    await user.click(whitelistTabBtn);

    expect(screen.getByText('Control which players are allowed to join.')).toBeInTheDocument();
  });

  it('opens GiveItemModal from player row and gives pre-enchanted item', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'players').mockResolvedValue({
      players: [{ name: 'Steve' }],
      source: 'rcon',
    });
    vi.spyOn(api, 'ops').mockResolvedValue({ ops: [] });
    const runCommandSpy = vi.spyOn(api, 'runPlayerCommand').mockResolvedValue({ ok: true, response: 'Success' });

    render(<PlayersPanel server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Steve')).toBeInTheDocument();
    });

    // Click Actions dropdown menu
    const actionsBtn = screen.getByRole('button', { name: 'Commands for Steve' });
    await user.click(actionsBtn);

    // Click Give Items in dropdown
    const giveOption = screen.getByRole('menuitem', { name: /Give Items/i });
    await user.click(giveOption);

    // Give Item Modal opens
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText(/Give Items/i)).toBeInTheDocument();

    // Toggle NBT switch
    const nbtSwitch = screen.getByRole('switch');
    await user.click(nbtSwitch);

    // Click God Sword preset
    const godSwordPreset = screen.getByRole('button', { name: /God Sword/i });
    await user.click(godSwordPreset);

    // Click Give button
    const submitBtn = screen.getByRole('button', { name: /Give to Steve/i });
    await user.click(submitBtn);

    expect(runCommandSpy).toHaveBeenCalledWith(
      'test-server-id',
      'Steve',
      'give',
      expect.objectContaining({
        item: 'minecraft:diamond_sword',
        amount: 1,
        nbt: expect.stringContaining("sharpness':5"),
      }),
    );

    // Modal closes and success banner appears
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(screen.getByText(/Successfully gave 1x minecraft:diamond_sword to Steve/i)).toBeInTheDocument();
    });
  });
});
