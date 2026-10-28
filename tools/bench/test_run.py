"""Test benchmark finalization and optional comparison without remote access."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
BASE = "20261028-100000"
NEW = "20261028-110000"


class BenchRunTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name)
        self.log = self.path / 'calls.jsonl'
        task = self.path / 'task'
        task.write_text(r"""#!/usr/bin/env python3
import json, os, pathlib, sys
args = sys.argv[1:]
if args[0] == '--silent': args = args[1:]
name = args[0]
with open(os.environ['TEST_CALLS'], 'a') as f: f.write(json.dumps(args) + '\n')
status = 0
if name == 'compare-runs':
    stage = 'BASELINE' if 'CHECK_ONLY=true' in args else 'COMPARE'
    status = int(os.environ.get('TEST_' + stage + '_STATUS', '0'))
elif name == 'before-bench':
    run = pathlib.Path(os.environ['ISUCON_BENCH_RESULTS_DIR']) / '20261028-110000'
    run.mkdir(parents=True)
    pathlib.Path(os.environ['ISUCON_BENCH_RUN_STATE_FILE']).write_text(
        '20261028-110000\n' + str(pathlib.Path.cwd().resolve()) + '\n')
elif name == 'bench-timestamp': print('2026-10-28T11:00:00Z')
elif name == 'after-bench':
    pathlib.Path(os.environ['ISUCON_BENCH_RUN_STATE_FILE']).unlink()
    status = int(os.environ.get('TEST_FINALIZE_STATUS', '0'))
sys.exit(status)
""")
        task.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.path) + os.pathsep + os.environ['PATH'],
                        TEST_CALLS=str(self.log), ISUCON_BENCH_BASE_RUN=BASE,
                        ISUCON_BENCH_RUN_STATE_FILE=str(self.path / 'state'),
                        ISUCON_BENCH_RESULTS_DIR=str(self.path / 'runs'),
                        ISUCON_BENCH_PROFILES_ENABLED='false', TEST_BENCH_STATUS='0',
                        TEST_BASELINE_STATUS='0', TEST_FINALIZE_STATUS='0', TEST_COMPARE_STATUS='0')

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def run_bench(self, **env):
        return subprocess.run(['sh', 'tools/bench/run.sh', '--', 'sh', '-c',
                               'echo benchmark-output; exit "$TEST_BENCH_STATUS"'],
                              cwd=ROOT, env=dict(self.env, **env), text=True, capture_output=True)

    def test_success_compares_this_run_after_finalization_once(self):
        result = self.run_bench()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.calls()
        self.assertEqual(calls[0], ['compare-runs', 'CHECK_ONLY=true', f'BASE_RUN={BASE}', f'NEW_RUN={BASE}'])
        self.assertEqual(calls[-1], ['compare-runs', f'BASE_RUN={BASE}', f'NEW_RUN={NEW}'])
        self.assertEqual([c[0] for c in calls].count('after-bench'), 1)
        self.assertEqual(calls[-2][0], 'after-bench')

    def test_no_baseline_keeps_existing_behavior(self):
        result = self.run_bench(ISUCON_BENCH_BASE_RUN='')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('compare-runs', [c[0] for c in self.calls()])
        self.assertEqual([c[0] for c in self.calls()].count('after-bench'), 1)

    def test_invalid_baseline_stops_before_starting_run(self):
        result = self.run_bench(TEST_BASELINE_STATUS='17')
        self.assertEqual(result.returncode, 17)
        self.assertEqual(len(self.calls()), 1)
        self.assertFalse((self.path / 'state').exists())

    def test_bench_failure_still_finalizes_and_compares(self):
        result = self.run_bench(TEST_BENCH_STATUS='19', TEST_COMPARE_STATUS='22')
        self.assertEqual(result.returncode, 19)
        self.assertEqual([c[0] for c in self.calls()].count('after-bench'), 1)
        self.assertEqual(self.calls()[-1][-1], f'NEW_RUN={NEW}')

    def test_finalization_failure_is_preserved(self):
        result = self.run_bench(TEST_FINALIZE_STATUS='18', TEST_COMPARE_STATUS='22')
        self.assertEqual(result.returncode, 18)
        self.assertEqual([c[0] for c in self.calls()].count('after-bench'), 1)

    def test_comparison_failure_is_reported(self):
        self.assertEqual(self.run_bench(TEST_COMPARE_STATUS='22').returncode, 22)


if __name__ == '__main__':
    unittest.main()
