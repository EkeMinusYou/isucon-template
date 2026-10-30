import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch

import run


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/redirect':
            self.send_response(302)
            self.send_header('Location', '/public')
            self.end_headers()
            return
        if self.path != '/public':
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header('Cache-Control', 'public, max-age=60')
        self.end_headers()
        self.wfile.write(b'generic response')

    def log_message(self, *args):
        pass


class ValidationTest(unittest.TestCase):
    def test_public_http_checks_observe_redirects_headers_body_and_failures(self):
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'VALIDATION_TEST_BASE': f'http://127.0.0.1:{server.server_port}'}):
            config = {'version': 1, 'http': [
                {'id': 'public', 'url': '${VALIDATION_TEST_BASE}/public', 'expect': {'status': 200,
                 'headers': {'cache-control': {'matches': 'max-age=60'}, 'x-not-present': {'absent': True}},
                 'body': {'contains': 'response', 'sha256': hashlib.sha256(b'generic response').hexdigest()}}},
                {'id': 'redirect', 'url': '${VALIDATION_TEST_BASE}/redirect', 'expect': {'status': 302}},
                {'id': 'error-response', 'url': '${VALIDATION_TEST_BASE}/missing', 'expect': {'status': 404}},
                {'id': 'mismatch', 'url': '${VALIDATION_TEST_BASE}/public', 'expect': {'status': 201,
                 'headers': {'Cache-Control': 'private'}, 'body': {'contains': 'different'}}},
                {'id': 'unset', 'url': '${NONEXISTENT_VALIDATION_TEST_ENV}/public', 'expect': {'status': 200}}
            ]}
            path = Path(directory)/'config.json'
            path.write_text(json.dumps(config))
            report = run.run(run.load_config(path), path)
            self.assertEqual(report['status'], 'failed')
            self.assertEqual([check['status'] for check in report['checks']], ['passed']*3+['failed']*2)
            self.assertEqual(len(report['checks'][3]['failures']), 3)
            self.assertNotIn(os.environ['VALIDATION_TEST_BASE'], json.dumps(report))

    def test_commands_preserve_failures_timeouts_and_redact_declared_secrets(self):
        with tempfile.TemporaryDirectory() as directory, patch.dict(os.environ, {'VALIDATION_TEST_SECRET': 'session-secret-value', 'VALIDATION_TEST_SMALL': '1'}):
            config = {'version': 1, 'commands': [
                {'id': 'redacted', 'argv': [sys.executable, '-c', 'import sys; print(sys.argv[1]); print(sys.argv[2])', '${VALIDATION_TEST_SECRET}', '${VALIDATION_TEST_SMALL}']},
                {'id': 'failure', 'argv': [sys.executable, '-c', 'import sys; print("failure",file=sys.stderr); sys.exit(7)']},
                {'id': 'timeout', 'argv': [sys.executable, '-c', 'import time; time.sleep(10)'], 'timeout_seconds': .05}
            ]}
            path=Path(directory)/'config.json'
            path.write_text(json.dumps(config))
            report=run.run(run.load_config(path), path)
            self.assertEqual(report['status'], 'failed')
            self.assertNotIn('session-secret-value', json.dumps(report))
            self.assertEqual(report['checks'][1]['exit_code'], 7)
            self.assertTrue(report['checks'][2]['timed_out'])
            self.assertEqual(report['version'], 1)

    def test_missing_configuration_and_explicit_skip_are_not_passes(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'config.json'
            for config, status in [({'version':1}, 'not_configured'),
                                   ({'version':1, 'http':[{'id':'unused', 'skip_reason':'not applicable'}]}, 'skipped')]:
                path.write_text(json.dumps(config))
                self.assertEqual(run.run(run.load_config(path),path)['status'],status)
            path.write_text(json.dumps({'version':1,'http':[{'id':'invalid','url':'http://localhost'}]}))
            with self.assertRaises(ValueError):run.load_config(path)

    def test_misspelled_expectations_do_not_silently_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)/'config.json'
            for expected in ({'status':200, 'header':{'Cache-Control':'private'}},
                             {'status':200, 'headers':{'Cache-Control':{'equal':'private'}}}):
                path.write_text(json.dumps({'version':1, 'http':[{'id':'typo', 'url':'http://localhost', 'expect':expected}]}))
                with self.assertRaises(ValueError):
                    run.load_config(path)

    def test_snapshot_does_not_promote_old_success_to_current_validation(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory)/'config.json'
            config.write_text('{"version":1}')
            previous = Path(directory)/'previous.json'
            self.assertEqual(run.snapshot(previous, config)['status'], 'not_run')
            previous.write_text(json.dumps({'version':1, 'status':'passed', 'source_commit':'old-source',
                                           'tracked_changes':[], 'config_sha256':'old-config', 'checks':[]}))
            report = run.snapshot(previous, config)
            self.assertEqual(report['status'], 'stale')
            self.assertEqual(report['previous_status'], 'passed')
            self.assertFalse(report['reexecuted_for_run'])

    def test_cli_writes_failure_report_with_private_permissions(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'config.json'
            path.write_text(json.dumps({'version':1,'commands':[{'id':'failed','argv':[sys.executable,'-c','raise SystemExit(9)']}]}))
            output=Path(directory)/'result.json'
            command=subprocess.run([sys.executable,str(Path(run.__file__).resolve()),'--config',str(path),'--output',str(output)],capture_output=True,text=True)
            self.assertEqual(command.returncode,1)
            self.assertEqual(json.loads(output.read_text())['checks'][0]['exit_code'],9)
            self.assertEqual(output.stat().st_mode & 0o777,0o600)


if __name__ == '__main__':
    unittest.main()
