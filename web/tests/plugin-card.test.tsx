import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import {
  PluginCard,
  PluginCardSkeleton,
  PluginEmptyState,
  PluginGridSkeleton,
} from '../src/components/plugin-card';
import { Badge } from '../src/components/ui/badge';

describe('Shared PluginCard Component', () => {
  it('renders basic plugin information and handles install click', async () => {
    const user = userEvent.setup();
    const onInstall = vi.fn();
    const onViewDetails = vi.fn();

    render(
      <PluginCard
        id="mod-1"
        title="EssentialsX"
        author="EssentialsX Team"
        description="The essential plugin suite for Spigot and Paper servers."
        downloads={5000000}
        stars={1200}
        provider="hangar"
        installLabel="Install Latest"
        onInstall={onInstall}
        onViewDetails={onViewDetails}
      />,
    );

    expect(screen.getByText('EssentialsX')).toBeInTheDocument();
    expect(screen.getByText('EssentialsX Team')).toBeInTheDocument();
    expect(screen.getByText(/The essential plugin suite/i)).toBeInTheDocument();
    expect(screen.getByText('5M')).toBeInTheDocument();
    expect(screen.getByText('1.2K')).toBeInTheDocument();

    const installBtn = screen.getByRole('button', { name: /Install Latest/i });
    await user.click(installBtn);
    expect(onInstall).toHaveBeenCalled();

    const detailsBtn = screen.getByRole('button', { name: /Details/i });
    await user.click(detailsBtn);
    expect(onViewDetails).toHaveBeenCalled();
  });

  it('renders installed state with installed badge, jar banner, and delete button', async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn();

    render(
      <PluginCard
        id={101}
        title="LuckPerms"
        author="Luck"
        description="An advanced permissions plugin."
        provider="spiget"
        isInstalled={true}
        installedJar="LuckPerms-Bukkit-5.4.102.jar"
        installLabel="Install Jar"
        onDelete={onDelete}
      />,
    );

    // Title row installed badge & button
    expect(screen.getAllByText('Installed').length).toBeGreaterThan(0);

    // Installed jar banner
    expect(screen.getByText(/Installed: LuckPerms-Bukkit-5.4.102.jar/i)).toBeInTheDocument();

    // Reinstall button
    const reinstallBtn = screen.getByRole('button', { name: /Installed/i });
    expect(reinstallBtn).toBeInTheDocument();

    // Delete button
    const deleteBtn = screen.getByRole('button', { name: /Delete LuckPerms-Bukkit-5.4.102.jar/i });
    await user.click(deleteBtn);
    expect(onDelete).toHaveBeenCalled();
  });

  it('renders custom badges and external link', () => {
    render(
      <PluginCard
        id="modrinth-1"
        title="Sodium"
        author="jellysquid"
        description="Modern rendering engine for Minecraft."
        provider="modrinth"
        badges={
          <>
            <Badge variant="destructive">Client Only</Badge>
            <Badge variant="secondary">Optimization</Badge>
          </>
        }
        externalUrl="https://modrinth.com/mod/sodium"
        externalLabel="Modrinth"
      />,
    );

    expect(screen.getByText('Client Only')).toBeInTheDocument();
    expect(screen.getByText('Optimization')).toBeInTheDocument();

    const externalLink = screen.getByRole('link', { name: /Modrinth/i });
    expect(externalLink).toHaveAttribute('href', 'https://modrinth.com/mod/sodium');
  });

  it('renders installing loader state when isInstalling is true', () => {
    render(
      <PluginCard
        id="mod-3"
        title="Chunky"
        provider="curseforge"
        isInstalling={true}
      />,
    );

    expect(screen.getByText('Installing...')).toBeInTheDocument();
  });
});

describe('Shared PluginCardSkeleton and PluginEmptyState', () => {
  it('renders skeleton grid with specified count', () => {
    const { container } = render(<PluginGridSkeleton count={3} />);
    const skeletons = container.querySelectorAll('.animate-pulse');
    expect(skeletons.length).toBe(3);
  });

  it('renders single skeleton card', () => {
    const { container } = render(<PluginCardSkeleton />);
    expect(container.querySelector('.animate-pulse')).toBeInTheDocument();
  });

  it('renders empty state with title, description, and action', async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();

    render(
      <PluginEmptyState
        title="No plugins found"
        description="Try searching with different keywords."
        action={<button onClick={onAction}>Reset</button>}
      />,
    );

    expect(screen.getByText('No plugins found')).toBeInTheDocument();
    expect(screen.getByText('Try searching with different keywords.')).toBeInTheDocument();

    const actionBtn = screen.getByRole('button', { name: /Reset/i });
    await user.click(actionBtn);
    expect(onAction).toHaveBeenCalled();
  });
});
