import contextlib
import io
import json
import os
from pathlib import Path
import sqlite3
import sys
import urllib.error
import urllib.parse
import urllib.request


UNINSTALL = Path(sys.argv[1])
TEST_ROOT = Path(sys.argv[2])
TOKEN = "cf-test-secret-canary"
START = 'cf_out="$(python3 - "${sqlite}" "${VAR_ROOT}" <<\'PY\' 2>&1 || true\n'
END = "\nPY\n)\""


def embedded_cleanup():
    source = UNINSTALL.read_text(encoding="utf-8")
    start = source.index(START) + len(START)
    end = source.index(END, start)
    return source[start:end]


class Response:
    status = 200

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, traceback):
        return False


def run_case(name, statuses):
    root = TEST_ROOT / name
    secrets = root / "secrets"
    secrets.mkdir(parents=True)
    (secrets / "cloudflare-api-token").write_text(TOKEN, encoding="utf-8")
    state = root / "redgres.db"
    connection = sqlite3.connect(state)
    connection.execute("CREATE TABLE domain_deployment (id INTEGER PRIMARY KEY, payload TEXT NOT NULL)")
    connection.execute(
        "INSERT INTO domain_deployment(id, payload) VALUES(1, ?)",
        (
            json.dumps(
                {
                    "account_id": "account",
                    "zone_id": "zone",
                    "tunnel_id": "tunnel",
                    "access_apps": [{"app_id": "app", "policy_id": "policy"}],
                    "records": [{"id": "record"}],
                }
            ),
        ),
    )
    connection.commit()
    connection.close()

    calls = []

    def fake_urlopen(request, timeout=0):
        del timeout
        path = urllib.parse.urlsplit(request.full_url).path.removeprefix("/client/v4")
        calls.append(path)
        status = statuses.get(path, 200)
        if status >= 400:
            raise urllib.error.HTTPError(request.full_url, status, "fixture", {}, None)
        return Response()

    old_argv = sys.argv
    old_urlopen = urllib.request.urlopen
    stdout = io.StringIO()
    stderr = io.StringIO()
    try:
        sys.argv = ["embedded-cleanup", str(state), str(root)]
        urllib.request.urlopen = fake_urlopen
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            exec(compile(embedded_cleanup(), "uninstall.sh:cloudflare", "exec"), {"__name__": "__main__"})
    finally:
        sys.argv = old_argv
        urllib.request.urlopen = old_urlopen
    output = stdout.getvalue() + stderr.getvalue()
    assert TOKEN not in output
    return calls, output


policy = "/accounts/account/access/apps/app/policies/policy"
app = "/accounts/account/access/apps/app"
record = "/zones/zone/dns_records/record"
connections = "/accounts/account/cfd_tunnel/tunnel/connections"
tunnel = "/accounts/account/cfd_tunnel/tunnel"
all_calls = [policy, app, record, connections, tunnel]

calls, output = run_case("success", {})
assert calls == all_calls
assert "TUNNEL:deleted" in output and "STATUS:api_ok" in output

for name, failed_path in (("access-failure", policy), ("dns-failure", record)):
    calls, output = run_case(name, {failed_path: 403})
    assert connections not in calls and tunnel not in calls
    assert "TUNNEL:preserved" in output and "STATUS:api_partial" in output

calls, output = run_case("connection-failure", {connections: 400})
assert connections in calls and tunnel not in calls
assert "TUNNEL:preserved" in output and "STATUS:api_partial" in output

calls, output = run_case("tunnel-failure", {tunnel: 400})
assert calls == all_calls
assert "TUNNEL:preserved" in output and "STATUS:api_partial" in output

calls, output = run_case("idempotent-404", {path: 404 for path in all_calls})
assert calls == all_calls
assert "TUNNEL:deleted" in output and "STATUS:api_ok" in output

print("uninstall_cloudflare_mock_api=pass")
