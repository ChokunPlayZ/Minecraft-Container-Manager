import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ResolvedDependency } from '../src/api/mod-dependencies';
import { ModDependenciesDialog } from '../src/components/mod-dependencies-dialog';

describe('ModDependenciesDialog Component', () => {
  const primaryMod = {
    title: 'Iris Shaders',
    versionNumber: '1.7.0',
    filename: 'iris-1.7.0.jar',
    iconUrl: 'https://example.com/iris.png',
  };

  const dependencies: ResolvedDependency[] = [
    {
      id: 'dep-sodium',
      slug: 'sodium',
      title: 'Sodium',
      description: 'Modern rendering engine',
      provider: 'modrinth',
      dependencyType: 'required',
      isLibrary: true,
      alreadyInstalled: false,
      filename: 'sodium-fabric-0.5.8.jar',
      fileSize: 1048576,
    },
    {
      id: 'dep-cloth',
      slug: 'cloth-config',
      title: 'Cloth Config',
      description: 'Configuration library',
      provider: 'modrinth',
      dependencyType: 'required',
      isLibrary: true,
      alreadyInstalled: false,
      filename: 'cloth-config-10.0.jar',
      fileSize: 524288,
    },
    {
      id: 'dep-modmenu',
      slug: 'modmenu',
      title: 'Mod Menu',
      description: 'Adds a mod menu to Minecraft',
      provider: 'modrinth',
      dependencyType: 'optional',
      isLibrary: false,
      alreadyInstalled: false,
      filename: 'modmenu-7.0.1.jar',
      fileSize: 204800,
    },
    {
      id: 'dep-fabric-api',
      slug: 'fabric-api',
      title: 'Fabric API',
      description: 'Core API library',
      provider: 'modrinth',
      dependencyType: 'required',
      isLibrary: true,
      alreadyInstalled: true,
      installedFile: 'fabric-api-0.92.0.jar',
    },
  ];

  it('renders primary mod details and detected dependencies with appropriate badges', () => {
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText('Mod Dependencies Detected')).toBeInTheDocument();
    expect(screen.getByText(/requires additional library mods to function properly on your server/)).toBeInTheDocument();

    // Primary Mod Card
    expect(screen.getByText('Primary Mod')).toBeInTheDocument();
    expect(screen.getByText('iris-1.7.0.jar')).toBeInTheDocument();

    // Dependencies
    expect(screen.getByText('Sodium')).toBeInTheDocument();
    expect(screen.getByText('Cloth Config')).toBeInTheDocument();
    expect(screen.getByText('Mod Menu')).toBeInTheDocument();

    // Badges
    const requiredBadges = screen.getAllByText('Required');
    expect(requiredBadges.length).toBeGreaterThanOrEqual(2);

    const libraryBadges = screen.getAllByText('Library');
    expect(libraryBadges.length).toBeGreaterThanOrEqual(2);

    expect(screen.getByText('Optional')).toBeInTheDocument();

    // Already Satisfied section
    expect(screen.getByText('Already Satisfied (1)')).toBeInTheDocument();
  });

  it('checks required/library dependencies by default and updates count on toggle', () => {
    const onConfirm = vi.fn();
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={onConfirm}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    // Sodium and Cloth Config are required libraries, so checked by default (2 dependencies + 1 primary = 3 mods)
    const installBtn = screen.getByRole('button', { name: /Install 3 Mods/i });
    expect(installBtn).toBeInTheDocument();

    // Toggle Mod Menu (optional) to also install it
    const modMenuCheckbox = screen.getByRole('checkbox', { name: 'Install Mod Menu' });
    expect(modMenuCheckbox).not.toBeChecked();

    fireEvent.click(modMenuCheckbox);
    expect(modMenuCheckbox).toBeChecked();

    // Now 3 dependencies + 1 primary = 4 mods
    expect(screen.getByRole('button', { name: /Install 4 Mods/i })).toBeInTheDocument();

    // Click confirm
    fireEvent.click(screen.getByRole('button', { name: /Install 4 Mods/i }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    const selected = onConfirm.mock.calls[0][0] as ResolvedDependency[];
    expect(selected).toHaveLength(3);
    expect(selected.map((s) => s.title)).toEqual(expect.arrayContaining(['Sodium', 'Cloth Config', 'Mod Menu']));
  });

  it('supports Deselect all and Select all buttons', () => {
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    // Click Deselect all
    fireEvent.click(screen.getByRole('button', { name: 'Deselect all' }));
    expect(screen.getByRole('button', { name: /Install Iris Shaders/i })).toBeInTheDocument();

    // Click Select all
    fireEvent.click(screen.getByRole('button', { name: 'Select all' }));
    expect(screen.getByRole('button', { name: /Install 4 Mods/i })).toBeInTheDocument();
  });

  it('triggers onSkipAndInstallPrimaryOnly when user skips dependencies', () => {
    const onSkip = vi.fn();
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={onSkip}
        onCancel={vi.fn()}
      />,
    );

    const skipBtn = screen.getByRole('button', { name: /Skip & Install Only Iris Shaders/i });
    fireEvent.click(skipBtn);
    expect(onSkip).toHaveBeenCalledTimes(1);
  });

  it('triggers onCancel when user cancels or closes modal', () => {
    const onCancel = vi.fn();
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={onCancel}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('shows satisfied dependencies details when toggled', () => {
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.queryByText('(fabric-api-0.92.0.jar)')).not.toBeInTheDocument();
    fireEvent.click(screen.getByText('Already Satisfied (1)'));
    expect(screen.getByText('(fabric-api-0.92.0.jar)')).toBeInTheDocument();
  });

  it('renders installation progress when isInstalling is true', () => {
    render(
      <ModDependenciesDialog
        isOpen={true}
        primaryMod={primaryMod}
        dependencies={dependencies}
        isInstalling={true}
        installProgress={{
          current: 2,
          total: 3,
          currentName: 'Sodium',
        }}
        onConfirmInstall={vi.fn()}
        onSkipAndInstallPrimaryOnly={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByText('Installing Mods (2 of 3)...')).toBeInTheDocument();
    expect(screen.getByText('Downloading: Sodium')).toBeInTheDocument();
  });
});
