# Configuration reference

Wave 0 implements the Core table only. PostgreSQL, Redis, feature-gate, and tool-link keys remain target. Machine-checked reference generation from the config struct is still outstanding.

## Core

Status: implemented in `internal/config`.

| Variable | Required in production | Example | Rule |
|---|---:|---|---|
| `REDGRES_ENVIRONMENT` | Yes | `production` | `development` or `production`; controls fail-closed validation, not authorization |
| `REDGRES_ADDRESS` | Yes | `127.0.0.1:8790` | Production must be loopback unless an ADR approves a trusted private bind |
| `REDGRES_BASE_URL` | Yes | `https://console.onelifeltd.xyz` | Exact public origin for cookie/origin checks; production must be `https` |
| `REDGRES_SQLITE_PATH` | Yes | `/var/lib/redgres/redgres.db` | Absolute path in production |
| `REDGRES_SESSION_TTL` | No | `12h` | Idle expiry; minimum 5m, maximum 24h |
| `REDGRES_ABSOLUTE_SESSION_TTL` | No | `24h` | Must be >= idle expiry and at most 168h |
| `REDGRES_COOKIE_SECURE` | Yes | `true` | Must be true in production |
| `REDGRES_LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, or `error`. Never enables secret/body logging |

## PostgreSQL

| Variable | Purpose |
|---|---|
| `REDGRES_POSTGRES_HOST` / `PORT` / `DATABASE` / `USER` | Direct administrative connection components |
| `REDGRES_POSTGRES_PASSWORD_FILE` | Password file/systemd credential path |
| `REDGRES_POSTGRES_SSLMODE` | `verify-full` preferred remotely; local deployment policy explicit |
| `REDGRES_POSTGRES_SSLROOTCERT` | Trusted CA path when verifying |
| `REDGRES_POSTGRES_PUBLIC_HOST` | Host placed in project URLs |
| `REDGRES_POSTGRES_DIRECT_PORT` | Usually 5432 |
| `REDGRES_POSTGRES_POOLED_PORT` | Usually 6432 |
| `REDGRES_POSTGRES_EXPECTED_MAJOR` | Optional identity assertion (`17` or `18` initially); detected version remains authoritative |
| `REDGRES_POSTGRES_PROTECTED_DATABASES` | Additional comma-separated deny set |
| `REDGRES_POSTGRES_PROTECTED_ROLES` | Additional deny set |
| `REDGRES_LEGACY_VAULT_SECRET_FILE` | Exact legacy KDF secret source |

These are application connection settings, not authority to install PostgreSQL or change extensions. Installer-only lifecycle values such as `POSTGRES_MODE`, `POSTGRES_MAJOR`, `PGBOUNCER_MODE`, `POSTGRES_EXTENSION_POLICY`, and `POSTGRES_EXTENSION_PLAN_FILE` live in the protected install configuration and are validated under [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). The application environment never accepts package names, repositories, arbitrary extension SQL or preload libraries.

Never accept a full admin DSN on the CLI. If a DSN file is supported, parse/redact it and give it the same protection as a password.

## Redis

| Variable | Purpose |
|---|---|
| `REDGRES_REDIS_ADMIN_URL_FILE` | Protected `redis://`/`rediss://` admin URL source |
| `REDGRES_REDIS_PUBLIC_HOST` | Host placed in project URLs |
| `REDGRES_REDIS_PUBLIC_PORT` | Usually TLS 6380 |
| `REDGRES_REDIS_ALLOW_PLAINTEXT` | False by default; true only for explicit loopback/private path |
| `REDGRES_REDIS_EXPECTED_SERIES` | Optional identity assertion (`8.2` or `8.8` initially); detected version remains authoritative |

Plain `redis://` to non-loopback is rejected unless the explicit private-path override is true. Public generated URLs use `rediss://`.

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

Recommended precedence: explicit flags (non-secret) > process environment > optional development `.env` > defaults. Production does not read repository `.env` files, including when selected with `-environment production`. A `.env` that sets `REDGRES_ENVIRONMENT=production` is rejected. Dotenv applies only `REDGRES_*` keys and never overwrites an already-set process variable. `REDGRES_BASE_URL` must be an origin (`scheme://host[:port]`) with no userinfo, path, query, or fragment. `REDGRES_SQLITE_PATH` must not contain `?`, `#`, or NUL. Secret-file/systemd-credential values take precedence over secret environment variables; conflicting definitions fail closed.

## Startup validation

Production startup fails for default/empty admin values, non-loopback UI bind, insecure base URL, insecure remote Redis, unavailable secret files, permissive secret modes, impossible session durations, duplicate public endpoints, missing vault secret when legacy data exists, invalid protected lists, unsupported detected service versions, expected-version mismatches, or missing required service capabilities.

Validation errors identify the variable/path but never echo its value.
