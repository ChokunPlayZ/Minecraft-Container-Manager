import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import type { Server } from '../src/api/types';
import { PropertiesEditor } from '../src/components/properties-editor';

const mockServer: Server = {
  id: 'test-server',
  name: 'Test Survival World',
  server_type: 'paper',
  version: '1.21.4',
  build: '145',
  ram_mb: 8192,
  cpu_limit: 4,
  memory_limit_mb: 8192,
  host_port: 25565,
  extra_ports: [],
  container_id: 'container-test',
  state: 'running',
  backup_enabled: true,
  backup_interval_minutes: 60,
  spin_down_enabled: false,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const sampleContent = `# Minecraft server properties
server-port=25565
motd=§aTest SMP §7| §eWelcome!
max-players=100
pvp=true
difficulty=hard
custom-plugin-key=custom-value
`;

describe('PropertiesEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders properties in interactive mode with MOTD designer and mapped inputs', async () => {
    vi.spyOn(api, 'getProperties').mockResolvedValue({
      exists: true,
      content: sampleContent,
    });

    render(<PropertiesEditor server={mockServer} />);

    // Wait for properties to load
    await waitFor(() => {
      expect(screen.getByText('Server Properties')).toBeInTheDocument();
    });

    // Check that MOTD designer live preview is present
    expect(screen.getByText(/Live Minecraft Server Browser Preview/i)).toBeInTheDocument();
    expect(screen.getByText('Test Survival World')).toBeInTheDocument();

    // Check mapped fields exist
    expect(screen.getByLabelText(/Server Port/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Player vs Player/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/Game Difficulty/i)).toBeInTheDocument();

    // Check custom property section
    expect(screen.getByText('custom-plugin-key')).toBeInTheDocument();
  });

  it('switches between interactive and raw config modes', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getProperties').mockResolvedValue({
      exists: true,
      content: sampleContent,
    });

    render(<PropertiesEditor server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Server Properties')).toBeInTheDocument();
    });

    // Click "Raw Config"
    const rawButton = screen.getByRole('button', { name: /Raw Config/i });
    await user.click(rawButton);

    // Textarea should now be visible with raw content
    const textarea = screen.getByPlaceholderText(/# Minecraft server properties/i);
    expect(textarea).toBeInTheDocument();
    expect(textarea).toHaveValue(sampleContent);

    // Switch back to "Interactive"
    const interactiveButton = screen.getByRole('button', { name: /Interactive/i });
    await user.click(interactiveButton);

    expect(screen.getByText(/Live Minecraft Server Browser Preview/i)).toBeInTheDocument();
  });

  it('updates a property and saves updated content', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getProperties').mockResolvedValue({
      exists: true,
      content: sampleContent,
    });
    const saveSpy = vi.spyOn(api, 'saveProperties').mockImplementation(async (_id, content) => ({
      exists: true,
      content,
    }));

    render(<PropertiesEditor server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Server Properties')).toBeInTheDocument();
    });

    // Toggle PvP switch
    const pvpSwitch = screen.getByRole('switch', { name: /Player vs Player/i });
    expect(pvpSwitch).toHaveAttribute('aria-checked', 'true');
    await user.click(pvpSwitch);
    expect(pvpSwitch).toHaveAttribute('aria-checked', 'false');

    // Click Save Properties
    const saveButton = screen.getByRole('button', { name: /Save Properties/i });
    await user.click(saveButton);

    expect(saveSpy).toHaveBeenCalledWith('test-server', expect.stringContaining('pvp=false'));
    await waitFor(() => {
      expect(screen.getByText(/Properties saved successfully/i)).toBeInTheDocument();
    });
  });

  it('adds a new custom property', async () => {
    const user = userEvent.setup();
    vi.spyOn(api, 'getProperties').mockResolvedValue({
      exists: true,
      content: sampleContent,
    });

    render(<PropertiesEditor server={mockServer} />);

    await waitFor(() => {
      expect(screen.getByText('Server Properties')).toBeInTheDocument();
    });

    const keyInput = screen.getByPlaceholderText(/Property key/i);
    const valInput = screen.getByPlaceholderText(/^Value$/i);
    const addButton = screen.getByRole('button', { name: /^Add$/i });

    await user.type(keyInput, 'enable-voice-chat');
    await user.type(valInput, 'true');
    await user.click(addButton);

    // The new key should appear in the custom properties list
    expect(screen.getByText('enable-voice-chat')).toBeInTheDocument();
  });
});
