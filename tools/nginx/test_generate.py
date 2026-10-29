import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest
import urllib.request
import uuid

from generate import generate

ROOT = Path(__file__).resolve().parents[2]


class GenerationTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "source"
        shutil.copytree(ROOT / "nginx", self.source)
        self.output = self.root / "generated"
        self.options = dict(
            all_hosts=["h1", "h2", "h3"], nginx_hosts=["h1", "h2"],
            app_hosts=["h1", "h2", "h3"], traffic_hosts=["h1", "h2"],
            addresses={"h1": "127.0.0.1", "h2": "192.0.2.2", "h3": "2001:db8::3"},
            host_config={}, app_port=8080, nginx_port=8088,
            static_root="/home/isucon/public", access_log="/var/log/nginx/access.log",
        )

    def render(self, **overrides):
        options = copy.deepcopy(self.options)
        options.update(overrides)
        generate(self.source, self.output, **options)

    def test_default_hosts_share_base_and_upstreams(self):
        self.render()
        for host in ("h1", "h2"):
            config = self.output / host
            self.assertIn("listen 8088", (config / "sites-enabled/app.conf").read_text())
            self.assertIn("gzip on;", (config / "nginx.conf").read_text())
            upstream = (config / "conf.d/upstream.conf").read_text()
            self.assertIn("127.0.0.1:8080", upstream)
            self.assertIn("192.0.2.2:8080", upstream)
            self.assertIn("keepalive_requests 100000;", upstream)
        self.assertFalse((self.output / "h3").exists())
        self.assertIn("@@NGINX_PORT@@", (self.source / "roles/edge/sites-enabled/app.conf").read_text())

    def test_role_and_backend_overrides_remain_isolated(self):
        role = self.source / "roles/custom/sites-enabled"
        role.mkdir(parents=True)
        (role / "app.conf").write_text("# custom role\nserver { listen @@NGINX_PORT@@; }\n")
        self.render(host_config={"h2": {"role": "custom", "app_hosts": ["h3"], "listen_port": 9090}})
        self.assertIn("custom role", (self.output / "h2/sites-enabled/app.conf").read_text())
        self.assertNotIn("custom role", (self.output / "h1/sites-enabled/app.conf").read_text())
        upstream = (self.output / "h2/conf.d/upstream.conf").read_text()
        self.assertIn("[2001:db8::3]:8080", upstream)
        self.assertNotIn("192.0.2.2", upstream)
        probes = (self.output / "_checks/h2.sh").read_text()
        self.assertIn("listen 9090", (self.output / "h2/sites-enabled/app.conf").read_text())
        self.assertIn("/dev/tcp/127.0.0.1/9090", probes)
        self.assertIn("/dev/tcp/2001:db8::3/8080", probes)
        self.assertNotIn("192.0.2.2", probes)

    @unittest.skipUnless(shutil.which("task"), "task is not installed")
    def test_taskfile_passes_host_overrides_to_renderer(self):
        taskfile = (ROOT / "Taskfile.yml").read_text().replace(
            "  NGINX_HOST_CONFIG:\n    map: {}",
            "  NGINX_HOST_CONFIG:\n    map:\n      isucon-2:\n        role: edge\n        app_hosts: [isucon-3]",
        )
        (self.root / "Taskfile.yml").write_text(taskfile)
        shutil.copyfile(ROOT / "Taskfile.contest.yml", self.root / "Taskfile.contest.yml")
        shutil.copytree(self.source, self.root / "nginx")
        (self.root / "tools/nginx").mkdir(parents=True)
        shutil.copyfile(ROOT / "tools/nginx/generate.py", self.root / "tools/nginx/generate.py")
        subprocess.run(["task", "gen", "NGINX_HOSTS=isucon-1 isucon-2",
                        "APP_HOSTS=isucon-1 isucon-2 isucon-3"],
                       cwd=self.root, check=True, capture_output=True, text=True)
        first = (self.root / "generated/nginx/isucon-1/conf.d/upstream.conf").read_text()
        second = (self.root / "generated/nginx/isucon-2/conf.d/upstream.conf").read_text()
        self.assertIn("172.31.0.126:8080", first)
        self.assertIn("172.31.5.116:8080", second)
        self.assertNotIn("172.31.0.126:8080", second)
        unit = (self.root / "generated/systemd/app.service").read_text()
        self.assertIn("ExecStart=/home/isucon/webapp/go/app", unit)
        self.assertIn("Environment=APP_ADDR=:8080", unit)
        self.assertIn("Environment=MEASUREMENT_ADDR=127.0.0.1:6060", unit)

    def test_regeneration_removes_old_host_and_files(self):
        self.render()
        (self.output / "h1/stale.conf").write_text("stale")
        self.render(nginx_hosts=["h1"])
        self.assertFalse((self.output / "h2").exists())
        self.assertFalse((self.output / "_checks/h2.sh").exists())
        self.assertFalse((self.output / "h1/stale.conf").exists())

    def test_invalid_configuration_keeps_previous_output(self):
        self.render()
        previous = (self.output / "h1/nginx.conf").read_text()
        for invalid in (
            {"h2": {"role": "../edge"}}, {"h2": {"app_hosts": ["unknown"]}},
            {"h3": {"role": "edge"}}, {"h2": {"role": "missing"}},
            {"h2": {"app_hosts": []}}, {"h2": {"typo": True}}, {"h2": {"listen_port": 65536}},
        ):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                self.render(host_config=invalid)
            self.assertEqual((self.output / "h1/nginx.conf").read_text(), previous)

    @unittest.skipUnless(os.environ.get("NGINX_TEST_IMAGE"), "set NGINX_TEST_IMAGE for Docker validation")
    def test_generated_configs_run_on_nginx(self):
        # Keep test state under the shared workspace for Docker Desktop mounts.
        (ROOT / ".task").mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(dir=ROOT / ".task") as runtime:
            runtime = Path(runtime)
            source = runtime / "source"
            shutil.copytree(self.source, source)
            # A local nginx fixture serves as the upstream; the public entry is unchanged.
            (source / "common/conf.d").mkdir()
            (source / "common/conf.d/test-backend.conf").write_text(
                "server { listen 8080; access_log off; location / { "
                "add_header X-Measurement-Session-ID stable-id; "
                "add_header X-Measurement-Request-Body-Bytes 7; return 200 'backend'; } }"
            )
            site = source / "roles/edge/sites-enabled/app.conf"
            site.write_text(site.read_text().replace("    location / {", """
    location /assets/ {
        include /etc/nginx/snippets/static-cache.conf;
        try_files $uri =404;
    }
    location / {"""))
            (source / "common/mime.types").write_text("types { text/css css; }")
            output = runtime / "generated"
            options = copy.deepcopy(self.options)
            options.update(traffic_hosts=["h1"], access_log="/tmp/access.log", static_root="/srv/assets")
            generate(source, output, **options)
            assets = runtime / "assets/assets"
            assets.mkdir(parents=True)
            (assets / "test.css").write_text("body { color: green; }\n" * 200)
            for host in ("h1", "h2"):
                name = "isucon-nginx-test-" + uuid.uuid4().hex[:12]
                def docker(*args):
                    return subprocess.check_output(["docker", *args], text=True, timeout=40).strip()
                try:
                    docker("run", "--rm", "-d", "--name", name, "-p", "127.0.0.1::8088",
                           "-v", f"{output / host}:/etc/nginx:ro", "-v", f"{runtime / 'assets'}:/srv/assets:ro",
                           os.environ["NGINX_TEST_IMAGE"], "nginx", "-g", "daemon off;")
                    base = "http://" + docker("port", name, "8088/tcp").splitlines()[0]
                    for _ in range(30):
                        try:
                            with urllib.request.urlopen(base, timeout=1):
                                break
                        except OSError:
                            time.sleep(0.1)
                    docker("exec", name, "nginx", "-t")
                    with urllib.request.urlopen(base + "/api?q=1", timeout=2) as response:
                        self.assertEqual(response.read(), b"backend")
                        self.assertIsNone(response.headers.get("X-Measurement-Session-ID"))
                        self.assertIsNone(response.headers.get("Cache-Control"))
                    request = urllib.request.Request(base + "/assets/test.css", headers={"Accept-Encoding": "gzip"})
                    with urllib.request.urlopen(request, timeout=2) as response:
                        response.read()
                        self.assertEqual(response.headers["Content-Encoding"], "gzip")
                        self.assertIn("max-age=86400", response.headers["Cache-Control"])
                    docker("exec", name, "nginx", "-s", "reopen")
                    time.sleep(1.2)
                    rows = [json.loads(line) for line in docker("exec", name, "cat", "/tmp/access.log").splitlines()]
                    row = next(row for row in rows if row["uri"] == "/api?q=1")
                    self.assertEqual(row["session_id"], "stable-id")
                    self.assertEqual(row["request_body_bytes"], 7)
                finally:
                    subprocess.run(["docker", "rm", "-f", name], check=False,
                                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=40)


if __name__ == "__main__":
    unittest.main()
