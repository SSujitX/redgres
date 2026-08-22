# Configuration reference

This is the target namespace; implementation must generate a machine-checked reference from the actual config struct and keep this document synchronized.

## Core

| Variable | Required in production | Example | Rule |
|---|---:|---|---|
| `REDGRES_ENVIRONMENT` | Yes | `production` | Controls fail-closed validation, not authorization |
| `REDGRES_ADDRESS` | Yes | `127.0.0.1:8790` | Production must be loopback unless an ADR approves a trusted private bind |
| `REDGRES_BASE_URL` | Yes | `https://console.onelifeltd.xyz` | Exact public origin for cookie/origin checks |
| `REDGRES_SQLITE_PATH` | Yes | `/var/lib/redgres/redgres.db` | Absolute path in production |
| `REDGRES_SESSION_TTL` | No | `12h` | Idle expiry with safe min/max |
| `REDGRES_ABSOLUTE_SESSION_TTL` | No | `24h` | Must be >= idle expiry and bounded |
| `REDGRES_COOKIE_SECURE` | Yes | `true` | Must be true in production |
| `REDGRES_LOG_LEVEL` | No | `info` | Never enables secret/body logging |

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
| `REDGRES_POSTGRES_PROTECTED_DATABASES` | Additional comma-separated deny set |
| `REDGRES_POSTGRES_PROTECTED_ROLES` | Additional deny set |
| `REDGRES_LEGACY_VAULT_SECRET_FILE` | Exact legacy KDF secret source |

Never accept a full admin DSN on the CLI. If a DSN file is supported, parse/redact it and give it the same protection as a password.

## Redis

| Variable | Purpose |
|---|---|
| `REDGRES_REDIS_ADMIN_URL_FILE` | Protected `redis://`/`rediss://` admin URL source |
| `REDGRES_REDIS_PUBLIC_HOST` | Host placed in project URLs |
| `REDGRES_REDIS_PUBLIC_PORT` | Usually TLS 6380 |
| `REDGRES_REDIS_ALLOW_PLAINTEXT` | False by default; true only for explicit loopback/private path |
| `REDGRES_REDIS_SUPPORTED_MAJOR` | Compatibility guard, initially 8 |

Plain `redis://` to non-loopback is rejected unless the explicit private-path override is true. Public generated URLs use `rediss://`.

## UI/tool links

- `REDGRES_PGADMIN_URL`
- `REDGRES_REDISINSIGHT_URL`

Both are optional and must be validated `https://` URLs in production. They are links, not embedded privileged sessions.

## Feature gates

- `REDGRES_FEATURE_POSTGRES_DROP=false`
- `REDGRES_FEATURE_POSTGRES_TRUNCATE=false`
- `REDGRES_FEATURE_POSTGRES_ROW_DELETE=false`

Enabling a flag makes the server-side workflow reachable; it never bypasses capabilities, protected targets, CSRF, confirmation, reauthentication, or audit.

## Precedence

Recommended precedence: explicit flags (non-secret) > process environment > optional development `.env` > defaults. Production does not automatically read repository `.env` files. Secret-file/systemd-credential values take precedence over secret environment variables; conflicting definitions fail closed.

## Startup validation

Production startup fails for default/empty admin values, non-loopback UI bind, insecure base URL, insecure remote Redis, unavailable secret files, permissive secret modes, impossible session durations, duplicate public endpoints, missing vault secret when legacy data exists, or invalid protected lists.

Validation errors identify the variable/path but never echo its value.
