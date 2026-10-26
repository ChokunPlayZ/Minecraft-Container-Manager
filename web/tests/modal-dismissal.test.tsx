import type { ReactElement } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../src/api/client';
import * as modUpdates from '../src/api/mod-updates';
import type { ModpackManifest, Server } from '../src/api/types';
import { CopyServerDialog } from '../src/components/copy-server-dialog';
import { CreateServerDialog } from '../src/components/create-server-dialog';
import { GiveItemModal } from '../src/components/give-item-modal';
import { ModDependenciesDialog } from '../src/components/mod-dependencies-dialog';
import { ModJarPickerDialog } from '../src/components/mod-jar-picker-dialog';
import { ModUpdateDialog } from '../src/components/mod-update-dialog';
import { ModpackInstallDialog } from '../src/components/modpack-install-dialog';
import { useModal } from '../src/components/ui/modal';

const server: Server = {
  id: 'server-1',
  name: 'Test Server',
  server_type: 'paper',
  version: '1.21.1',
  build: '123',
  ram_mb: 2048,
  cpu_limit: 1,
  memory_limit_mb: 2048,
  host_port: 25565,
  extra_ports: [],
  container_id: null,
  state: 'stopped',
  backup_enabled: false,
  backup_interval_minutes: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const manifest: ModpackManifest = {
  format: 'modrinth',
  name: 'Test Modpack',
  version: '1.0',
  minecraft_version: '1.21.1',
  loader: 'paper',
  loader_version: '123',
  total_files: 1,
  server_files: 1,
  client_only_files: 0,
};

beforeEach(() => {
  vi.spyOn(api, 'javaVersions').mockResolvedValue([]);
  vi.spyOn(api, 'availablePorts').mockResolvedValue({ available: [25566], used: [] });
  vi.spyOn(api, 'listServers').mockResolvedValue([]);
  vi.spyOn(api, 'jarVersions').mockResolvedValue([{ name: '1.21.1' }]);
  vi.spyOn(api, 'jarBuilds').mockResolvedValue([]);
  vi.spyOn(api, 'getModAvailableVersions').mockResolvedValue([]);
  vi.spyOn(modUpdates, 'fetchModAvailableJars').mockResolvedValue([]);
});

afterEach(() => vi.restoreAllMocks());

const dialogs: {
  name: string;
  renderDialog: (onClose: () => void) => ReactElement;
  dismissesOnBackdrop: boolean;
}[] = [
  {
    name: 'Modpack Install',
    renderDialog: (onClose) => (
      <ModpackInstallDialog server={server} manifest={manifest} installOptions={{}} onClose={onClose} onInstalled={vi.fn()} />
    ),
    dismissesOnBackdrop: true,
  },
  {
    name: 'Copy Server',
    renderDialog: (onClose) => <CopyServerDialog server={server} open onClose={onClose} />,
    dismissesOnBackdrop: true,
  },
  {
    name: 'Mod Dependencies',
    renderDialog: (onClose) => (
      <ModDependenciesDialog isOpen primaryMod={{ title: 'Test Mod' }} dependencies={[]} onCancel={onClose} onConfirmInstall={vi.fn()} onSkipAndInstallPrimaryOnly={vi.fn()} />
    ),
    dismissesOnBackdrop: true,
  },
  {
    name: 'Mod Update',
    renderDialog: (onClose) => (
      <ModUpdateDialog isOpen modTitle="Test Mod" installedJar="old.jar" newJar="new.jar" onCancel={onClose} onConfirmDeleteAndInstall={vi.fn()} onConfirmKeepAndInstall={vi.fn()} />
    ),
    dismissesOnBackdrop: true,
  },
  {
    name: 'Mod Jar Picker',
    renderDialog: (onClose) => (
      <ModJarPickerDialog isOpen server={server} mod={{ name: 'Test Mod', file: 'mod.jar', enabled: true }} onClose={onClose} onUpdated={vi.fn()} />
    ),
    dismissesOnBackdrop: true,
  },
  {
    name: 'Give Item',
    renderDialog: (onClose) => <GiveItemModal open player="Steve" onClose={onClose} onSubmit={vi.fn()} />,
    dismissesOnBackdrop: false,
  },
];

describe.each(dialogs)('$name dismissal', ({ renderDialog, dismissesOnBackdrop }) => {
  it.each(['content', 'panel'] as const)('stays open when a drag starts on the %s and ends outside', async (start) => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    const { container } = render(renderDialog(onClose));
    await act(async () => {});
    const backdrop = container.firstElementChild as HTMLElement;
    const panel = backdrop.firstElementChild as HTMLElement;
    const target = start === 'panel' ? panel : panel.querySelector('h2, h3')!;

    await user.pointer([
      { target, keys: '[MouseLeft>]' },
      { target: backdrop },
      { keys: '[/MouseLeft]' },
    ]);

    expect(onClose).not.toHaveBeenCalled();
    expect(panel).toBeInTheDocument();
  });

  it('preserves the existing backdrop click behavior', async () => {
    const onClose = vi.fn();
    const user = userEvent.setup();
    const { container } = render(renderDialog(onClose));
    await act(async () => {});

    await user.click(container.firstElementChild as HTMLElement);

    expect(onClose).toHaveBeenCalledTimes(dismissesOnBackdrop ? 1 : 0);
  });
});

describe('Create Server dismissal', () => {
  it.each(['content', 'panel'] as const)('preserves the form when a drag starts on the %s and ends outside', async (start) => {
    const user = userEvent.setup();
    const { container } = render(<CreateServerDialog onCreated={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /create server/i }));
    const input = screen.getByLabelText(/server name/i);
    fireEvent.change(input, { target: { value: 'Unsaved server' } });
    const backdrop = container.querySelector<HTMLElement>('.fixed.inset-0')!;
    const panel = backdrop.firstElementChild as HTMLElement;

    await user.pointer([
      { target: start === 'panel' ? panel : input, keys: '[MouseLeft>]' },
      { target: backdrop },
      { keys: '[/MouseLeft]' },
    ]);

    expect(input).toBeInTheDocument();
    expect(input).toHaveValue('Unsaved server');
  });

  it('still closes when clicking the backdrop directly', async () => {
    const user = userEvent.setup();
    const { container } = render(<CreateServerDialog onCreated={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: /create server/i }));

    await user.click(container.querySelector<HTMLElement>('.fixed.inset-0')!);

    expect(screen.queryByRole('heading', { name: 'Create server' })).not.toBeInTheDocument();
  });
});

function ModalHarness({ kind, onResult }: { kind: 'confirm' | 'prompt'; onResult: (value: boolean | string | null) => void }) {
  const { confirm, prompt, dialog } = useModal();
  return (
    <>
      <button onClick={() => void (kind === 'confirm' ? confirm('Confirm action?') : prompt('Enter a name')).then(onResult)}>Open</button>
      {dialog}
    </>
  );
}

describe.each(['confirm', 'prompt'] as const)('%s modal dismissal', (kind) => {
  it('stays open after a drag out, then cancels on a direct backdrop click', async () => {
    const onResult = vi.fn();
    const user = userEvent.setup();
    render(<ModalHarness kind={kind} onResult={onResult} />);
    await user.click(screen.getByRole('button', { name: 'Open' }));
    const panel = screen.getByRole('dialog');
    const backdrop = panel.parentElement!;

    await user.pointer([
      { target: panel, keys: '[MouseLeft>]' },
      { target: backdrop },
      { keys: '[/MouseLeft]' },
    ]);

    expect(panel).toBeInTheDocument();
    expect(onResult).not.toHaveBeenCalled();

    await user.click(backdrop);

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(onResult).toHaveBeenCalledWith(kind === 'confirm' ? false : null);
  });
});
