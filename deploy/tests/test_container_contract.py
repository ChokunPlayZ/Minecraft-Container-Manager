"""Configuration and entrypoint contracts; no real engine or provider writes."""
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]

class ContainerContractTests(unittest.TestCase):
    def test_compose_forwards_s3_limits_and_proxy_configuration(self):
        keys = ('MCM_S3_ENDPOINT', 'MCM_S3_ACCESS_KEY', 'MCM_S3_SECRET_KEY',
                'MCM_S3_BUCKET', 'MCM_S3_REGION', 'MCM_LOGIN_MAX_ATTEMPTS',
                'MCM_LOGIN_LOCKOUT', 'MCM_RATE_LIMIT_MAX', 'MCM_RATE_LIMIT_WINDOW',
                'MCM_DEFAULT_CPU_LIMIT', 'MCM_DEFAULT_MEMORY_MB', 'MCM_TRUSTED_PROXIES')
        for name in ('docker-compose.yml', 'podman-compose.yml'):
            text = (ROOT / name).read_text()
            for key in keys:
                self.assertIn(f'      {key}: ${{{key}:-', text, (name, key))

    def test_entrypoint_grants_the_actual_mounted_socket_group(self):
        with tempfile.TemporaryDirectory(prefix='mcm-socket-') as temporary:
            root = Path(temporary)
            commands = root / 'bin'
            commands.mkdir()
            invocation = root / 'su-exec-args'
            scripts = {
                'id': '#!/bin/sh\necho 0\n',
                'chown': '#!/bin/sh\nexit 0\n',
                'stat': '#!/bin/sh\necho 1987\n',
                'su-exec': '#!/bin/sh\nprintf "%s\\n" "$@" > "$TEST_INVOCATION"\n',
            }
            for name, source in scripts.items():
                command = commands / name
                command.write_text(source)
                command.chmod(0o755)
            runtime_socket = root / 'runtime.sock'
            with socket.socket(socket.AF_UNIX) as listener:
                listener.bind(str(runtime_socket))
                env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH'],
                           MCM_DATA_DIR=str(root / 'data'), PUID='1000', PGID='1000',
                           MCM_CONTAINER_HOST='unix://' + str(runtime_socket),
                           TEST_INVOCATION=str(invocation))
                result = subprocess.run(['sh', str(ROOT / 'docker/entrypoint.sh')],
                                        env=env, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(invocation.read_text().splitlines()[:2],
                                 ['1000:1987', '/usr/local/bin/mcm'])
