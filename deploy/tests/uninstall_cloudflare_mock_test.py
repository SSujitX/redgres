import base64
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
TUNNEL_SECRET = "tunnel-secret-canary-value"
START = (
    'cf_out="$(python3 - "${sqlite}" "${VAR_ROOT}" "${ETC_ROOT}" \\\n'
    '    "/var/lib/redgres-tls/issue.result" "/etc/redgres/tls-lineage" <<\'PY\' 2>&1 || true\n'
)
END = "\nPY\n)\""


def embedded_cleanup():
    source = UNINSTALL.read_text(encoding="utf-8")
    start = source.index(START) + len(START)
    end = source.index(END, start)
    return source[start:end]


class Response:
    def __init__(self, status=200, body=b'{"success":true,"result":null}'):
        self.status = status
        self._body = body

    def read(self):
        return self._body

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, traceback):
        return False


def tunnel_token_blob(account="accountid1234567890abcdef", tunnel="11111111-2222-3333-4444-555555555555"):
    payload = {"a": account, "t": tunnel, "s": TUNNEL_SECRET}
    return base64.urlsafe_b64encode(json.dumps(payload, separators=(",", ":")).encode("utf-8")).decode("ascii").rstrip("=")


def write_api_token(root: Path, token: str = TOKEN):
    secrets = root / "secrets"
    secrets.mkdir(parents=True, exist_ok=True)
    if token:
        (secrets / "cloudflare-api-token").write_text(token, encoding="utf-8")


def write_sqlite(root: Path, payload: dict):
    state = root / "redgres.db"
    connection = sqlite3.connect(state)
    connection.execute("CREATE TABLE domain_deployment (id INTEGER PRIMARY KEY, payload TEXT NOT NULL)")
    connection.execute("INSERT INTO domain_deployment(id, payload) VALUES(1, ?)", (json.dumps(payload),))
    connection.commit()
    connection.close()
    return state


def run_case(
    name,
    statuses,
    *,
    sqlite_payload=None,
    missing_sqlite=False,
    extra_files=None,
    get_results=None,
    api_token=TOKEN,
):
    root = TEST_ROOT / name
    if root.exists():
        for path in sorted(root.rglob("*"), reverse=True):
            if path.is_file() or path.is_symlink():
                path.unlink()
            elif path.is_dir():
                path.rmdir()
    root.mkdir(parents=True, exist_ok=True)
    write_api_token(root, api_token)
    etc = root / "etc"
    etc.mkdir(parents=True, exist_ok=True)
    issue = root / "tls-issue.result"
    lineage = root / "tls-lineage"
    state = root / "missing.db"
    if sqlite_payload is not None:
        state = write_sqlite(root, sqlite_payload)
    elif not missing_sqlite:
        state = write_sqlite(
            root,
            {
                "account_id": "account",
                "zone_id": "zone",
                "tunnel_id": "tunnel",
                "access_apps": [{"app_id": "app", "policy_id": "policy"}],
                "records": [{"id": "record"}],
            },
        )
    if extra_files:
        for rel, content in extra_files.items():
            path = root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")

    calls = []
    get_results = get_results or {}

    def fake_urlopen(request, timeout=0):
        del timeout
        parsed = urllib.parse.urlsplit(request.full_url)
        path = parsed.path.removeprefix("/client/v4")
        query = parsed.query
        key = path if not query else f"{path}?{query}"
        calls.append(key if request.get_method() == "GET" else path)
        if request.get_method() == "GET":
            status = statuses.get(key, statuses.get(path, 200))
            if status >= 400:
                raise urllib.error.HTTPError(request.full_url, status, "fixture", {}, None)
            body = get_results.get(key, get_results.get(path, {"success": True, "result": []}))
            return Response(status=status, body=json.dumps(body).encode("utf-8"))
        status = statuses.get(path, 200)
        if status >= 400:
            raise urllib.error.HTTPError(request.full_url, status, "fixture", {}, None)
        return Response(status=status)

    old_argv = sys.argv
    old_urlopen = urllib.request.urlopen
    stdout = io.StringIO()
    stderr = io.StringIO()
    try:
        sys.argv = [
            "embedded-cleanup",
            str(state),
            str(root),
            str(etc),
            str(issue),
            str(lineage),
        ]
        urllib.request.urlopen = fake_urlopen
        with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            exec(compile(embedded_cleanup(), "uninstall.sh:cloudflare", "exec"), {"__name__": "__main__"})
    finally:
        sys.argv = old_argv
        urllib.request.urlopen = old_urlopen
    output = stdout.getvalue() + stderr.getvalue()
    assert TOKEN not in output, output
    assert TUNNEL_SECRET not in output, output
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
assert "SCOPE:full" in output

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

account = "accountid1234567890abcdef"
tunnel_id = "11111111-2222-3333-4444-555555555555"
fb_connections = f"/accounts/{account}/cfd_tunnel/{tunnel_id}/connections"
fb_tunnel = f"/accounts/{account}/cfd_tunnel/{tunnel_id}"

calls, output = run_case(
    "fallback-tunnel-only",
    {},
    missing_sqlite=True,
    extra_files={
        "secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id),
        # Root-owned TLS marker (app cannot forge alone on a real host).
        "tls-issue.result": "# redgres footprint\n",
    },
)
assert calls == [fb_connections, fb_tunnel]
assert "SCOPE:tunnel_only" in output
assert "TUNNEL:deleted" in output and "STATUS:api_ok" in output

calls, output = run_case(
    "fallback-tunnel-no-footprint",
    {},
    missing_sqlite=True,
    extra_files={"secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id)},
)
assert calls == []
assert "STATUS:no_state" in output

calls, output = run_case(
    "fallback-token-only",
    {},
    missing_sqlite=True,
)
assert calls == []
assert "STATUS:no_state" in output

zone_query = "/zones?name=example.com"
dns_query = "/zones/zoneid1234567890abcd/dns_records?name=console.example.com"
access_list = f"/accounts/{account}/access/apps"
access_del = f"/accounts/{account}/access/apps/app-console"
dns_del = "/zones/zoneid1234567890abcd/dns_records/dns-console"

calls, output = run_case(
    "fallback-hostnames",
    {},
    missing_sqlite=True,
    extra_files={
        "secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id),
        "tls-issue.result": "host=console.example.com\n",
    },
    get_results={
        zone_query: {
            "success": True,
            "result": [{"id": "zoneid1234567890abcd", "name": "example.com", "account": {"id": account}}],
        },
        access_list: {
            "success": True,
            "result": [
                {"id": "app-console", "domain": "console.example.com", "name": "console.example.com"},
                {"id": "app-other", "domain": "other.example.com", "name": "other.example.com"},
            ],
        },
        dns_query: {
            "success": True,
            "result": [{"id": "dns-console", "name": "console.example.com", "type": "CNAME"}],
        },
    },
)
assert zone_query in calls
assert access_list in calls
assert access_del in calls
assert "/accounts/%s/access/apps/app-other" % account not in calls
assert dns_query in calls
assert dns_del in calls
assert fb_connections in calls and fb_tunnel in calls
assert "SCOPE:full" in output
assert "TUNNEL:deleted" in output and "STATUS:api_ok" in output

calls, output = run_case(
    "fallback-env-hosts-ignored",
    {},
    missing_sqlite=True,
    extra_files={
        "secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id),
        "tls-issue.result": "# footprint only\n",
        "etc/redgres.env": "REDGRES_BASE_URL=https://console.example.com\n",
    },
)
# App-writable env hostnames must not drive DNS/Access deletes.
assert zone_query not in calls
assert access_list not in calls
assert dns_query not in calls
assert calls == [fb_connections, fb_tunnel]
assert "SCOPE:tunnel_only" in output
assert "STATUS:api_ok" in output

calls, output = run_case(
    "fallback-zone-miss",
    {zone_query: 404},
    missing_sqlite=True,
    extra_files={
        "secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id),
        "tls-issue.result": "host=console.example.com\n",
    },
)
assert fb_tunnel not in calls
assert "STATUS:insufficient_evidence" in output
assert "TUNNEL:preserved" in output

# Access name-only must not delete (domain mismatch).
calls, output = run_case(
    "fallback-access-name-ignored",
    {},
    missing_sqlite=True,
    extra_files={
        "secrets/cloudflared-tunnel-token": tunnel_token_blob(account, tunnel_id),
        "tls-issue.result": "host=console.example.com\n",
    },
    get_results={
        zone_query: {
            "success": True,
            "result": [{"id": "zoneid1234567890abcd", "name": "example.com", "account": {"id": account}}],
        },
        access_list: {
            "success": True,
            "result": [
                {"id": "app-named", "domain": "other.example.com", "name": "console.example.com"},
            ],
        },
        dns_query: {"success": True, "result": []},
    },
)
assert f"/accounts/{account}/access/apps/app-named" not in calls
assert "STATUS:api_ok" in output

oauth_token = "oauth-access-canary-secret"
calls, output = run_case(
    "oauth-canary",
    {},
    api_token="",
    sqlite_payload={
        "account_id": "account",
        "zone_id": "zone",
        "tunnel_id": "tunnel",
        "access_apps": [{"app_id": "app", "policy_id": "policy"}],
        "records": [{"id": "record"}],
    },
    extra_files={"secrets/cloudflare-oauth-token.json": json.dumps({"access_token": oauth_token})},
)
assert calls == all_calls
assert oauth_token not in output
assert "STATUS:api_ok" in output

print("uninstall_cloudflare_mock_api=pass")
