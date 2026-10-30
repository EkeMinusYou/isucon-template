#!/usr/bin/env python3
"""Run declared HTTP expectations and contest tests without contest semantics."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import ssl
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

ENV = re.compile(r'\$\{([A-Za-z_][A-Za-z0-9_]*)\}')
MAX_BODY = 2 * 1024 * 1024
MAX_LOG = 64 * 1024


def load_config(path):
    config = json.loads(Path(path).read_text())
    if not isinstance(config, dict) or config.get('version') != 1 or set(config) - {'version', 'http', 'commands'}:
        raise ValueError('expected version 1 with http and commands declarations')
    ids = set()
    for kind in ('http', 'commands'):
        entries = config.get(kind, [])
        if not isinstance(entries, list):
            raise ValueError(f'{kind} must be a list')
        for entry in entries:
            if not isinstance(entry, dict):
                raise ValueError(f'{kind}: each check must be an object')
            allowed = {'id', 'skip_reason', 'timeout_seconds'} | ({'argv', 'cwd'} if kind == 'commands' else {'url', 'method', 'headers', 'request_body', 'ca_file', 'expect'})
            if set(entry) - allowed:
                raise ValueError(f'{kind}: unknown check fields')
            if 'skip_reason' in entry and (not isinstance(entry['skip_reason'], str) or not entry['skip_reason'].strip()):
                raise ValueError('skip_reason must be a nonempty string')
            name = entry.get('id')
            if not isinstance(name, str) or not re.fullmatch(r'[A-Za-z0-9_.-]+', name) or name in ids:
                raise ValueError('each check requires a unique, nonempty identifier')
            ids.add(name)
            timeout = entry.get('timeout_seconds', 15 if kind == 'http' else 60)
            if not isinstance(timeout, (int, float)) or not 0 < timeout <= 3600:
                raise ValueError(f'{name}: timeout_seconds must be within (0,3600]')
            if entry.get('skip_reason'):
                continue
            if kind == 'commands':
                argv = entry.get('argv')
                if not isinstance(argv, list) or not argv or any(not isinstance(a, str) for a in argv):
                    raise ValueError(f'{name}: argv must be a nonempty string array')
            else:
                if not isinstance(entry.get('url'), str) or not entry['url']:
                    raise ValueError(f'{name}: url is required')
                expected = entry.get('expect', {})
                if not isinstance(expected, dict) or set(expected) - {'status', 'headers', 'body'}:
                    raise ValueError(f'{name}: unknown expectation fields')
                if not isinstance(expected.get('headers', {}), dict):
                    raise ValueError(f'{name}: expected headers must be a mapping')
                status = expected.get('status')
                if not isinstance(status, int) or not 100 <= status <= 599:
                    raise ValueError(f'{name}: an expected HTTP status is required')
                if not isinstance(entry.get('headers', {}), dict) or any(not isinstance(v, str) for v in entry.get('headers', {}).values()):
                    raise ValueError(f'{name}: request headers must be a string mapping')
                for condition in expected.get('headers', {}).values():
                    if isinstance(condition, str):
                        continue
                    if not isinstance(condition, dict) or len(condition) != 1:
                        raise ValueError(f'{name}: header condition requires equals, matches, or absent')
                    key, value = next(iter(condition.items()))
                    if key not in ('equals', 'matches', 'absent') or (key == 'absent' and value is not True):
                        raise ValueError(f'{name}: invalid header condition')
                    if key != 'absent' and not isinstance(value, str):
                        raise ValueError(f'{name}: header comparison must use a string')
                    if key == 'matches':
                        re.compile(value)
                body = expected.get('body', {})
                if not isinstance(body, dict) or set(body) - {'contains', 'sha256'}:
                    raise ValueError(f'{name}: body supports contains and sha256')
                if 'contains' in body and not isinstance(body['contains'], str):
                    raise ValueError(f'{name}: body contains must be a string')
                if 'sha256' in body and (not isinstance(body['sha256'], str) or not re.fullmatch('[a-fA-F0-9]{64}', body['sha256'])):
                    raise ValueError(f'{name}: body sha256 must be a hex digest')
    return config


def expand(value):
    def replace(match):
        name = match.group(1)
        if name not in os.environ:
            raise ValueError(f'missing environment variable: {name}')
        return os.environ[name]
    return ENV.sub(replace, value)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def check_http(entry):
    headers = {name: expand(value) for name, value in entry.get('headers', {}).items()}
    body = entry.get('request_body')
    request = urllib.request.Request(expand(entry['url']),
                                    data=expand(body).encode() if body is not None else None,
                                    headers=headers, method=entry.get('method', 'GET'))
    ca_file = expand(entry['ca_file']) if entry.get('ca_file') else None
    opener = urllib.request.build_opener(NoRedirect(), urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=ca_file)))
    try:
        response = opener.open(request, timeout=entry.get('timeout_seconds', 15))
    except urllib.error.HTTPError as error:
        response = error
    with response:
        failures = []
        expected = entry['expect']
        if response.code != expected['status']:
            failures.append('HTTP status mismatch')
        for name, condition in expected.get('headers', {}).items():
            value = response.headers.get(name)
            if isinstance(condition, str):
                matched = value == condition
            elif 'absent' in condition:
                matched = value is None
            elif 'equals' in condition:
                matched = value == condition['equals']
            else:
                matched = value is not None and re.search(condition['matches'], value) is not None
            if not matched:
                failures.append(f'header mismatch: {name}')
        # Header-only checks do not wait for an arbitrary streaming response body.
        body_checks = expected.get('body', {})
        if body_checks:
            content = response.read(MAX_BODY + 1)
            if len(content) > MAX_BODY:
                failures.append('response body exceeds verification limit')
            else:
                if 'contains' in body_checks and expand(body_checks['contains']).encode() not in content:
                    failures.append('body content mismatch')
                if 'sha256' in body_checks and hashlib.sha256(content).hexdigest() != body_checks['sha256'].lower():
                    failures.append('body SHA-256 mismatch')
        return {'status': 'failed' if failures else 'passed', 'http_status': response.code, 'failures': failures}


def check_command(entry):
    argv = [expand(arg) for arg in entry['argv']]
    cwd = expand(entry.get('cwd', '.'))
    with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
        process = subprocess.Popen(argv, cwd=cwd, stdout=stdout, stderr=stderr, start_new_session=True)
        timed_out = False
        try:
            process.wait(timeout=entry.get('timeout_seconds', 60))
        except subprocess.TimeoutExpired:
            timed_out = True
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.wait()
        result = {'status': 'passed' if process.returncode == 0 and not timed_out else 'failed',
                  'exit_code': process.returncode, 'timed_out': timed_out}
        for name, stream in (('stdout', stdout), ('stderr', stderr)):
            stream.seek(0)
            data = stream.read(MAX_LOG + 1)
            result[name] = data[:MAX_LOG].decode(errors='replace')
            result[name + '_truncated'] = len(data) > MAX_LOG
        return result


def run(config, config_path):
    results = []
    for kind in ('http', 'commands'):
        for entry in config.get(kind, []):
            started = time.monotonic()
            if entry.get('skip_reason'):
                result = {'status': 'skipped', 'reason': entry['skip_reason']}
            else:
                try:
                    result = check_http(entry) if kind == 'http' else check_command(entry)
                except Exception as error:
                    # HTTP exceptions may embed credential-bearing request URLs.
                    result = {'status': 'failed', 'error_type': type(error).__name__}
                    if isinstance(error, ValueError) and str(error).startswith('missing environment variable:'):
                        result['reason'] = str(error)
            results.append({'id': entry['id'], 'kind': kind, 'duration_seconds': round(time.monotonic()-started, 3), **result})
    status = 'not_configured'
    if any(r['status'] == 'failed' for r in results):
        status = 'failed'
    elif any(r['status'] == 'passed' for r in results):
        status = 'passed'
    elif results:
        status = 'skipped'
    commit = subprocess.run(['git', 'rev-parse', 'HEAD'], capture_output=True, text=True)
    dirty = subprocess.run(['git', 'diff', '--name-only', 'HEAD'], capture_output=True, text=True)
    report = {'version': 1, 'status': status, 'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'source_commit': commit.stdout.strip() if commit.returncode == 0 else None,
              'tracked_changes': dirty.stdout.splitlines() if dirty.returncode == 0 else None,
              'config_sha256': hashlib.sha256(Path(config_path).read_bytes()).hexdigest(), 'checks': results}
    # Redact string values only; short environment values must not corrupt JSON.
    values = [os.environ[name] for name in set(ENV.findall(json.dumps(config))) if os.environ.get(name)]
    def redact(value):
        if isinstance(value, str):
            for secret in sorted(values, key=len, reverse=True):
                value = value.replace(secret, '[REDACTED]')
            return value
        if isinstance(value, list):
            return [redact(item) for item in value]
        if isinstance(value, dict):
            return {key: redact(item) for key, item in value.items()}
        return value
    for check in report['checks']:
        for key in ('stdout', 'stderr'):
            if key in check:
                check[key] = redact(check[key])
    return report



def snapshot(previous, config_path):
    if not Path(previous).is_file():
        return {'version': 1, 'status': 'not_run', 'checks': []}
    report = json.loads(Path(previous).read_text())
    current_commit = subprocess.run(['git', 'rev-parse', 'HEAD'], capture_output=True, text=True)
    changes = subprocess.run(['git', 'diff', '--name-only', 'HEAD'], capture_output=True, text=True)
    same_source = (current_commit.returncode == 0 and changes.returncode == 0
                   and report.get('source_commit') == current_commit.stdout.strip()
                   and report.get('tracked_changes') == [] and not changes.stdout.strip())
    same_config = report.get('config_sha256') == hashlib.sha256(Path(config_path).read_bytes()).hexdigest()
    report['reexecuted_for_run'] = False
    if report.get('status') == 'passed' and not (same_source and same_config):
        report['previous_status'] = report['status']
        report['status'] = 'stale'
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', default='tools/contest/validation.json')
    parser.add_argument('--output', default='generated/validation/latest.json')
    parser.add_argument('--check-config', action='store_true')
    parser.add_argument('--snapshot', help='copy previous results into a RUN without executing checks')
    args = parser.parse_args()
    try:
        config = load_config(args.config)
    except (ValueError, OSError, TypeError, re.error) as error:
        parser.error(str(error))
    if args.check_config:
        print('validation declarations: valid')
        return
    report = snapshot(args.output, args.config) if args.snapshot else run(config, args.config)
    output = Path(args.snapshot or args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode='w', dir=output.parent, delete=False) as temporary:
        json.dump(report, temporary, ensure_ascii=False, indent=2)
        temporary.write('\n')
    Path(temporary.name).replace(output)
    print(f"validation: {report['status']} ({len(report['checks'])} checks); {output}")
    for check in report['checks']:
        print(f"  {check['id']}: {check['status']}")
    raise SystemExit(1 if report['status'] == 'failed' else 0)


if __name__ == '__main__':
    main()
