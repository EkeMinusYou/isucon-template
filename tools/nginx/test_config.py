"""Exercise the shipped nginx config against a local, isolated Docker fixture."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import time
import unittest
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
IMAGE = os.environ.get("NGINX_TEST_IMAGE", "nginx:1.26.3")


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, timeout=40).strip()


class NginxConfigTest(unittest.TestCase):
    def test_proxy_cache_compression_and_measurement(self):
        with tempfile.TemporaryDirectory(prefix="nginx-test-", dir=ROOT / ".task") as tmp:
            tmp = Path(tmp)
            conf = tmp / "nginx"
            shutil.copytree(ROOT / "nginx", conf)
            (conf / "mime.types").write_text("types { text/html html; text/css css; application/javascript js; }\n")
            main = conf / "nginx.conf"
            main.write_text(main.read_text().replace("/var/log/nginx/access.log", "/tmp/access.log"))
            # Keep this test independent of contest topology and credentials.
            (conf / "conf.d/upstream.conf").write_text(
                "upstream app { server 127.0.0.1:8081; keepalive 1024; keepalive_requests 100000; }\n"
                "server { listen 8081; access_log off; location / { "
                "add_header X-Measurement-Session-ID stable-id; "
                "add_header X-Measurement-Request-Body-Bytes 7; "
                "return 200 'upstream response'; } }\n"
            )
            (conf / "snippets/static-root.conf").write_text("root /srv/assets;\n")
            site = conf / "sites-enabled/app.conf"
            site.write_text(site.read_text().replace(
                "    location / {",
                "    location /assets/ { include /etc/nginx/snippets/static-cache.conf; try_files $uri =404; }\n"
                "    location / {",
            ))
            assets = tmp / "public/assets"
            assets.mkdir(parents=True)
            (assets / "bundle.css").write_text("body { color: green; }\n" * 200)
            name = "isucon-nginx-test-" + uuid.uuid4().hex[:12]
            try:
                docker("run", "--rm", "-d", "--name", name,
                       "-p", "127.0.0.1::80", "-v", f"{conf}:/etc/nginx:ro",
                       "-v", f"{tmp / 'public'}:/srv/assets:ro", IMAGE,
                       "nginx", "-g", "daemon off;")
                address = docker("port", name, "80/tcp").splitlines()[0]
                base = "http://" + address
                for _ in range(50):
                    try:
                        with urllib.request.urlopen(base, timeout=1):
                            break
                    except (OSError, urllib.error.URLError):
                        time.sleep(0.1)
                else:
                    self.fail(docker("logs", name))
                docker("exec", name, "nginx", "-t")
                with urllib.request.urlopen(urllib.request.Request(base + "/api?q=1", headers={
                    "X-Measurement-Session-ID": "forged-id"
                })) as response:
                    self.assertEqual(response.read(), b"upstream response")
                    self.assertIsNone(response.headers.get("X-Measurement-Session-ID"))
                    self.assertIsNone(response.headers.get("X-Measurement-Request-Body-Bytes"))
                    self.assertIsNone(response.headers.get("Cache-Control"))
                with urllib.request.urlopen(urllib.request.Request(base + "/assets/bundle.css", headers={
                    "Accept-Encoding": "gzip"
                })) as response:
                    response.read()
                    self.assertIn("max-age=86400", response.headers["Cache-Control"])
                    self.assertEqual(response.headers["Content-Encoding"], "gzip")
                    self.assertIn("Accept-Encoding", response.headers["Vary"])
                docker("exec", name, "nginx", "-s", "reopen")
                time.sleep(1.2)
                lines = docker("exec", name, "cat", "/tmp/access.log").splitlines()
                entries = [json.loads(line) for line in lines]
                request = next(e for e in entries if e["uri"] == "/api?q=1")
                self.assertEqual(request["session_id"], "stable-id")
                self.assertEqual(request["request_body_bytes"], 7)
                self.assertIsNone(request["request_content_length"])
                static = next(e for e in entries if e["uri"] == "/assets/bundle.css")
                self.assertIsNone(static["request_body_bytes"])
                self.assertEqual(static["status"], 200)
            finally:
                subprocess.run(["docker", "rm", "-f", name], stdout=subprocess.DEVNULL,
                               stderr=subprocess.DEVNULL, check=False)


if __name__ == "__main__":
    (ROOT / ".task").mkdir(exist_ok=True)
    unittest.main()
