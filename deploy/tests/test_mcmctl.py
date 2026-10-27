"""Exercise installation and update transactions without a real service manager.

Run: python3 -m unittest discover -s deploy/tests -v
"""
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
STUB = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
root = pathlib.Path(os.environ['MCM_TEST_ROOT'])
with (root / 'calls').open('a') as log:
    log.write(json.dumps([name, *args]) + '\n')
if name == 'uname': print('Linux')
elif name == 'id': print('tester' if '-un' in args else '1000')
elif name == 'flock': pass
elif name == 'loginctl': print('yes')
elif name in ('podman', 'docker'):
    if args[0] == 'build':
        if os.environ.get('MCM_TEST_BUILD_FAIL'): sys.exit(1)
        (root / 'built-version').write_text(os.environ.get('MCM_TEST_VERSION', 'one'))
    elif args[0] == 'create': print('build-container')
    elif args[0] == 'cp':
        pathlib.Path(args[-1]).write_text('#!/bin/sh\n# version: ' + (root / 'built-version').read_text() + '\n')
    elif args[:2] == ['image', 'inspect']:
        sys.exit(0 if (root / 'runtime-image').exists() else 1)
    elif args[0] == 'pull': (root / 'runtime-image').touch()
elif name == 'systemctl':
    cmd = args[1]
    active = root / 'active'
    if cmd == 'is-active': sys.exit(0 if active.exists() else 3)
    if cmd == 'stop' or (cmd == 'disable' and '--now' in args): active.unlink(missing_ok=True)
    if cmd in ('start', 'restart'):
        binary = pathlib.Path(os.environ['MCM_BIN_DIR']) / 'mcm'
        config = pathlib.Path(os.environ['XDG_CONFIG_HOME']) / 'mcm/mcm.env'
        if not config.exists() or not binary.exists(): sys.exit(1)
        if os.environ.get('MCM_TEST_FAIL_VERSION') and os.environ['MCM_TEST_FAIL_VERSION'] in binary.read_text():
            # Model migrations performed before the new app fails to start.
            if os.environ.get('MCM_TEST_DATABASE'):
                pathlib.Path(os.environ['MCM_TEST_DATABASE']).write_text('new schema')
            sys.exit(1)
        active.touch()
elif name == 'curl':
    sys.exit(0 if (root / 'active').exists() and (root / 'runtime-image').exists() else 22)
elif name == 'git':
    if 'status' in args and os.environ.get('MCM_TEST_DIRTY'): print(' M local-file')
    if 'pull' in args and os.environ.get('MCM_TEST_PULL_FAIL'): sys.exit(1)
elif name == 'sleep': pass
elif name == 'journalctl': print('test service journal')
'''


class ManagerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='mcm manager ')
        self.root = Path(self.temp.name)
        self.source = self.root / 'checkout with spaces'
        self.source.mkdir()
        shutil.copy(ROOT / 'mcmctl', self.source / 'mcmctl')
        shutil.copy(ROOT / 'Dockerfile', self.source / 'Dockerfile')
        self.stubbin = self.root / 'stubs'
        self.stubbin.mkdir()
        for name in ('uname', 'id', 'flock', 'loginctl', 'podman', 'docker', 'systemctl', 'curl', 'git', 'sleep', 'journalctl'):
            path = self.stubbin / name
            path.write_text(STUB.replace('#!/usr/bin/env python3', '#!' + shutil.which('python3')))
            path.chmod(0o755)
        self.env = {
            **os.environ,
            'PATH': str(self.stubbin) + os.pathsep + os.environ['PATH'],
            'MCM_TEST_ROOT': str(self.root),
            'MCM_BIN_DIR': str(self.root / 'bin'),
            'XDG_CONFIG_HOME': str(self.root / 'config'),
            'XDG_DATA_HOME': str(self.root / 'data'),
            'XDG_RUNTIME_DIR': str(self.root / 'run'),
        }
        # AF_UNIX paths are length-limited; keep the runtime socket separate.
        self.socket_dir = tempfile.TemporaryDirectory(prefix='mcm-sock-')
        self.env['XDG_RUNTIME_DIR'] = self.socket_dir.name
        socket_path = Path(self.socket_dir.name) / 'podman/podman.sock'
        socket_path.parent.mkdir()
        self.socket = socket.socket(socket.AF_UNIX)
        self.socket.bind(str(socket_path))
        self.config = Path(self.env['XDG_CONFIG_HOME']) / 'mcm/mcm.env'
        self.binary = Path(self.env['MCM_BIN_DIR']) / 'mcm'
        self.unit = Path(self.env['XDG_CONFIG_HOME']) / 'systemd/user/mcm.service'

    def tearDown(self):
        self.socket.close()
        self.socket_dir.cleanup()
        self.temp.cleanup()

    def run_manager(self, *args, ok=True, **extra_env):
        result = subprocess.run(
            ['bash', str(self.source / 'mcmctl'), *args],
            env={**self.env, **extra_env}, text=True, capture_output=True,
        )
        if ok:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def calls(self):
        path = self.root / 'calls'
        return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []

    def clear_calls(self):
        (self.root / 'calls').write_text('')

    def test_install_creates_config_before_start_and_embeds_frontend(self):
        self.run_manager('install', '--runtime', 'podman')
        config = self.config.read_text()
        secret = next(line.split('=', 1)[1] for line in config.splitlines() if line.startswith('MCM_SESSION_SECRET='))
        self.assertRegex(secret, r'^[a-f0-9]{64}$')
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o600)
        self.assertIn('MCM_CONTAINER_RUNTIME=podman', config)
        self.assertIn('MCM_CONTAINER_HOST=unix://' + self.socket_dir.name, config)
        self.assertIn('ExecStart="' + str(self.binary) + '"', self.unit.read_text())
        self.assertIn('EnvironmentFile="' + str(self.config) + '"', self.unit.read_text())
        builds = [call for call in self.calls() if call[:2] == ['podman', 'build']]
        self.assertEqual(len(builds), 1)
        self.assertIn('--target', builds[0])
        self.assertIn('go', builds[0])
        self.assertTrue((self.root / 'active').exists())

    def test_reinstall_preserves_secret_settings_and_database(self):
        self.run_manager('install')
        original_config = self.config.read_text() + 'MCM_PORT_RANGE=31000-31100\n'
        self.config.write_text(original_config)
        database = self.root / 'data/mcm/mcm.db'
        database.write_text('existing users and servers')
        self.run_manager('install')
        self.assertEqual(self.config.read_text(), original_config)
        self.assertEqual(database.read_text(), 'existing users and servers')

    def test_custom_existing_data_and_port(self):
        data = self.root / 'existing worlds'
        self.run_manager('install', '--data-dir', str(data), '--port', '9090')
        self.assertIn('MCM_DATA_DIR=' + str(data), self.config.read_text())
        self.assertIn('MCM_ADDR=:9090', self.config.read_text())
        self.assertTrue(any('http://127.0.0.1:9090/readyz' in call for call in self.calls()))

    def test_docker_install_does_not_require_podman_socket(self):
        self.run_manager('install', '--runtime', 'docker')
        self.assertIn('MCM_CONTAINER_RUNTIME=docker', self.config.read_text())
        self.assertNotIn('podman.socket', self.unit.read_text())
        self.assertFalse(any(call[0] == 'podman' for call in self.calls()))

    def test_build_failure_leaves_running_version_untouched(self):
        self.run_manager('install')
        old_binary = self.binary.read_bytes()
        self.clear_calls()
        self.run_manager('update', '--local', ok=False, MCM_TEST_BUILD_FAIL='1')
        self.assertEqual(self.binary.read_bytes(), old_binary)
        self.assertTrue((self.root / 'active').exists())
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    def test_update_preserves_config_and_supports_rollback(self):
        self.run_manager('install')
        old_config = self.config.read_bytes()
        database = self.root / 'data/mcm/mcm.db'
        database.write_text('old schema')
        self.run_manager('update', '--local', MCM_TEST_VERSION='two')
        self.assertIn('version: two', self.binary.read_text())
        self.assertEqual(self.config.read_bytes(), old_config)
        self.run_manager('rollback')
        self.assertIn('version: one', self.binary.read_text())
        self.assertEqual(database.read_text(), 'old schema')

    def test_failed_start_restores_binary_and_pre_migration_database(self):
        self.run_manager('install')
        database = self.root / 'data/mcm/mcm.db'
        database.write_text('old schema')
        result = self.run_manager(
            'update', '--local', ok=False, MCM_TEST_VERSION='two',
            MCM_TEST_FAIL_VERSION='version: two', MCM_TEST_DATABASE=str(database),
        )
        self.assertIn('previous version is running', result.stderr)
        self.assertIn('version: one', self.binary.read_text())
        self.assertEqual(database.read_text(), 'old schema')
        self.assertTrue((self.root / 'active').exists())

    def test_dirty_checkout_is_not_pulled_or_stopped(self):
        self.run_manager('install')
        self.clear_calls()
        self.run_manager('update', ok=False, MCM_TEST_DIRTY='1')
        self.assertFalse(any('pull' in call for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    def test_update_uses_fast_forward(self):
        self.run_manager('install')
        self.clear_calls()
        self.run_manager('update')
        self.assertIn(['git', '-C', str(self.source), 'pull', '--ff-only'], self.calls())

    def test_uninstall_keeps_settings_database_and_worlds(self):
        self.run_manager('install')
        data = self.root / 'data/mcm'
        (data / 'world').mkdir()
        (data / 'world/level.dat').write_text('world')
        self.run_manager('uninstall')
        self.assertFalse(self.binary.exists())
        self.assertFalse(self.unit.exists())
        self.assertTrue(self.config.exists())
        self.assertEqual((data / 'world/level.dat').read_text(), 'world')

    def test_secret_content_is_not_executed(self):
        self.config.parent.mkdir(parents=True)
        marker = self.root / 'must not exist'
        self.config.write_text('MCM_SESSION_SECRET=$(touch "' + str(marker) + '")\n')
        self.run_manager('install')
        self.assertFalse(marker.exists())

    def test_runtime_switch_requires_updating_explicit_host_config(self):
        self.run_manager('install', '--runtime', 'podman')
        self.run_manager('install', '--runtime', 'docker', ok=False)
        self.assertIn('MCM_CONTAINER_RUNTIME=podman', self.config.read_text())


if __name__ == '__main__':
    unittest.main()
