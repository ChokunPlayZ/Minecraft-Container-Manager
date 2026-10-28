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
SYSTEMD_ANALYZE = shutil.which('systemd-analyze')
REAL_GIT = shutil.which('git')
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
    elif args[0] == 'inspect' and os.environ.get('MCM_TEST_PANEL_CONTAINER'): print('true')
    elif args[0] == 'cp':
        if os.environ.get('MCM_TEST_COPY_FAIL'): sys.exit(1)
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
        unit = pathlib.Path(os.environ['XDG_CONFIG_HOME']) / 'systemd/user/mcm.service'
        for line in unit.read_text().splitlines():
            if line.startswith(('EnvironmentFile=', 'WorkingDirectory=')):
                if not line.split('=', 1)[1].startswith('/'):
                    print('Path directive is not absolute: ' + line, file=sys.stderr)
                    sys.exit(1)
        if not config.exists() or not binary.exists(): sys.exit(1)
        if os.environ.get('MCM_TEST_FAIL_VERSION') and os.environ['MCM_TEST_FAIL_VERSION'] in binary.read_text():
            # Model migrations performed before the new app fails to start.
            if os.environ.get('MCM_TEST_DATABASE'):
                pathlib.Path(os.environ['MCM_TEST_DATABASE']).write_text('new schema')
            sys.exit(1)
        active.touch()
elif name == 'curl':
    if os.environ.get('MCM_TEST_UNREADY_VERSION'):
        binary = pathlib.Path(os.environ['MCM_BIN_DIR']) / 'mcm'
        if os.environ['MCM_TEST_UNREADY_VERSION'] in binary.read_text(): sys.exit(22)
    sys.exit(0 if (root / 'active').exists() and (root / 'runtime-image').exists() else 22)
elif name == 'git':
    if os.environ.get('MCM_TEST_REAL_GIT'):
        os.execv(os.environ['MCM_TEST_REAL_GIT'], [os.environ['MCM_TEST_REAL_GIT'], *args])
    if 'symbolic-ref' in args:
        if os.environ.get('MCM_TEST_DETACHED'): sys.exit(1)
        print('main')
    if 'config' in args:
        if os.environ.get('MCM_TEST_NO_UPSTREAM'): sys.exit(1)
        print('origin' if args[-1].endswith('.remote') else 'refs/heads/main')
    if 'remote' in args and os.environ.get('MCM_TEST_NO_ORIGIN'): sys.exit(2)
    if 'status' in args and os.environ.get('MCM_TEST_STATUS_FAIL'): sys.exit(128)
    if 'status' in args and os.environ.get('MCM_TEST_DIRTY'): print(' M local-file')
    if 'pull' in args and os.environ.get('MCM_TEST_PULL_FAIL'): sys.exit(1)
elif name == 'sleep': pass
elif name == 'journalctl': print('test service journal')
'''


class ManagerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='mcm manager ')
        self.root = Path(self.temp.name).resolve()
        self.source = self.root / 'checkout with spaces'
        self.source.mkdir()
        shutil.copy(ROOT / 'mcmctl', self.source / 'mcmctl')
        shutil.copy(ROOT / 'Dockerfile', self.source / 'Dockerfile')
        shutil.copy(ROOT / 'go.mod', self.source / 'go.mod')
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

    def run_manager(self, *args, ok=True, script=None, cwd=None, **extra_env):
        result = subprocess.run(
            ['bash', str(script or self.source / 'mcmctl'), *args], cwd=cwd,
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

    def git(self, directory, *args):
        result = subprocess.run(
            [REAL_GIT, '-C', str(directory), *args], env=self.env,
            text=True, capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout.strip()

    def prepare_real_git(self, remote_name='origin', remote_branch='main', tracking=False):
        self.env.update({
            'MCM_TEST_REAL_GIT': REAL_GIT,
            'GIT_CONFIG_NOSYSTEM': '1',
            'GIT_CONFIG_GLOBAL': os.devnull,
            'GIT_AUTHOR_NAME': 'MCM Tests',
            'GIT_AUTHOR_EMAIL': 'mcm@example.invalid',
            'GIT_COMMITTER_NAME': 'MCM Tests',
            'GIT_COMMITTER_EMAIL': 'mcm@example.invalid',
            'GIT_TERMINAL_PROMPT': '0',
        })
        self.git(self.source, 'init', '--initial-branch=main')
        self.git(self.source, 'add', 'mcmctl', 'Dockerfile', 'go.mod')
        self.git(self.source, 'commit', '-m', 'Initial source')
        self.remote = self.root / 'remote.git'
        self.remote.mkdir()
        self.git(self.remote, 'init', '--bare', '--initial-branch=' + remote_branch)
        self.git(self.source, 'remote', 'add', remote_name, str(self.remote))
        push_args = ['push'] + (['--set-upstream'] if tracking else [])
        self.git(self.source, *push_args, remote_name, 'main:' + remote_branch)
        self.writer = self.root / 'remote writer'
        self.git(self.root, 'clone', str(self.remote), str(self.writer))
        self.run_manager('install')
        self.clear_calls()

    def advance_remote(self):
        (self.writer / 'remote-change').write_text('new source')
        self.git(self.writer, 'add', 'remote-change')
        self.git(self.writer, 'commit', '-m', 'Advance remote')
        self.git(self.writer, 'push', 'origin', 'HEAD')
        return self.git(self.writer, 'rev-parse', 'HEAD')

    def test_install_creates_config_before_start_and_embeds_frontend(self):
        self.run_manager('install', '--runtime', 'podman')
        config = self.config.read_text()
        secret = next(line.split('=', 1)[1] for line in config.splitlines() if line.startswith('MCM_SESSION_SECRET='))
        self.assertRegex(secret, r'^[a-f0-9]{64}$')
        self.assertEqual(self.config.stat().st_mode & 0o777, 0o600)
        self.assertIn('MCM_CONTAINER_RUNTIME=podman', config)
        self.assertIn('MCM_CONTAINER_HOST=unix://' + self.socket_dir.name, config)
        self.assertIn('ExecStart="' + str(self.binary) + '"', self.unit.read_text())
        self.assertIn('EnvironmentFile=' + str(self.config) + '\n', self.unit.read_text())
        self.assertIn('WorkingDirectory=' + str(Path.home()) + '\n', self.unit.read_text())
        builds = [call for call in self.calls() if call[:2] == ['podman', 'build']]
        self.assertEqual(len(builds), 1)
        self.assertIn('--target', builds[0])
        self.assertIn('go', builds[0])
        self.assertTrue((self.root / 'active').exists())

    def test_unit_paths_with_spaces_and_literal_specifiers(self):
        self.env['XDG_CONFIG_HOME'] = str(self.root / 'config %n with spaces')
        self.env['MCM_BIN_DIR'] = str(self.root / 'bin %n with spaces')
        self.config = Path(self.env['XDG_CONFIG_HOME']) / 'mcm/mcm.env'
        self.binary = Path(self.env['MCM_BIN_DIR']) / 'mcm'
        self.unit = Path(self.env['XDG_CONFIG_HOME']) / 'systemd/user/mcm.service'
        self.run_manager('install')
        unit = self.unit.read_text()
        self.assertIn('ExecStart="' + str(self.binary).replace('%', '%%') + '"\n', unit)
        self.assertIn('EnvironmentFile=' + str(self.config).replace('%', '%%') + '\n', unit)
        self.assertNotIn('EnvironmentFile="', unit)
        self.assertNotIn('WorkingDirectory="', unit)
        self.verify_with_systemd()

    def verify_with_systemd(self, expect_valid=True):
        if not SYSTEMD_ANALYZE:
            return
        # Resolve Podman's dependency without installing or starting its service.
        (self.unit.parent / 'podman.socket').write_text('[Socket]\nListenStream=/tmp/mcm-test-not-started.sock\n')
        (self.unit.parent / 'podman.service').write_text('[Service]\nExecStart=/bin/true\n')
        result = subprocess.run(
            [SYSTEMD_ANALYZE, '--user', 'verify', '--man=no', str(self.unit)],
            env={**os.environ, 'SYSTEMD_UNIT_PATH': str(self.unit.parent) + ':'},
            text=True, capture_output=True,
        )
        if expect_valid:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('not absolute', result.stderr)

    @unittest.skipUnless(SYSTEMD_ANALYZE, 'systemd-analyze is available on Linux')
    def test_generated_service_passes_real_systemd_verification(self):
        self.run_manager('install')
        self.verify_with_systemd()

    @unittest.skipUnless(SYSTEMD_ANALYZE, 'systemd-analyze is available on Linux')
    def test_documented_repair_restores_a_valid_unit(self):
        self.run_manager('install')
        original = self.unit.read_text()
        broken_lines = []
        for line in original.splitlines():
            if line.startswith(('EnvironmentFile=', 'WorkingDirectory=')):
                key, value = line.split('=', 1)
                line = key + '="' + value + '"'
            broken_lines.append(line)
        self.unit.write_text('\n'.join(broken_lines) + '\n')
        self.verify_with_systemd(expect_valid=False)
        subprocess.run([
            'sed', '-i.bak', '-E', r's/^(EnvironmentFile|WorkingDirectory)="(.*)"$/\1=\2/',
            str(self.unit),
        ], check=True)
        self.assertEqual(self.unit.read_text(), original)
        self.assertTrue(Path(str(self.unit) + '.bak').exists())
        self.verify_with_systemd()

    def test_reinstall_preserves_secret_settings_and_database(self):
        self.run_manager('install')
        original_config = self.config.read_text() + 'MCM_PORT_RANGE=31000-31100\n'
        self.config.write_text(original_config)
        database = self.root / 'data/mcm/mcm.db'
        database.write_text('existing users and servers')
        self.run_manager('install')
        self.assertEqual(self.config.read_text(), original_config)
        self.assertEqual(database.read_text(), 'existing users and servers')

    def test_repairs_legacy_unit_with_missing_environment_file(self):
        self.unit.parent.mkdir(parents=True)
        self.unit.write_text('[Service]\nExecStart=/usr/local/bin/mcm\nEnvironmentFile=%h/.config/mcm/mcm.env\n')
        self.run_manager('install', '--runtime', 'podman')
        self.assertTrue(self.config.is_file())
        self.assertIn(str(self.binary), self.unit.read_text())
        self.assertIn('ExecStart=/usr/local/bin/mcm', (self.config.parent / 'previous.service').read_text())

    def test_custom_existing_data_and_port(self):
        data = self.root / 'existing worlds'
        self.run_manager('install', '--data-dir', str(data), '--port', '9090')
        self.assertIn('MCM_DATA_DIR=' + str(data), self.config.read_text())
        self.assertIn('MCM_ADDR=:9090', self.config.read_text())
        self.assertTrue(any('http://127.0.0.1:9090/readyz' in call for call in self.calls()))

    def test_example_database_setting_uses_native_data_directory(self):
        db_setting = next(line for line in (ROOT / '.env.example').read_text().splitlines()
                          if line.startswith('MCM_DB_PATH='))
        self.config.parent.mkdir(parents=True)
        data = self.root / 'existing native data'
        data.mkdir()
        database = data / 'mcm.db'
        database.write_text('existing users and servers')
        self.config.write_text('MCM_DATA_DIR=' + str(data) + '\n' + db_setting + '\n')
        self.run_manager('install')
        self.run_manager('backup')
        backup = Path((self.config.parent / 'last-backup').read_text().strip())
        self.assertEqual((backup / 'database-path').read_text().strip(), str(database))
        self.assertEqual((backup / 'database').read_text(), 'existing users and servers')

    def test_separate_backup_media_is_created_and_preserved_on_reinstall(self):
        backups = self.root / 'backup drive/server archives'
        result = self.run_manager('install', '--backup-dir', str(backups))
        self.assertTrue(backups.is_dir())
        self.assertIn('MCM_BACKUP_DIR=' + str(backups) + '\n', self.config.read_text())
        self.assertIn('Server backups: ' + str(backups), result.stdout)
        (backups / 'archive.tar.gz').write_text('existing backup')
        config = self.config.read_text()
        self.run_manager('install')
        self.assertEqual(self.config.read_text(), config)
        self.assertEqual((backups / 'archive.tar.gz').read_text(), 'existing backup')

    def test_separate_backup_media_requires_absolute_path(self):
        result = self.run_manager('install', '--backup-dir', 'relative/backups', ok=False)
        self.assertIn('Use an absolute path', result.stderr)
        self.assertFalse(any(call[:2] == ['podman', 'build'] for call in self.calls()))

    def test_unwritable_backup_media_does_not_stop_running_panel(self):
        self.run_manager('install')
        backups = self.root / 'read only backup drive'
        backups.mkdir()
        self.config.write_text(self.config.read_text() + 'MCM_BACKUP_DIR=' + str(backups) + '\n')
        backups.chmod(0o555)
        self.clear_calls()
        try:
            result = self.run_manager('update', '--local', ok=False)
        finally:
            backups.chmod(0o755)
        self.assertIn('MCM_BACKUP_DIR is not writable', result.stderr)
        self.assertFalse(any(call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())
        self.assertTrue((self.root / 'active').exists())

    def test_custom_database_path_is_preserved(self):
        self.config.parent.mkdir(parents=True)
        database = self.root / 'separate database/panel.db'
        database.parent.mkdir()
        database.write_text('existing users and servers')
        db_setting = 'MCM_DB_PATH=' + str(database)
        self.config.write_text(db_setting + '\n')
        self.run_manager('install')
        self.assertIn(db_setting, self.config.read_text())
        self.assertEqual(database.read_text(), 'existing users and servers')
        self.run_manager('backup')
        backup = Path((self.config.parent / 'last-backup').read_text().strip())
        self.assertEqual((backup / 'database').read_text(), 'existing users and servers')

    def test_install_rejects_unwritable_database_before_build_or_start(self):
        self.config.parent.mkdir(parents=True)
        database = self.root / 'read only database/panel.db'
        database.parent.mkdir()
        database.write_text('existing database')
        for target in (database.parent, database):
            with self.subTest(target=target):
                self.config.write_text('MCM_DB_PATH=' + str(database) + '\n')
                target.chmod(0o555 if target.is_dir() else 0o444)
                try:
                    result = self.run_manager('install', ok=False)
                finally:
                    target.chmod(0o755 if target.is_dir() else 0o644)
                self.assertIn('Cannot write the SQLite database at ' + str(database), result.stderr)
                self.assertIn('leave MCM_DB_PATH empty', result.stderr)
                self.assertIn(str(self.config), result.stderr)
                self.assertFalse(any(call[:2] == ['podman', 'build'] for call in self.calls()))
                self.assertNotIn(['systemctl', '--user', 'start', 'mcm.service'], self.calls())
                self.assertEqual(database.read_text(), 'existing database')

    def test_update_rejects_unwritable_database_without_stopping_panel(self):
        self.run_manager('install')
        original_binary = self.binary.read_bytes()
        database_dir = self.root / 'read only database'
        database_dir.mkdir()
        config = self.config.read_text() + 'MCM_DB_PATH=' + str(database_dir / 'panel.db') + '\n'
        self.config.write_text(config)
        database_dir.chmod(0o555)
        self.clear_calls()
        try:
            result = self.run_manager('update', ok=False)
        finally:
            database_dir.chmod(0o755)
        self.assertIn('leave MCM_DB_PATH empty', result.stderr)
        self.assertEqual(self.binary.read_bytes(), original_binary)
        self.assertEqual(self.config.read_text(), config)
        self.assertTrue((self.root / 'active').exists())
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())
        self.assertFalse(any(call[:2] == ['podman', 'build'] or 'pull' in call for call in self.calls()))

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

    def test_binary_extraction_failure_leaves_running_version_untouched(self):
        self.run_manager('install')
        old_binary = self.binary.read_bytes()
        self.clear_calls()
        self.run_manager('update', '--local', ok=False, MCM_TEST_COPY_FAIL='1')
        self.assertEqual(self.binary.read_bytes(), old_binary)
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())
        self.assertIn(['podman', 'rm', '-f', 'build-container'], self.calls())

    def test_failed_readiness_restores_running_version(self):
        self.run_manager('install')
        result = self.run_manager('update', '--local', ok=False, MCM_TEST_VERSION='two', MCM_TEST_UNREADY_VERSION='version: two')
        self.assertIn('previous version is running', result.stderr)
        self.assertIn('version: one', self.binary.read_text())
        self.assertTrue((self.root / 'active').exists())

    def test_backup_preserves_stopped_state_and_sqlite_sidecars(self):
        self.run_manager('install')
        database = self.root / 'data/mcm/mcm.db'
        database.write_text('database')
        Path(str(database) + '-wal').write_text('wal')
        self.run_manager('stop')
        self.run_manager('backup')
        self.assertFalse((self.root / 'active').exists())
        backup = Path((self.config.parent / 'last-backup').read_text().strip())
        self.assertEqual((backup / 'database-wal').read_text(), 'wal')

    def test_invalid_port_does_not_start_or_stop_services(self):
        self.run_manager('install', '--port', 'not-a-port', ok=False)
        self.assertFalse(any(call[0] == 'systemctl' for call in self.calls()))

    def test_existing_panel_container_requires_explicit_migration(self):
        result = self.run_manager('install', ok=False, MCM_TEST_PANEL_CONTAINER='1')
        self.assertIn('existing host data directory', result.stderr)
        self.assertFalse(self.binary.exists())
        self.assertFalse(self.config.exists())

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

    def test_update_without_upstream_pulls_same_branch_from_origin(self):
        self.run_manager('install')
        self.clear_calls()
        result = self.run_manager('update', MCM_TEST_NO_UPSTREAM='1')
        self.assertIn('Updating source checkout: ' + str(self.source), result.stdout)
        self.assertIn('No upstream configured for main; using origin/main.', result.stdout)
        self.assertIn(['git', '-C', str(self.source), 'pull', '--ff-only', 'origin', 'refs/heads/main'], self.calls())

    def test_detached_checkout_is_not_pulled_built_or_stopped(self):
        self.run_manager('install')
        self.clear_calls()
        result = self.run_manager('update', ok=False, MCM_TEST_DETACHED='1')
        self.assertIn('detached HEAD', result.stderr)
        self.assertFalse(any('pull' in call or call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    def test_git_status_failure_is_not_treated_as_clean_checkout(self):
        self.run_manager('install')
        self.clear_calls()
        self.run_manager('update', ok=False, MCM_TEST_STATUS_FAIL='1')
        self.assertFalse(any('pull' in call or call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_real_git_fast_forwards_recorded_checkout_without_upstream(self):
        self.prepare_real_git()
        expected = self.advance_remote()
        # Invoke a copy elsewhere: updates must still use install.conf's checkout.
        invocation = self.root / 'other checkout'
        invocation.mkdir()
        shutil.copy(self.source / 'mcmctl', invocation / 'mcmctl')
        recorded_source = self.source
        self.source = invocation
        result = self.run_manager('update', MCM_TEST_VERSION='two')
        self.assertIn('Updating source checkout: ' + str(recorded_source), result.stdout)
        self.assertEqual(self.git(recorded_source, 'rev-parse', 'HEAD'), expected)
        self.assertIn('version: two', self.binary.read_text())

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_update_repairs_stale_nested_source_path(self):
        self.prepare_real_git()
        expected = self.advance_remote()
        nested = self.source / self.source.name
        nested.mkdir()
        self.git(nested, 'init', '--initial-branch=main')
        # The saved nested repository has no origin; the invoked checkout does.
        with (self.source / '.git/info/exclude').open('a') as exclude:
            exclude.write('\n/' + nested.name + '/\n')
        state = self.config.parent / 'install.conf'
        state.write_text('SOURCE_DIR=' + str(nested) + '\n')
        result = self.run_manager('update', MCM_TEST_VERSION='two')
        self.assertIn('replacing recorded path: ' + str(nested), result.stdout)
        self.assertEqual(self.git(self.source, 'rev-parse', 'HEAD'), expected)
        self.assertEqual(state.read_text(), 'SOURCE_DIR=' + str(self.source) + '\n')
        self.assertIn('version: two', self.binary.read_text())
        self.assertFalse(any(call[:3] == ['git', '-C', str(nested)] for call in self.calls()))
        # The installed command must use the repaired path on subsequent runs.
        result = self.run_manager('update', script=self.binary.parent / 'mcmctl')
        self.assertIn('Updating source checkout: ' + str(self.source), result.stdout)

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_installed_update_uses_recorded_source_from_another_checkout(self):
        self.prepare_real_git()
        expected = self.advance_remote()
        self.run_manager('update', script=self.binary.parent / 'mcmctl', cwd=self.writer)
        self.assertEqual(self.git(self.source, 'rev-parse', 'HEAD'), expected)
        self.assertIn(['git', '-C', str(self.source), 'pull', '--ff-only', 'origin', 'refs/heads/main'], self.calls())

    def test_local_update_builds_invoked_checkout_and_repairs_missing_source(self):
        self.run_manager('install')
        state = self.config.parent / 'install.conf'
        state.write_text('SOURCE_DIR=' + str(self.root / 'missing checkout') + '\n')
        self.clear_calls()
        self.run_manager('update', '--local')
        builds = [call for call in self.calls() if call[:2] == ['podman', 'build']]
        self.assertEqual(builds[0][-1], str(self.source))
        self.assertFalse(any(call[0] == 'git' for call in self.calls()))
        self.assertEqual(state.read_text(), 'SOURCE_DIR=' + str(self.source) + '\n')

    def test_failed_update_does_not_save_replacement_source(self):
        self.run_manager('install')
        state = self.config.parent / 'install.conf'
        original = 'SOURCE_DIR=' + str(self.root / 'previous checkout') + '\n'
        state.write_text(original)
        for failure in ('MCM_TEST_BUILD_FAIL', 'MCM_TEST_UNREADY_VERSION'):
            with self.subTest(failure=failure):
                self.run_manager('update', '--local', ok=False, MCM_TEST_VERSION='two',
                                 **{failure: 'version: two'})
                self.assertEqual(state.read_text(), original)
                self.assertIn('version: one', self.binary.read_text())
                self.assertTrue((self.root / 'active').exists())

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_missing_origin_reports_source_repair_without_build_or_stop(self):
        self.prepare_real_git()
        self.git(self.source, 'remote', 'remove', 'origin')
        result = self.run_manager('update', ok=False)
        self.assertIn('No origin remote exists in ' + str(self.source), result.stderr)
        self.assertIn('correct SOURCE_DIR in ' + str(self.config.parent / 'install.conf'), result.stderr)
        self.assertFalse(any('pull' in call or call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_real_git_respects_custom_upstream_remote_and_branch(self):
        self.prepare_real_git(remote_name='upstream', remote_branch='release', tracking=True)
        expected = self.advance_remote()
        self.run_manager('update')
        self.assertEqual(self.git(self.source, 'rev-parse', 'HEAD'), expected)

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_real_git_divergence_leaves_panel_running(self):
        self.prepare_real_git()
        self.advance_remote()
        (self.source / 'local-change').write_text('local source')
        self.git(self.source, 'add', 'local-change')
        self.git(self.source, 'commit', '-m', 'Divergent local commit')
        original_head = self.git(self.source, 'rev-parse', 'HEAD')
        original_binary = self.binary.read_bytes()
        self.run_manager('update', ok=False)
        self.assertEqual(self.git(self.source, 'rev-parse', 'HEAD'), original_head)
        self.assertEqual(self.binary.read_bytes(), original_binary)
        self.assertTrue((self.root / 'active').exists())
        self.assertFalse(any(call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

    @unittest.skipUnless(REAL_GIT, 'git is required')
    def test_real_git_missing_remote_branch_does_not_build_or_stop(self):
        self.prepare_real_git()
        self.git(self.source, 'checkout', '-b', 'local-only')
        self.run_manager('update', ok=False)
        self.assertTrue((self.root / 'active').exists())
        self.assertFalse(any(call[:2] == ['podman', 'build'] for call in self.calls()))
        self.assertNotIn(['systemctl', '--user', 'stop', 'mcm.service'], self.calls())

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
