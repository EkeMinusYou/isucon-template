"""Exercise a few requests without running the benchmark.

Placeholder: `task setup-smoke` pipes this file into python3 on ENTRY_HOST, so it
runs on the server with no arguments and no local dependencies. Replace the
requests below with the contest's own session flow (registration, login, and a
few authenticated reads) during setup. Keep it read-mostly, keep it short, and
never print session identifiers or credentials.
"""

import urllib.request

headers = {"Content-Type": "application/json"}


def request(path, body=None):
    req = urllib.request.Request("http://127.0.0.1" + path, data=body, headers=headers)
    with urllib.request.urlopen(req, timeout=15) as response:
        assert response.status == 200, response.status
        return response.read()


paths = ("/",)
for path in paths:
    request(path)
print(f"Smoke: {len(paths)} unauthenticated read(s) passed; replace this script with the contest's own flow")
