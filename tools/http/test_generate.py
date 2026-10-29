import gzip
import tempfile
from pathlib import Path
import unittest

from generate import prepare_static


class StaticGenerationTest(unittest.TestCase):
    def test_rebuild_preserves_original_and_replaces_stale_variants(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            source, output = root / 'source', root / 'output'
            source.mkdir()
            (source / 'app.js').write_bytes(b'const value = 1;\n' * 100)
            prepare_static(source, output)
            first = (output / 'app.js.gz').read_bytes()
            self.assertEqual(gzip.decompress(first), (source / 'app.js').read_bytes())
            prepare_static(source, output)
            self.assertEqual(first, (output / 'app.js.gz').read_bytes())
            (source / 'app.js').unlink()
            prepare_static(source, output)
            self.assertFalse((output / 'app.js.gz').exists())

    def test_rejects_overlapping_trees_and_symlinks(self):
        with tempfile.TemporaryDirectory() as root:
            root = Path(root)
            with self.assertRaises(ValueError):
                prepare_static(root, root / 'output')
            (root / 'link').symlink_to('/etc/passwd')
            with self.assertRaises(ValueError):
                prepare_static(root, root.with_name(root.name + '-output'))

class HostGenerationTest(unittest.TestCase):
    def test_host_override_and_stale_config_cleanup(self):
        import json
        import os
        import subprocess
        import sys
        script = Path(__file__).with_name('generate.py').resolve()
        with tempfile.TemporaryDirectory() as root:
            env = dict(os.environ, ISUCON_APP_HOSTS='h1 h2', ISUCON_ALL_HOSTS='h1 h2',
                       ISUCON_APP_PORT='8080', ISUCON_ACCESS_LOG_PATH='/home/isucon/access.log',
                       ISUCON_STATIC_SOURCE=str(Path(root) / 'public'),
                       ISUCON_STATIC_OUTPUT=str(Path(root) / 'generated/public'),
                       ISUCON_HTTP_HOST_CONFIG='{"h2":{"address":":443","tls_cert_file":"/cert.pem","tls_key_file":"/key.pem"}}')
            subprocess.run([sys.executable, str(script)], env=env, cwd=root, check=True)
            first = json.loads((Path(root) / 'generated/http/h1.json').read_text())
            second = json.loads((Path(root) / 'generated/http/h2.json').read_text())
            self.assertEqual(first['max_connections'], 32768)
            self.assertEqual(second['address'], ':443')
            env.update(ISUCON_APP_HOSTS='h1', ISUCON_HTTP_HOST_CONFIG='{}')
            subprocess.run([sys.executable, str(script)], env=env, cwd=root, check=True)
            self.assertFalse((Path(root) / 'generated/http/h2.json').exists())
