# Source systems and behavioral inheritance

This document explains how the two supplied repositories work, what Redgres should inherit, and what must be corrected. It is based on source inspection, not live-host observation. Pin commits in [SOURCE_BASELINE.md](SOURCE_BASELINE.md) before relying on line-level behavior.

## 1. PostgreSQL source: `database-app`

Path: `D:\code\github\database-app`

### Stack and structure

- Python 3.11+, FastAPI, Uvicorn, Jinja2 server-rendered pages, psycopg 3, Starlette signed cookie sessions, and `cryptography` Fernet.
- Entry point: `app.py`.
- Configuration: `config/settings.py`; process environment wins over `.env.local`, which wins over `.env`.
- PostgreSQL access: `utils/database.py`; keyword DSNs avoid URL parsing problems for admin credentials.
- Domain operations: `modules/database_ops.py`, `security_ops.py`, `table_data.py`, `row_operations.py`.
- Vault: `modules/credential_vault.py`.
- Templates/static UI: `templates/`, `static/`.
- Existing operator references: `ONELIFE_DATABASE_PLATFORM_A_TO_Z.md` and `DEPLOYMENT.md`.

### Authentication and web behavior

- Static admin username/password come from environment.
- Login marks a signed cookie session as authenticated; session content is client-side signed, not encrypted server-side state.
- CSRF uses a random token stored in the signed session and sent in `X-CSRF-Token` for mutations.
- Production session cookie is HTTPS-only, SameSite Lax, with configurable max age.
- Security middleware sets nosniff, same-origin framing, referrer policy, and HSTS in production. It does not currently provide the stronger self-only CSP used by the Redis app.
- Some credential responses already set `Cache-Control: no-store`.

### PostgreSQL behavior to preserve

- Lists non-template, connectable databases while excluding `postgres` and the credential vault.
- Creates a restricted project login and project database, revokes PUBLIC CONNECT, grants owner connect, and returns direct/pooled URLs.
- Creates/reuses a dedicated PostgreSQL database named `database_console_vault` with PUBLIC access revoked.
- Stores generated role passwords in `public.project_credentials(role_name, encrypted_password, updated_at)`.
- GET connection metadata is masked; explicit POST reveal decrypts the saved password.
- Rotates eligible project-owner passwords and updates vault data.
- Shows database/security information, tables, paginated rows, and primary keys.
- Supports feature-flagged row delete, truncate, drop, and duplicate operations. Redgres duplicate is **not** behind `ENABLE_DESTRUCTIVE_ACTIONS` (PRD PG-010 is M4 provision; flags are M5 drop/truncate/row delete). Redgres row delete uses `REDGRES_FEATURE_POSTGRES_ROW_DELETE` only (default off). It does not copy sibling FastAPI no-CSRF, client `primary_key_column`, `schema.table::regclass` + `LIMIT 1`, HTTP 500 `str(e)`, or `{deleted, message}`. Server discovers a single-column PK from parameterized `information_schema` and fail-closes on composite/missing PK. AUTH-006 is in-handler `owner_password` (REDIS-008 pattern). Redgres truncate uses `REDGRES_FEATURE_POSTGRES_TRUNCATE` only (default off). It does not copy sibling FastAPI `DELETE /databases/{name}/data`, no-CSRF, `ENABLE_DESTRUCTIVE_ACTIONS`, per-table `CASCADE` with swallowed exceptions, `{truncated,total_tables,message}`, or HTTP 500 `str(e)`. One quoted `TRUNCATE TABLE … RESTART IDENTITY` of the GET-tables BASE TABLE set (RESTRICT default); truncated list fails closed; AUTH-006 is in-handler `owner_password`.
- Database duplication uses TEMPLATE cloning and per-object ownership transfer, avoiding `REASSIGN OWNED` against potentially shared source owners. Redgres keeps that isolation and does not copy the sibling client password, FastAPI 500/`str(e)`, or 202 operations envelope.
- Connection URL builder URL-encodes username/password/database and emits direct 5432 and pooled 6432 variants with TLS required.

### Exact legacy vault contract

The encryption key is:

```text
SHA-256(UTF-8("database-console-vault-v1:" + SESSION_SECRET))
```

The resulting 32 bytes are URL-safe base64 encoded and used as a Fernet key. Stored values are ASCII Fernet tokens. `SESSION_SECRET` therefore serves two roles: signed web sessions and vault key derivation. Redgres must reproduce this format exactly or perform a separately approved, reversible migration. It must not infer a different KDF, encoding, timestamp policy, or encryption format.

### Known gaps Redgres must not copy

- `drop_database` only explicitly blocks the vault. The final protected set must also block `postgres`, `template0`, `template1`, platform/admin databases, and operator-configured names.
- `list_databases` / `get_database_info` also under-protect: list only excludes `postgres` and the vault, and details can fetch `postgres` by name. Redgres inventory is stricter (hard-coded + configured names and owners, templates, `datallowconn=false`) and returns identical `404` for protected and missing details.
- The drop termination query does not exclude `pg_backend_pid()`; all termination logic must exclude the current backend.
- Static environment credentials and signed client-side session state are weaker than the Redis app’s Argon2id/server-side session model.
- Brute-force delay uses blocking `time.sleep(1)` and no durable per-IP/username rate-limit state.
- Destructive confirmation is inconsistent; the unified system must require target confirmation and reauthentication.
- Truncation loops and continues after errors, allowing partial success without a strong transaction/result contract.
- Large server-rendered templates and external styling assets make CSP, maintenance, and offline builds harder.
- Password rotation changes PostgreSQL then writes the vault; the two stores are not one atomic transaction. Redgres needs an explicit recoverable state machine/compensation behavior.

### Existing validation observed

On 2026-08-23, all 17 pytest tests passed. They cover URL construction/encoding, masked/reveal behavior, source isolation assumptions for duplicate, and some UI selection state. This is useful evidence, not enough for production parity; most PostgreSQL operations still require integration tests.

## 2. Redis source: `redis-ui` / Redact

Path: `D:\code\github\redis-ui`

### Stack and structure

- Go application using Chi, `go-redis/v9`, and `modernc.org/sqlite`.
- React 19 + TypeScript + Vite + Tailwind frontend embedded into the Go binary.
- Entry point and owner CLI: `cmd/redact/main.go`.
- Configuration: `internal/config/`; supports environment/flags and protected-file Redis admin URL.
- SQLite: `internal/database/`, `migrations/001_init.sql`.
- Auth: `internal/auth/`; Argon2id passwords, hashed opaque session and CSRF tokens, idle/absolute expiry, lockout records.
- HTTP: `internal/httpapi/server.go`; Chi routes, body limits, request IDs, CSP/security headers, same-origin and CSRF enforcement.
- Redis adapter: `internal/redisadmin/`.
- Audit: `internal/audit/`; metadata redaction checks.
- UI: `web/src/`; built output is embedded through `internal/web`.

### Authentication and state behavior to inherit

- CLI-only owner creation.
- Argon2id owner hashes in SQLite.
- Random session/CSRF tokens; only hashes stored in SQLite.
- One active owner session after login (existing sessions deleted), idle and absolute expiry.
- HttpOnly, SameSite Strict cookie with configurable Secure flag.
- Origin/Referer and CSRF validation for mutations, including origin validation for login.
- Persistent username+IP login-attempt records and 429 lockout behavior.
- SQLite tables for users, sessions, login attempts, and audit events.
- Strict JSON decode with unknown-field rejection and 64 KiB body limit.
- Request IDs, panic recovery, structured/secret-redacted errors, and strong self-only CSP.

### Redis behavior to preserve

- Redis status using PING, INFO sections, and DBSIZE.
- ACL list parsing and protected-user detection for `default`, `admin`, `redact_admin`, plus the configured admin identity.
- Username and normalized prefix validation; one `prefix:*` key pattern per managed user.
- Presets for cache read/write, read-only, and queue workers using Lists, Streams, or Sorted Sets.
- ACL creation starts from `reset`, uses one random 192-bit URL-safe password, resets channels, applies `-@all`, then explicit commands.
- Permission updates preserve the password; enable/disable preserves permissions; rotation uses `resetpass` and immediately invalidates prior passwords.
- Deletion requires exact username and owner-password reauthentication.
- The browser never talks to Redis directly; it receives only Redgres API data.
- Administrator URLs are rejected if plain Redis is remote unless an explicit private-network override is enabled; `rediss://` is accepted.

### Known gaps Redgres must correct

- Credential-bearing create/rotate responses do not currently set `Cache-Control: no-store` in the inspected server handler. Redgres enforces `Cache-Control: no-store` on every `/api/v1/*` response.
- The inspected server echoes inbound `X-Request-ID`. Redgres generates a 128-bit request ID and ignores the inbound header.
- Custom Redis commands currently reject a finite dangerous deny-list but accept other arbitrary command names. Redgres requires an explicit allow-list that fails closed.
- ACL category parsing does not expand Redis category rules returned by the server, so imported externally managed users may be represented incompletely. The UI/API must label this limitation rather than silently imply exact parity.
- Runtime/build artifacts (`redact.exe`, SQLite DB/WAL/SHM) exist in the supplied folder and must not be copied or committed.
- The supplied folder has no `.git`, so provenance and exact baseline must be established before code reuse.

### Existing validation observed

On 2026-08-23, all Go tests, `go vet`, all 18 frontend tests, and a frontend production build passed. Tests cover auth/session/CSRF, config fail-closed behavior, secret redaction, key-prefix validation, presets, API guards, components, and docs. Integration with a real Redis 8/TLS instance remains a migration gate.

## 3. Inheritance decision

| Concern | Source of truth for first Redgres implementation |
|---|---|
| Go module/application skeleton | Redis app, renamed and reorganized |
| React UI/build/embed pipeline | Redis app, redesigned for unified navigation |
| Owner auth, sessions, CSRF, login lockout | Redis app |
| Audit model and redacted errors | Redis app, expanded for PostgreSQL |
| Redis ACL use cases | Redis app, after no-store and allow-list fixes |
| PostgreSQL operation semantics | Python app, ported through explicit use cases |
| PostgreSQL vault compatibility | Python vault implementation and production-compatible fixtures |
| Deployment and operational truth | Live inventory + both A-to-Z runbooks, reconciled into Redgres runbooks |

Redgres should be a modular merger of capabilities, not a directory concatenation and not a line-by-line Python translation.

## 4. Source review checklist for agents

Before porting a feature:

1. Pin the source commit/snapshot.
2. Read the complete source function, callers, frontend use, tests, and runbook behavior.
3. Write characterization tests for behavior that must survive.
4. Record intentional behavior changes in the PRD/ADR.
5. Implement through a Redgres domain interface.
6. Run unit plus real-service integration tests.
7. Add traceability evidence.
