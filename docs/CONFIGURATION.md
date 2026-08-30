# Configuration reference

Core keys, the PostgreSQL administrative connection keys, the optional legacy vault secret file, the Redis URL-file / plaintext-override / optional public host-port / expected-series keys, and the optional expert tool-link hrefs listed as implemented below are loaded by `internal/config`. Remaining feature-gate and URL-generation keys remain target. Production startup that fails when vault rows exist and the vault secret file is missing remains outstanding. Machine-checked reference generation from the config struct is still outstanding.

## Core

Status: implemented in `internal/config`.

| Variable | Required in production | Example | Rule |
|---|---:|---|---|
| `REDGRES_ENVIRONMENT` | Yes | `production` | `development` or `production`; controls fail-closed validation, not authorization |
| `REDGRES_ADDRESS` | Yes | `127.0.0.1:8790` | Production must be loopback unless an ADR approves a trusted private bind |
| `REDGRES_BOOTSTRAP_ADDRESS` | No | `0.0.0.0:8989` | Optional first-run bootstrap listener (PRD OPS-008, ADR-012). Empty disables it (fail-closed). May bind a non-loopback address (the ADR-012 exception, source-restricted by the firewall rather than the bind). Must be a distinct `host:port` from `REDGRES_ADDRESS`; validation errors never echo the value. After confirm-reachable (or TTL persist), `bootstrap.closed` next to SQLite skips this listener even if the env value is still set; confirm rewrites the env file to empty. |
| `REDGRES_BOOTSTRAP_TTL` | No | `30m` | Bootstrap hard-cap auto-close; must be positive. The listener closes itself after this duration even if setup is abandoned. TTL also writes `bootstrap.closed` so a later systemd restart does not reopen `:8989`. Process SIGTERM does not persist close (crash/restart during setup can reopen until TTL or confirm). |
| `REDGRES_BOOTSTRAP_ALLOW_FROM` | No | *(empty)* | Optional single operator IP for the installer UFW rule (`ufw allow from IP to any port 8989`). When unset, the installer uses the SSH client address from `SSH_CONNECTION` / `SSH_CLIENT`, recovers those values from ancestor process environments after `sudo` strips them, or parses the remote host from `who -m`. If still unknown and `/dev/tty` is readable, it asks once for the public IP the operator will browse from. Loopback, unspecified, and CIDR values are rejected. Never opens `8989/tcp` to the world. |
| `REDGRES_CLOUDFLARE_TOKEN_FILE` | No | `/var/lib/redgres/secrets/cloudflare-api-token` | Optional path where the per-zone Cloudflare API token is stored (server-side secret, PRD OPS-009). Empty disables the Domain & Network wizard. The value is a path, never the token itself; production must be under `/var/lib/redgres`. Live install/upgrade writes this path; the file is created later when the owner pastes a token. |
| `REDGRES_TUNNEL_TOKEN_FILE` | No | `/var/lib/redgres/secrets/cloudflared-tunnel-token` | Path where apply stores the one-time cloudflared connector token (`0600`, never returned). Must match `LoadCredential=` in `deploy/systemd/cloudflared-redgres.service`. Production path must be under `/var/lib/redgres`. Live install/upgrade writes this path. See [CLOUDFLARED.md](CLOUDFLARED.md). |
| `REDGRES_CLOUDFLARE_OAUTH_CLIENT_FILE` | No | `/var/lib/redgres/secrets/cloudflare-oauth-client.json` | OAuth app `{client_id, client_secret}` pasted by operator (`0600`). Live install/upgrade writes this path. |
| `REDGRES_CLOUDFLARE_OAUTH_TOKEN_FILE` | No | `/var/lib/redgres/secrets/cloudflare-oauth-token.json` | Steady-state OAuth `{access_token, refresh_token, expires_at}` (`0600`, never returned). Live install/upgrade writes this path. |
| `REDGRES_CLOUDFLARE_OAUTH_AUTH_URL` | No | `https://dash.cloudflare.com/oauth2/auth` | OAuth authorization endpoint override. |
| `REDGRES_CLOUDFLARE_OAUTH_TOKEN_URL` | No | `https://dash.cloudflare.com/oauth2/token` | OAuth token endpoint override. |
| `REDGRES_CLOUDFLARE_OAUTH_REVOKE_URL` | No | `https://dash.cloudflare.com/oauth2/revoke` | OAuth revoke endpoint override. |
| `REDGRES_CERTBOT_BIN` | No | `certbot` | Certbot executable for DNS-01 issuance (db/redis). |
| `REDGRES_CERTBOT_DNS_TOKEN_FILE` | No | `/var/lib/redgres/secrets/certbot-dns.ini` | certbot-dns-cloudflare credentials file (never returned). Live install/upgrade writes this path. Apply with an API token writes the ini from that token. |
| `REDGRES_TLS_ISSUE_REQUEST_FILE` | No | `/var/lib/redgres/tls-issue.request` | Hostnames-only request file for the privileged TLS helper (`redgres-tls-issue.path`). Not a credential. Live install/upgrade writes this path. |
| `REDGRES_TLS_ISSUE_RESULT_FILE` | No | `/var/lib/redgres/tls-issue.result` | Helper writes `issued` or `failed` (no secrets). GET `/api/v1/domain` overlays issued onto `tls`. |
| `REDGRES_TLS_CERT_DIR` | No | `/etc/ssl/redgres` | Staging directory for Let's Encrypt copies. PostgreSQL reload uses postgres-owned files in `/etc/postgresql/*/main/`; PgBouncer uses pgbouncer-owned files in `/etc/pgbouncer/`. |
| `REDGRES_BOOTSTRAP_UFW_REMOVE_CMD` | No | `/usr/libexec/redgres/bootstrap-ufw-remove.sh` | Absolute path to a helper that removes the bootstrap UFW rule. Confirm-reachable also writes `/var/lib/redgres/bootstrap-ufw-remove.requested` for a root systemd path unit (so `User=redgres` can still request removal). |
| `REDGRES_BASE_URL` | Yes | `https://console.onelifeltd.xyz` | Exact **browser page origin** for cookie/origin checks; production must be `https` except during the ADR-012 bootstrap window (`REDGRES_BOOTSTRAP_ADDRESS` set), when `http` is allowed so the operator can open `:8989`. This is not the listen address. |
| `REDGRES_SQLITE_PATH` | Yes | `/var/lib/redgres/redgres.db` | Must not contain `?`, `#`, `%`, or NUL. Production path must be a file beneath a verified real `/var/lib/redgres` directory (not a lexical prefix check alone); development keeps relative and temporary paths |
| `REDGRES_SESSION_TTL` | No | `12h` | Idle expiry; minimum 5m, maximum 24h |
| `REDGRES_ABSOLUTE_SESSION_TTL` | No | `24h` | Must be >= idle expiry and at most 168h |
| `REDGRES_COOKIE_SECURE` | Yes | `true` | Must be true in production except during the ADR-012 bootstrap window (`REDGRES_BOOTSTRAP_ADDRESS` set), when `false` is allowed for HTTP `:8989`. Confirm-reachable rewrites the env file to `true` and an `https` `REDGRES_BASE_URL`. |
| `REDGRES_LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, or `error`. Never enables secret/body logging |
| `REDGRES_DEV_ASSET_DIR` | No (rejected) | `./internal/web/dist/app` | Development only; must be an existing directory; rejected in production; must not contain `?`, `#`, or NUL. Empty selects the embedded assets |
| `REDGRES_BACKUP_CATALOG` | No | `/var/lib/redgres/backups` | Optional catalog jail directory. Must not contain `?`, `#`, `%`, or NUL (never echo the path). Production must be `/var/lib/redgres` or a path under it. Empty is allowed at Load. The only catalog file is jail-root `current.json`. HTTP DROP loads that file and evaluates the backup gate. Live dump/copy/restore is installer-recovery. |

## PostgreSQL

Status: administrative connection + protected lists implemented for inventory. Optional pooled-observation port implemented for GET `/api/v1/status` PgBouncer Ping. Optional public URL host and direct port implemented for masked and revealed project URLs. Optional legacy vault secret file implemented for POST reveal.

| Variable | Status | Purpose |
|---|---|---|
| `REDGRES_POSTGRES_HOST` / `PORT` / `DATABASE` / `USER` | Implemented | Direct administrative connection components. Never a full admin DSN on the CLI. |
| `REDGRES_POSTGRES_PASSWORD_FILE` | Implemented | Only password source. Symlinks and non-regular files are rejected. Production also rejects missing, empty, or group/world-readable files. |
| `REDGRES_POSTGRES_SSLMODE` | Implemented | Development default `prefer`. Production requires `require`, `verify-ca`, or `verify-full`. |
| `REDGRES_POSTGRES_SSLROOTCERT` | Implemented | Trusted CA path when verifying |
| `REDGRES_POSTGRES_EXPECTED_MAJOR` | Implemented | Optional identity assertion (`17` or `18`); detected `server_version_num` remains authoritative |
| `REDGRES_POSTGRES_PROTECTED_DATABASES` | Implemented | Additional comma-separated deny set (plus hard-coded `postgres`, `template0`, `template1`, `database_console_vault`, and the admin catalog database) |
| `REDGRES_POSTGRES_PROTECTED_ROLES` | Implemented | Additional owner deny set (plus hard-coded admin/builtin roles and `pg_*`) |
| `REDGRES_POSTGRES_PUBLIC_HOST` | Implemented | Optional host placed in project `postgresql://` URLs. Production `serve` does not require it. When set, the value must not contain whitespace, `@`, `/`, or a URI scheme. Never copies `REDGRES_POSTGRES_HOST`. No silent default (`db.example.com` is not applied). Does not mark PostgreSQL configured. |
| `REDGRES_POSTGRES_DIRECT_PORT` | Implemented | Optional TCP port (1–65535) placed in masked **direct** project URLs. No silent `5432`. Production `serve` does not require it. Never copies admin `REDGRES_POSTGRES_PORT`. Does not mark PostgreSQL configured. `masked_direct_url` is omitted unless this and `REDGRES_POSTGRES_PUBLIC_HOST` are both set and vault status is `present`. |
| `REDGRES_POSTGRES_POOLED_PORT` | Implemented | Optional TCP port (1–65535) with two consumers: (1) PgBouncer console observation on GET `/api/v1/status` (admin host/user/password/TLS; virtual database `pgbouncer`); (2) masked **pooled** project URL port when `REDGRES_POSTGRES_PUBLIC_HOST` is also set and vault status is `present`. No silent `6432`. Live `fresh` PgBouncer writes `6432` only after the installer has configured and health-checked loopback listen. Production `serve` does not require it. Set only when the administrative PostgreSQL connection is otherwise configured (same fail-closed partial-key rule as other `REDGRES_POSTGRES_*` keys). Observation never uses the public URL host. The admin role must be in PgBouncer `admin_users` or `stats_users`. Never a second password file, `pgbouncer_auth`, or `userlist.txt` for the application (the installer may write PgBouncer's own `auth_file` for `auth_user` only). There is no `REDGRES_POSTGRES_PUBLIC_POOLED_PORT`. |
| `REDGRES_LEGACY_VAULT_SECRET_FILE` | Implemented | Optional path to the exact legacy KDF secret (`SESSION_SECRET` bytes). File path only. Not part of `PostgresConfigured` / `postgresAnySet`. Production `serve` does not require it. When set and PostgreSQL is configured, `postgresadmin.Open` reads it with the same fail-closed regular-file/no-symlink rules as `REDGRES_POSTGRES_PASSWORD_FILE` (`TrimRight` `\r\n`; production not group/world-readable; empty rejected). Errors name this env var and never echo contents. Open stores the derived Fernet key on the service and wipes the raw secret. Unset → Open succeeds; POST reveal and POST create return `503`. Does not mark PostgreSQL configured. PG-003 create does not add `REDGRES_POSTGRES_CONNECTION_LIMIT` (role limit is constant 20). |

Development may start without PostgreSQL; list/details then return `503` `dependency_unavailable` and do not fabricate an empty healthy cluster. Production `serve` fails closed if the administrative connection is incomplete or the password file is unusable. Production `serve` does not require `REDGRES_POSTGRES_POOLED_PORT`, `REDGRES_POSTGRES_PUBLIC_HOST`, `REDGRES_POSTGRES_DIRECT_PORT`, or `REDGRES_LEGACY_VAULT_SECRET_FILE`. Connecting **to** the `postgres` catalog database is required; listing or detailing that database is forbidden. Direct project URLs can exist without pooled URLs and vice versa.

These are application connection settings, not authority to install PostgreSQL or change extensions. Installer-only lifecycle values such as `POSTGRES_MODE`, `POSTGRES_MAJOR`, `PGBOUNCER_MODE`, `POSTGRES_EXTENSION_POLICY`, and `POSTGRES_EXTENSION_PLAN_FILE` live in the protected install configuration and are validated under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). The main installer dry-run now syntax-validates exactly those five keys from a trusted, descriptor-pinned file: one `KEY=VALUE` per line, optional empty lines and `#` comments, no duplicate or unknown keys, no `export` syntax, maximum 64 KiB. Values are inert bytes and are never sourced, evaluated, interpolated, exported, or printed. This Partial parser does not yet apply config values, define precedence over CLI selections, or perform the per-key domain validation required before mutation. The application environment never accepts package names, repositories, arbitrary extension SQL or preload libraries.

Never accept a full admin DSN on the CLI. If a DSN file is supported, parse/redact it and give it the same protection as a password.

## Redis

Status: administrator URL file + plaintext override implemented for Ping health, GET `/api/v1/redis/status` metrics, ACL inspect, and POST `/api/v1/redis/users` create. `skip_verify` is rejected. Optional public host/port are implemented for generated project URLs. Expected series is implemented as an identity guard.

| Variable | Status | Purpose |
|---|---|---|
| `REDGRES_REDIS_ADMIN_URL_FILE` | Implemented | Path to a regular, non-symlink file containing one `redis://` or `rediss://` admin URL. File path only; a raw URL as the env value is rejected. No `REDGRES_REDIS_ADMIN_URL` fallback. Production `serve` fails closed if the file is missing, empty, unreadable, group/world-readable, replaced while opening, or not regular. Development may start without Redis; GET `/status` and GET `/api/v1/redis/status` then report `not_configured`. `skip_verify=true` / `1` (go-redis `ParseURL` `InsecureSkipVerify` on `rediss`) is rejected in every environment as `REDGRES_REDIS_ADMIN_URL_FILE: invalid value`; the error never echoes the URL or password. |
| `REDGRES_REDIS_ALLOW_PLAINTEXT` | Implemented | Default false. `redis://` to non-loopback requires true. Loopback `redis://` is allowed without it. `rediss://` is always accepted. |
| `REDGRES_REDIS_PUBLIC_HOST` | Implemented | Optional host placed in created project `rediss://` URLs. Production `serve` does not require it. When set, the value must not contain whitespace, `@`, `/`, or a URI scheme. |
| `REDGRES_REDIS_PUBLIC_PORT` | Implemented | Optional TCP port (1–65535) placed in created project URLs. No silent default. `credential.urls` is omitted unless both this and `REDGRES_REDIS_PUBLIC_HOST` are set. Production `serve` does not require it. |
| `REDGRES_REDIS_EXPECTED_SERIES` | Implemented | Optional identity assertion (`8.2` or `8.8`); detected `INFO server` `redis_version` remains authoritative. Empty is allowed. Any other value fails `Load` and names this env var (never echoes the value). Setting it without `REDGRES_REDIS_ADMIN_URL_FILE` fails closed (same partial-key pattern as `REDGRES_POSTGRES_EXPECTED_MAJOR`). It cannot widen the release-owned matrix in [COMPATIBILITY.md](COMPATIBILITY.md). |

Plain `redis://` to non-loopback is rejected unless the explicit private-path override is true. Public generated URLs use `rediss://`. Validation errors name the environment variable and never echo the URL, userinfo, or host from the secret file.

Supported service versions are defined by the Redgres release and [COMPATIBILITY.md](COMPATIBILITY.md), not by environment configuration. Expected-version settings detect connection to the wrong server; they cannot make an unsupported version supported. PostgreSQL is detected with `SHOW server_version_num`, Redis from `INFO server`, and PgBouncer with `SHOW VERSION`, followed by required capability checks.

## UI/tool links

Status: optional pgAdmin and RedisInsight hrefs implemented for GET `/api/v1/session` and GET `/api/v1/status` presence. They are links, not embedded privileged sessions. Redgres never fetches, pings, proxies, or iframes these URLs at Load or on GET `/status`.

| Variable | Status | Purpose |
|---|---|---|
| `REDGRES_PGADMIN_URL` | Implemented | Optional absolute href for the expert pgAdmin tool. Empty/unset is valid. Production `serve` does not require it. Independent of `REDGRES_REDISINSIGHT_URL`. |
| `REDGRES_REDISINSIGHT_URL` | Implemented | Optional absolute href for RedisInsight. Empty/unset is valid. Production `serve` does not require it. Independent of `REDGRES_PGADMIN_URL`. |

When set, the value must be an absolute URL with a scheme and host, no userinfo, and no fragment. Query string and path are allowed. Relative URLs and `javascript:`, `data:`, and `file:` schemes are rejected. Production requires `https`. Development accepts `http` or `https`. Validation errors name the environment variable and never echo the URL. There is no silent default hostname.

## Feature gates

- `REDGRES_FEATURE_POSTGRES_DROP=false` (PG-011 Partial freeze: load with `envBool`; default unset = off; HTTP 403 **Drop is turned off.** before JSON decode)
- `REDGRES_FEATURE_POSTGRES_TRUNCATE=false` (PG-009 Partial freeze: **implemented**)
- `REDGRES_FEATURE_POSTGRES_ROW_DELETE=false` (PG-008 Partial freeze: **implemented**)

`REDGRES_FEATURE_POSTGRES_ROW_DELETE`, `REDGRES_FEATURE_POSTGRES_TRUNCATE`, and `REDGRES_FEATURE_POSTGRES_DROP` are parsed with `envBool`: unset/empty → false; `1`/`true`/`yes`/`on` → true; `0`/`false`/`no`/`off` → false; any other value fails `Load` and names the env var (never echoes the value). Do not use `envBoolDefaultFalse` (it would swallow invalid as off). Do not enable truncate or row-delete from the DROP key, or drop from the truncate/row-delete keys.

There is no `REDGRES_FEATURE_POSTGRES_CREATE`. Create is not a destructive flag.
There is no `REDGRES_FEATURE_POSTGRES_ROTATE`. Rotate is not a destructive flag.
There is no `REDGRES_FEATURE_POSTGRES_DUPLICATE`. Duplicate is not a destructive flag.
There is no `ENABLE_DESTRUCTIVE_ACTIONS` and no `REDGRES_FEATURE_POSTGRES_DELETE`.

Enabling a flag makes the server-side workflow reachable; it never bypasses capabilities, protected targets, CSRF, confirmation, reauthentication, or audit. Flag-off DELETE rows is `403` `forbidden` with copy **Row delete is turned off.** before JSON decode. Flag-off POST truncate is `403` `forbidden` with copy **Truncate is turned off.** before JSON decode. Flag-off DELETE database is `403` `forbidden` with copy **Drop is turned off.** before JSON decode. `GET /api/v1/session` does not gain a `features` object in this slice. `REDGRES_BACKUP_CATALOG` is loaded (optional). Unset is allowed at `config.Load`. Production values must be under `/var/lib/redgres`. Reserved `?`, `#`, `%`, and NUL are rejected without echoing the path. HTTP DROP loads jail-root `current.json` via `backup.LoadCurrent` and calls `EvaluateDropGate` after AUTH-006 and before terminate/`DROP DATABASE`. Unset or unusable catalog is `503` **Backup catalog is not configured.** (never echo the path). The catalog file is jail-root `current.json` only. Live dump/copy/restore is installer-recovery.

## Owner bootstrap CLI

`redgres create-owner` does not call full `config.Load` (production BaseURL/CookieSecure checks would block a local bootstrap). Flags: `--username` (required), `--sqlite-path` (default `REDGRES_SQLITE_PATH` or `./redgres.db`), `--replace`. Password is read twice from a TTY via `golang.org/x/term`; there is no `--password` flag or password environment variable. Development `.env` is applied with the same production-skip rule as `serve`. Password policy constants live in `internal/auth` (15 Unicode code points, 1024-byte maximum); they are not environment keys.

## Precedence

`REDGRES_BASE_URL` must match the origin the browser shows. `127.0.0.1` and `localhost` are different origins. The Vite proxy does not rewrite `Origin`.

| Workflow | `REDGRES_ADDRESS` | `REDGRES_BASE_URL` | Open in browser |
|---|---|---|---|
| Unified dev (`npm --prefix web run dev:full`) | `127.0.0.1:8989` | `http://127.0.0.1:8989` | `http://127.0.0.1:8989` |
| Vite HMR (`npm run dev` in `web/`) | `127.0.0.1:8790` | `http://127.0.0.1:5173` | `http://127.0.0.1:5173` |
| Embedded UI (`npm run build` + `redgres serve`) | `127.0.0.1:8790` | `http://127.0.0.1:8790` | `http://127.0.0.1:8790` |

`dev:full` sets `REDGRES_DEV_ASSET_DIR` itself so rebuilt UI assets are served without restarting the Go process. `internal/config` resolves and validates the value; `internal/web` reads no environment and opens the directory as an `os.Root` so links inside it cannot resolve outside it. There is no CLI flag for it.

Recommended precedence: explicit flags (non-secret) > process environment > optional development `.env` > defaults. Production does not read repository `.env` files, including when selected with `-environment production`. A `.env` that sets `REDGRES_ENVIRONMENT=production` is rejected. Dotenv applies only `REDGRES_*` keys and never overwrites an already-set process variable. `REDGRES_BASE_URL` must be an origin (`scheme://host[:port]`) with no userinfo, path, query, or fragment. `REDGRES_SQLITE_PATH` must not contain `?`, `#`, `%`, or NUL; errors never echo the path. SQLite is opened with a filesystem path (not a concatenated `file:` URI and not `_pragma` query parameters). In production the file is created and opened relative to a verified real `/var/lib/redgres` directory; a lexical prefix check alone is not sufficient. Ancestor and intermediate directory components are inspected with `Lstat` and must not be symlinks; development and test paths still reject those components and must not create files outside a planted symlink. SQLite main/journal/WAL/SHM files and application secret files are opened as regular files with final-component no-symlink and file-identity checks. `OpenRegular` rejects `O_TRUNC`. Secret-file/systemd-credential values take precedence over secret environment variables; conflicting definitions fail closed.

## Startup validation

Production startup fails for default/empty admin values, non-loopback UI bind, insecure base URL, insecure remote Redis, unavailable secret files, permissive secret modes, impossible session durations, duplicate public endpoints, missing vault secret when legacy data exists, invalid protected lists, unsupported detected service versions, expected-version mismatches, or missing required service capabilities.

Validation errors identify the variable/path but never echo its value.
