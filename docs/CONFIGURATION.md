# Configuration reference

Core keys, the PostgreSQL administrative connection keys, and the Redis URL-file / plaintext-override / optional public host-port keys listed as implemented below are loaded by `internal/config`. Remaining Redis, feature-gate, tool-link, URL-generation, and vault-secret keys remain target. Machine-checked reference generation from the config struct is still outstanding.

## Core

Status: implemented in `internal/config`.

| Variable | Required in production | Example | Rule |
|---|---:|---|---|
| `REDGRES_ENVIRONMENT` | Yes | `production` | `development` or `production`; controls fail-closed validation, not authorization |
| `REDGRES_ADDRESS` | Yes | `127.0.0.1:8790` | Production must be loopback unless an ADR approves a trusted private bind |
| `REDGRES_BASE_URL` | Yes | `https://console.onelifeltd.xyz` | Exact **browser page origin** for cookie/origin checks; production must be `https`. This is not the listen address. |
| `REDGRES_SQLITE_PATH` | Yes | `/var/lib/redgres/redgres.db` | Absolute path in production |
| `REDGRES_SESSION_TTL` | No | `12h` | Idle expiry; minimum 5m, maximum 24h |
| `REDGRES_ABSOLUTE_SESSION_TTL` | No | `24h` | Must be >= idle expiry and at most 168h |
| `REDGRES_COOKIE_SECURE` | Yes | `true` | Must be true in production |
| `REDGRES_LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, or `error`. Never enables secret/body logging |
| `REDGRES_DEV_ASSET_DIR` | No (rejected) | `./internal/web/dist/app` | Development only; must be an existing directory; rejected in production; must not contain `?`, `#`, or NUL. Empty selects the embedded assets |

## PostgreSQL

Status: administrative connection + protected lists implemented for inventory. Optional pooled-observation port implemented for GET `/api/v1/status` PgBouncer Ping. URL hosts/direct-port and vault secret remain target.

| Variable | Status | Purpose |
|---|---|---|
| `REDGRES_POSTGRES_HOST` / `PORT` / `DATABASE` / `USER` | Implemented | Direct administrative connection components. Never a full admin DSN on the CLI. |
| `REDGRES_POSTGRES_PASSWORD_FILE` | Implemented | Only password source. Production rejects missing, empty, or group/world-readable files. |
| `REDGRES_POSTGRES_SSLMODE` | Implemented | Development default `prefer`. Production requires `require`, `verify-ca`, or `verify-full`. |
| `REDGRES_POSTGRES_SSLROOTCERT` | Implemented | Trusted CA path when verifying |
| `REDGRES_POSTGRES_EXPECTED_MAJOR` | Implemented | Optional identity assertion (`17` or `18`); detected `server_version_num` remains authoritative |
| `REDGRES_POSTGRES_PROTECTED_DATABASES` | Implemented | Additional comma-separated deny set (plus hard-coded `postgres`, `template0`, `template1`, `database_console_vault`, and the admin catalog database) |
| `REDGRES_POSTGRES_PROTECTED_ROLES` | Implemented | Additional owner deny set (plus hard-coded admin/builtin roles and `pg_*`) |
| `REDGRES_POSTGRES_PUBLIC_HOST` | Target | Host placed in project URLs |
| `REDGRES_POSTGRES_DIRECT_PORT` | Target | Usually 5432 |
| `REDGRES_POSTGRES_POOLED_PORT` | Implemented | Optional TCP port (1–65535) for PgBouncer console observation on GET `/api/v1/status`. No silent default. Production `serve` does not require it. Set only when the administrative PostgreSQL connection is otherwise configured (same fail-closed partial-key rule as other `REDGRES_POSTGRES_*` keys). Host, user, password file, and TLS settings are the existing admin keys (`REDGRES_POSTGRES_HOST`, not a public URL host). The admin role must be in PgBouncer `admin_users` or `stats_users`. Never a second password file, `pgbouncer_auth`, or `userlist.txt`. |
| `REDGRES_LEGACY_VAULT_SECRET_FILE` | Target | Exact legacy KDF secret source |

Development may start without PostgreSQL; list/details then return `503` `dependency_unavailable` and do not fabricate an empty healthy cluster. Production `serve` fails closed if the administrative connection is incomplete or the password file is unusable. Production `serve` does not require `REDGRES_POSTGRES_POOLED_PORT`. Connecting **to** the `postgres` catalog database is required; listing or detailing that database is forbidden.

These are application connection settings, not authority to install PostgreSQL or change extensions. Installer-only lifecycle values such as `POSTGRES_MODE`, `POSTGRES_MAJOR`, `PGBOUNCER_MODE`, `POSTGRES_EXTENSION_POLICY`, and `POSTGRES_EXTENSION_PLAN_FILE` live in the protected install configuration and are validated under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). The application environment never accepts package names, repositories, arbitrary extension SQL or preload libraries.

Never accept a full admin DSN on the CLI. If a DSN file is supported, parse/redact it and give it the same protection as a password.

## Redis

Status: administrator URL file + plaintext override implemented for Ping health, GET `/api/v1/redis/status` metrics, ACL inspect, and POST `/api/v1/redis/users` create. `skip_verify` is rejected. Optional public host/port are implemented for generated project URLs. Expected series remains target.

| Variable | Status | Purpose |
|---|---|---|
| `REDGRES_REDIS_ADMIN_URL_FILE` | Implemented | Path to a file containing one `redis://` or `rediss://` admin URL. File path only; a raw URL as the env value is rejected. No `REDGRES_REDIS_ADMIN_URL` fallback. Production `serve` fails closed if the file is missing, empty, unreadable, or group/world-readable. Development may start without Redis; GET `/status` and GET `/api/v1/redis/status` then report `not_configured`. `skip_verify=true` / `1` (go-redis `ParseURL` `InsecureSkipVerify` on `rediss`) is rejected in every environment as `REDGRES_REDIS_ADMIN_URL_FILE: invalid value`; the error never echoes the URL or password. |
| `REDGRES_REDIS_ALLOW_PLAINTEXT` | Implemented | Default false. `redis://` to non-loopback requires true. Loopback `redis://` is allowed without it. `rediss://` is always accepted. |
| `REDGRES_REDIS_PUBLIC_HOST` | Implemented | Optional host placed in created project `rediss://` URLs. Production `serve` does not require it. When set, the value must not contain whitespace, `@`, `/`, or a URI scheme. |
| `REDGRES_REDIS_PUBLIC_PORT` | Implemented | Optional TCP port (1–65535) placed in created project URLs. No silent default. `credential.urls` is omitted unless both this and `REDGRES_REDIS_PUBLIC_HOST` are set. Production `serve` does not require it. |
| `REDGRES_REDIS_EXPECTED_SERIES` | Target | Optional identity assertion (`8.2` or `8.8` initially); detected version remains authoritative |

Plain `redis://` to non-loopback is rejected unless the explicit private-path override is true. Public generated URLs use `rediss://`. Validation errors name the environment variable and never echo the URL, userinfo, or host from the secret file.

Supported service versions are defined by the Redgres release and [COMPATIBILITY.md](COMPATIBILITY.md), not by environment configuration. Expected-version settings detect connection to the wrong server; they cannot make an unsupported version supported. PostgreSQL is detected with `SHOW server_version_num`, Redis from `INFO server`, and PgBouncer with `SHOW VERSION`, followed by required capability checks.

## UI/tool links

- `REDGRES_PGADMIN_URL`
- `REDGRES_REDISINSIGHT_URL`

Both are optional and must be validated `https://` URLs in production. They are links, not embedded privileged sessions.

## Feature gates

- `REDGRES_FEATURE_POSTGRES_DROP=false`
- `REDGRES_FEATURE_POSTGRES_TRUNCATE=false`
- `REDGRES_FEATURE_POSTGRES_ROW_DELETE=false`

Enabling a flag makes the server-side workflow reachable; it never bypasses capabilities, protected targets, CSRF, confirmation, reauthentication, or audit.

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

Recommended precedence: explicit flags (non-secret) > process environment > optional development `.env` > defaults. Production does not read repository `.env` files, including when selected with `-environment production`. A `.env` that sets `REDGRES_ENVIRONMENT=production` is rejected. Dotenv applies only `REDGRES_*` keys and never overwrites an already-set process variable. `REDGRES_BASE_URL` must be an origin (`scheme://host[:port]`) with no userinfo, path, query, or fragment. `REDGRES_SQLITE_PATH` must not contain `?`, `#`, or NUL. Secret-file/systemd-credential values take precedence over secret environment variables; conflicting definitions fail closed.

## Startup validation

Production startup fails for default/empty admin values, non-loopback UI bind, insecure base URL, insecure remote Redis, unavailable secret files, permissive secret modes, impossible session durations, duplicate public endpoints, missing vault secret when legacy data exists, invalid protected lists, unsupported detected service versions, expected-version mismatches, or missing required service capabilities.

Validation errors identify the variable/path but never echo its value.
