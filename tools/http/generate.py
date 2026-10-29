"""Generate host-specific Go HTTP configuration and precompressed static assets."""
import gzip
import json
import os
from pathlib import Path
import shutil
import tempfile


def prepare_static(source, target):
    source, target = Path(source).resolve(), Path(target).resolve()
    if source == target or target in source.parents or source in target.parents:
        raise ValueError('static source and generated output must be separate trees')
    target.parent.mkdir(parents=True, exist_ok=True)
    stage = Path(tempfile.mkdtemp(prefix='.static-', dir=target.parent))
    try:
        if source.exists():
            for file in source.rglob('*'):
                if file.is_symlink():
                    raise ValueError(f'static symlinks are unsupported: {file}')
                if not file.is_file() or any(part.startswith('.') for part in file.relative_to(source).parts):
                    continue
                if file.suffix == '.gz':
                    raise ValueError(f'provide original static assets, not gzip sidecars: {file}')
                output = stage / file.relative_to(source)
                output.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(file, output)
                data = file.read_bytes()
                compressed = gzip.compress(data, compresslevel=9, mtime=0)
                if len(compressed) < len(data):
                    sidecar = output.with_name(output.name + '.gz')
                    sidecar.write_bytes(compressed)
                    os.utime(sidecar, ns=(file.stat().st_atime_ns, file.stat().st_mtime_ns))
        if target.exists():
            shutil.rmtree(target)
        stage.rename(target)
    finally:
        if stage.exists():
            shutil.rmtree(stage)


def main():
    hosts = os.environ['ISUCON_APP_HOSTS'].split()
    all_hosts = os.environ['ISUCON_ALL_HOSTS'].split()
    overrides = json.loads(os.environ.get('ISUCON_HTTP_HOST_CONFIG', '{}'))
    routes = json.loads(os.environ.get('ISUCON_HTTP_STATIC_ROUTES', '[]'))
    if not hosts or not set(hosts) <= set(all_hosts) or not set(overrides) <= set(hosts):
        raise ValueError('HTTP hosts must belong to APP_HOSTS and ALL_HOSTS')
    default = dict(address=':' + os.environ['ISUCON_APP_PORT'],
                   measurement_address='127.0.0.1:6060',
                   access_log_path=os.environ['ISUCON_ACCESS_LOG_PATH'],
                   tls_cert_file=os.environ.get('ISUCON_TLS_CERT_FILE', ''),
                   tls_key_file=os.environ.get('ISUCON_TLS_KEY_FILE', ''),
                   max_connections=int(os.environ.get('ISUCON_MAX_CONNECTIONS', '32768')),
                   static_routes=routes)
    output = Path('generated/http')
    configs = {}
    for host in hosts:
        if '/' in host or host in {'.', '..'}:
            raise ValueError('invalid host name')
        override = overrides.get(host, {})
        if not set(override) <= set(default):
            raise ValueError(f'unknown HTTP setting for {host}')
        config = dict(default, **override)
        if bool(config['tls_cert_file']) != bool(config['tls_key_file']) or config['max_connections'] < 1:
            raise ValueError(f'invalid TLS or connection settings for {host}')
        if config['measurement_address'] != '127.0.0.1:6060':
            raise ValueError('keep the standard measurement listener private at 127.0.0.1:6060')
        for route in config['static_routes']:
            if not route.get('cache_control') or not route.get('prefix', '').startswith('/') or not route['prefix'].endswith('/') or not Path(route.get('directory', '')).is_absolute():
                raise ValueError(f'invalid static route for {host}')
        configs[host] = json.dumps(config, indent=2) + '\n'
    prepare_static(os.environ['ISUCON_STATIC_SOURCE'], os.environ['ISUCON_STATIC_OUTPUT'])
    output.mkdir(parents=True, exist_ok=True)
    for old in output.glob('*.json'):
        if old.stem not in configs:
            old.unlink()
    for host, content in configs.items():
        (output / f'{host}.json').write_text(content)


if __name__ == '__main__':
    main()
