# Target architecture

Status: target; implementation evidence must be recorded separately.

## 1. System context

```text
Operator browser
  │ HTTPS + Cloudflare Access
  ▼
Cloudflare edge ── remotely managed Tunnel ──► 127.0.0.1:8790 Redgres (migration)
                                                   │
                         ┌─────────────────────────┼─────────────────────────┐
                         ▼                         ▼                         ▼
                  SQLite control state      PostgreSQL admin path      Redis admin path
                  /var/lib/redgres/          local/direct 5432          local 6379 or TLS
                  redgres.db                 PgBouncer observed 6432    public service 6380

Application clients ── DNS-only + TLS ──► db.onelifeltd.xyz:5432/6432
Application clients ── DNS-only + TLS ──► rs.onelifeltd.xyz:6380
```

Cloudflare Tunnel protects HTTP consoles; it is not the ordinary database transport. The raw endpoints remain real network attack surfaces and require TLS, authentication, allow rules, firewall restrictions, monitoring, and patching.

## 2. Deployment evolution

### Coexistence phase

- Legacy PostgreSQL FastAPI console: `127.0.0.1:6969`, `database.onelifeltd.xyz`.
- Legacy Redis/Redact Go console: `127.0.0.1:8787`, `redis-admin.onelifeltd.xyz`.
- New Redgres staging port: `127.0.0.1:8790` (recommended during migration).
- New Redgres hostname: `console.onelifeltd.xyz`.

Do not bind both Redact and Redgres to 8787 during coexistence. After retirement, Redgres may take 8787, but keeping 8790 is also acceptable if documented.

### Final phase

- One `redgres` Go binary serves `/api/v1/*` and embedded React assets.
- One SQLite database stores owner/session/audit/operation state only.
- A [supported PostgreSQL selection](COMPATIBILITY.md) remains host-native with PgBouncer host-native.
- A [supported Redis selection](COMPATIBILITY.md) remains Docker-managed with persistent host volumes.
- pgAdmin and RedisInsight remain optional, isolated tools.

## 3. Modular monolith

```text
HTTP transport
  ├── auth endpoints ─────► auth service ─────► control-state repository (SQLite)
  ├── audit endpoints ────► audit service ────► control-state repository (SQLite)
  ├── status endpoint ────► platform.Collect ──► SQLite ping + postgresadmin.Ping + redisadmin.Ping
  ├── redis status endpoint ► redisadmin.Status ──► Ping RTT + INFO sections + DBSIZE
  ├── redis users endpoints ► redisadmin.ListUsers/GetUser/CreateUser/UpdatePermissions/SetEnabled/RotateUser ──► ACL LIST + ACL SETUSER
  ├── redis presets endpoint ► redisadmin.NamedPresets (static catalog; no Redis)
  ├── redis commands endpoint ► redisadmin.AllowedCommands (static allow-list; no Redis)
  ├── search endpoint ────► platform.ResourceGroups ► postgresadmin.Search names + redisadmin.Search names
  ├── postgres endpoints ─► postgres use cases ► PostgreSQL adapter (pgxpool)
  │                              └──────────────► vault adapter (PostgreSQL/Fernet)
  └── redis endpoints ────► redis use cases ───► Redis ACL adapter (go-redis)
```

Dependency direction is inward: transport depends on use cases; use cases depend on interfaces; infrastructure adapters implement interfaces. `postgresadmin` and `redisadmin` do not import each other. Cross-system dashboard aggregation belongs in a platform/status service.

`internal/platform.Collect` is that aggregator: it probes Redgres state, PostgreSQL direct, PgBouncer, and Redis through `PingFunc` values and a config-presence flag (`Collect(ctx, statePing, postgresPing, pgbouncerPing, redisPing, toolLinksConfigured bool)`). Redis and PgBouncer mapping equal `postgres_direct`: nil ping or `platform.ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Collect remains Ping-only for Redis (no INFO, DBSIZE, Ping RTT metrics, or ACL LIST). PgBouncer ping is `SHOW VERSION` on a pooled-observation pool (dbname `pgbouncer`, `DefaultQueryExecMode = QueryExecModeSimpleProtocol`, not `QueryExecModeExec`, not `pgxpool.Ping`); success is no error, the version string is discarded (do not `Scan`), and the component must not emit `not_implemented`. `tool_links` is not a PingFunc and is never HTTP-fetched: both optional expert URLs empty → `not_configured`; one or both set → `ok`; never `unavailable` or `not_implemented`. GET `/status` still never includes URLs; hrefs are GET `/session` only. `platform` still does not import `postgresadmin`. The pooled pool is not used for catalog SQL, is not startup-Pinged, and must not fail `serve` when 6432 is down. Direct admin `pgxpool` stays extended protocol on 5432. `platform.ResourceGroups` is the bounded search aggregator: HTTP maps `postgresadmin.Search` and `redisadmin.Search` name lists (or `not_configured` / `unavailable`) into postgres and Redis groups. Hits are emitted only when that group's status is `ok`. Navigation and documentation are not server search results. `platform` does not import `postgresadmin` or `redisadmin`; the HTTP layer maps adapter sentinels onto `platform.ErrNotConfigured`. GET `/api/v1/search` calls `redisadmin.Service.Search`, which reuses `ListUsers`/`loadUsers` (`ACL LIST` only) and omits protected usernames; it does not call `ACL GETUSER`, `ACL SETUSER`, or `Do`. `redisadmin.Open` does not Ping when a URL is configured: `go-redis` `NewClient` is lazy, so an unreachable Redis must not block `serve`. GET `/api/v1/status` reports `unavailable` instead. GET `/api/v1/redis/status` calls `redisadmin.Service.Status` (Ping wall-clock RTT, `INFO server clients memory stats`, and `DBSIZE` of the URL-selected database) and does not call ACL LIST. GET `/api/v1/redis/users` and GET `/api/v1/redis/users/{username}` call `ACLList` only (go-redis `ACLList(ctx)` → `ACL LIST`); they do not call `ACL GETUSER`, `ACL USERS`, or `Do`. GET `/api/v1/redis/presets` calls `redisadmin.NamedPresets()` only: it does not use the Redis adapter, `ACLList`, `ACLSetUser`, Ping, INFO, or DBSIZE, and it succeeds when the adapter is nil. GET `/api/v1/redis/commands` calls `redisadmin.AllowedCommands()` only (unique-sorted union of `NamedPresets()[].Commands`); it does not use the Redis adapter and succeeds when the adapter is nil. POST `/api/v1/redis/users` calls `redisadmin.Service.CreateUser(ctx, username, keyPattern, preset, queueKind string, commands []string)` (empty `preset` means `cache-read-write`; named/empty-preset rejects non-empty `commands` before Redis; `preset` `custom` uses `resolveCustomCommands` / `AllowedCommands()` with empty `queueKind` and `inferPreset` for the result labels), which issues `ACLList` for an exact-username duplicate check, then one `ACLSetUser` (`ACL SETUSER`) with `reset`, `on`, a generated password, one `~prefix:*`, `resetchannels`, `-@all`, and explicit `+CMD` grants from the matching named inspect set (`cache-read-write`, `read-only`, or `queue-worker` lists/streams/sorted-sets) or the custom allow-list. There is no `+@all` and no deny-list path. POST enable/disable call `GetUser` then one `ACLSetUser` with only `on` or `off`. POST `/api/v1/redis/users/{username}/credentials/rotate` generates the password inside `redisadmin.Service.RotateUser` (`GeneratePassword`, 24 bytes, `base64.RawURLEncoding`) — not the HTTP layer — then calls `GetUser` and one `ACLSetUser` with only `resetpass` and `>password` (Redis `ACL SETUSER` `resetpass` clears passwords/`nopass`; `>password` adds the new secret; `ACL SETUSER` upserts, so `GetUser` is mandatory). These paths do not call `ACL GETUSER`, `ACL USERS`, `ACLGenPass`, `CLIENT KILL`, or `Do`. After `ParseURL`, `Open` captures `opts.Username` for protected-user comparison, discards the admin URL, and rejects `rediss` URLs whose `TLSConfig.InsecureSkipVerify` is true (go-redis `skip_verify=true`) in every environment and does not create a client.

## 4. Backend stack

- Go: `go 1.27.0` in `go.mod` (installed/local and CI via `go-version-file`). Official [Go 1.27 release notes](https://go.dev/doc/go1.27) (2026-08) keep the Go 1 compatibility promise. Wave 0 originally considered `go 1.26.7` as the previous-line newest patch; the operator installed 1.27 and Wave 0 builds/tests passed against it with `modernc.org/sqlite` v1.57.0 and `chi` v5.3.2.
- Router: `github.com/go-chi/chi/v5` `v5.3.2`.
- PostgreSQL: `github.com/jackc/pgx/v5` `v5.10.0` (`pgxpool`). Pin verified 2026-08-23 via `go list -m -versions` / `go list -m -json` against proxy.golang.org (newest stable; Go ≥1.25; MIT). Inventory lives in `internal/postgresadmin`; GET paths do not decrypt vault rows. POST `/connection/reveal` decrypts ciphertext via `internal/secrets` after Open derives the Fernet key. Catalog list/details use the admin `pgxpool`. Optional PgBouncer observation uses a separate pooled pool (dbname `pgbouncer`, `QueryExecModeSimpleProtocol` on pgx v5.10.0) for `Inventory.PingPooled` only; it is not startup-Pinged, does not `checkServerMajor`, and is not used for list/details/security/tables/rows. Frozen `GET /api/v1/postgres/databases/{db}/connection` (session + `postgres.read`) reuses `SavedRoleNames` existence only and builds masked URLs from `REDGRES_POSTGRES_PUBLIC_HOST` plus `REDGRES_POSTGRES_DIRECT_PORT` and/or `REDGRES_POSTGRES_POOLED_PORT`; it never copies the administrative host or port and does not import `internal/secrets`. `Inventory.Connection` returns identity plus `saved_credential` only; the HTTP handler omits or emits `masked_*` from `s.cfg`. `MaskedProjectConnectionURL` percent-encodes owner/database (RFC 3986 unreserved) and hardcodes `sslmode=require`. Frozen `POST /api/v1/postgres/databases/{db}/connection/reveal` (session + `postgres.credentials` + CSRF) is registered next to GET `/connection`. Open reads optional `REDGRES_LEGACY_VAULT_SECRET_FILE` when PostgreSQL is configured, stores the derived Fernet key on the service, and wipes the raw secret. `Catalog` adds `EncryptedPassword(ctx, roleName)` (`SELECT encrypted_password FROM public.project_credentials WHERE role_name = $1`). `Inventory.Reveal` decrypts with no TTL and builds public-host URLs with `encodeRFC3986Unreserved` (`one_time: false`). GET `/connection` still uses `SavedRoleNames` only and does not import `internal/secrets`. `GET /api/v1/postgres/security` and details call `postgresadmin.Service.SecurityOverview` / `Details` on the admin `pgxpool` (`Catalog.List` plus `ListConnectionGroups` from `pg_stat_activity`, plus `Catalog.SavedRoleNames`). Security includes protected non-template names and does not change list/details `Manageable` filtering. `Service.SecurityOverview` derives `rotation_eligible` on each `databases[]` row from owner, `protected`, `owner_can_login`, and `owner_is_superuser` (no new catalog SQL; never sibling `can_rotate`). Frozen `POST /api/v1/postgres/databases` (session + `postgres.provision` + CSRF) is registered next to GET list. `Inventory.Create` generates a 24-byte RawURLEncoding password (duplicate of Redis; packages must not import each other), runs `CREATE ROLE` (`LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 20`), optional `GRANT … WITH INHERIT TRUE, SET TRUE`, `CREATE DATABASE` outside a transaction, CONNECT lock, `secrets.Encrypt`, and vault INSERT (`role_name`, `encrypted_password`, `updated_at`; no upsert; no `ensure_vault`). Compensation deletes only this operation’s vault row / database / role. `Catalog` may add insert/delete/exists helpers owned by the API writer. Frozen `POST /api/v1/postgres/databases/{db}/credentials/rotate` (session + `postgres.credentials` + CSRF) is registered before GET `/{db}`. `Inventory.Rotate` generates the same password helper, encrypts, ALTER ROLE with CONNECTION LIMIT 20, and vault upsert (`ON CONFLICT (role_name) DO UPDATE`; retries 3; no re-ALTER). Create INSERT stays without upsert. `Catalog` may add `AlterRolePassword` and `UpsertCredential`. No `ensure_vault`. `SavedRoleNames` reuses `connectTarget` to `database_console_vault` (`search_path=pg_catalog,information_schema,pg_temp`) and runs `SELECT role_name FROM public.project_credentials WHERE role_name = ANY($1)` with a non-nil `[]string` (pgx v5.10.0 `TryWrapSliceEncodePlan` encodes `[]string` as `text[]`; see `pgtype/pgtype.go` in `github.com/jackc/pgx/v5` v5.10.0). The query does not mention `encrypted_password` or `updated_at`. Empty `roles` returns an empty map without connecting. Connect/query/timeout failure, including PostgreSQL `3D000` and `42P01`, is `ErrVaultUnavailable` (never `ErrUnavailable`) and becomes HTTP 200 `not_available`/`vault_unavailable` with existing databases/connections; catalog List/connection-group failure remains 503. `internal/secrets` is not imported. Table list opens a short-lived `pgx.ConnectConfig` to the target database (copied pool `ConnConfig` with `Database` replaced and `search_path=pg_catalog,information_schema,pg_temp` on a cloned `RuntimeParams` map) and closes it after the query; it does not run `information_schema` on the admin catalog database. The adapter query is `LIMIT 501`; the service still returns at most 500 rows. Row browse reuses that short-lived target connect and `search_path`, quotes identifiers with `pgx.Identifier`, and pages with offset/limit (default 50, clamp 500). Vault decrypt is not used on list/details/security/tables/rows/GET connection.
- Redis: `github.com/redis/go-redis/v9` `v9.22.0` (BSD-2-Clause). Pin verified 2026-08-25 via `go list -m -versions` / `go list -m -json` against proxy.golang.org (newest non-prerelease v9; GitHub tag 2026-08-03, https://github.com/redis/go-redis). Only `internal/redisadmin` imports it, and only `ParseURL` / `NewClient` / `Ping` / `Info` / `DBSize` / `ACLList` / `ACLSetUser` / `ACLDelUser` / `SetLogger` are used. `SetLogger` runs once from `init` (not from `Open`) so a leftover pool dial cannot race a process-wide logger write. `ACLList` is `NewStringSliceCmd(ctx, "acl", "list")` and `ACLSetUser` is `NewStatusCmd(ctx, "acl", "setuser", username, rules...)` in go-redis `acl_commands.go`. `ACLDelUser` is `ACLDelUser(ctx, username string) *IntCmd` wrapping `.Result()` (official Redis `ACL DELUSER`). `Open` does not Ping; GET `/api/v1/status` uses Ping only. GET `/api/v1/redis/status` uses Ping RTT, `Info(ctx, "server", "clients", "memory", "stats")`, and `DBSize`. GET `/api/v1/redis/users`, GET `/api/v1/redis/users/{username}`, and GET `/api/v1/search` Redis hits use `ACLList` only (`redisadmin.Search` reuses `ListUsers`). GET `/api/v1/redis/presets` does not call Redis (`NamedPresets` only). GET `/api/v1/redis/commands` does not call Redis (`AllowedCommands` only). POST `/api/v1/redis/users` uses `CreateUser(ctx, username, keyPattern, preset, queueKind string, commands []string)` then `ACLList` then one `ACLSetUser`. Custom create expands only through `AllowedCommands()` before Redis (no deny-list). PATCH `/api/v1/redis/users/{username}` uses `UpdatePermissions(ctx, username, keyPattern, preset, queueKind string, commands []string)`: grant resolution through named inspect sets or `AllowedCommands()` happens before Redis; then `GetUser` (`ACLList`) then one `ACLSetUser` with `resetkeys`, `~prefix:*`, `resetchannels`, `nocommands`, `-@all`, and `+CMD` (no `reset`/`resetpass`/`>`/`on`/`off`). Custom PATCH is fail-closed allow-list only (no deny-list, no categories). POST `/api/v1/redis/users/{username}/enable` and `/disable` use `GetUser` (`ACLList`) then one `ACLSetUser` with only `on` or `off` (not a rule rewrite). POST `/api/v1/redis/users/{username}/credentials/rotate` uses `GetUser` then one `ACLSetUser` with only `resetpass` and `>password` (official Redis `ACL SETUSER` rules: https://redis.io/commands/acl-setuser). Frozen `DELETE /api/v1/redis/users/{username}` (session + `redis.destructive` + CSRF) uses in-handler AUTH-006 then `GetUser` (`ACL LIST`) then one `ACLDelUser`; it does not call `ACL SETUSER`, `CLIENT KILL`, or generic `Do`. After `ParseURL`, `TLSConfig.InsecureSkipVerify` (go-redis `skip_verify=true` on `rediss`, see `options.go`) is rejected in every environment. `ACL GETUSER`, `ACL USERS`, generic `Do`, `ACLGenPass`, `CLIENT KILL`, and `REDGRES_REDIS_EXPECTED_SERIES` are not in this slice.
- SQLite: `modernc.org/sqlite` `v1.57.0`.
- Passwords: `golang.org/x/crypto` `v0.55.0` (`argon2.IDKey`, version `0x13`). Interactive owner bootstrap uses `golang.org/x/term` `v0.45.0`. Both are official `go.googlesource.com` modules; `openpgp` is not imported.
- Fernet (PG-005 Partial in-process gate): `internal/secrets` is a small audited stdlib package (`crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/sha256`, `crypto/subtle`, `encoding/base64`). It does not add a Go module. `github.com/fernet/fernet-go` is not used (no tagged stable pin; `VerifyAndDecrypt` returns nil without a typed error). `golang.org/x/crypto` remains Argon2id-only at `v0.55.0`. KDF is SHA-256(UTF-8(`database-console-vault-v1:` + secret)) then URL-safe Base64, matching sibling `credential_vault._cipher` at `1c3e8e2`. Decrypt applies no TTL (Python `Fernet.decrypt` without `ttl`; official [cryptography Fernet.decrypt](https://cryptography.io/en/latest/fernet/) `ttl=None` does not consider age). Fixtures are committed ASCII and Unicode tokens in `internal/secrets/testdata/python49.json`, generated with Python `cryptography==49.0.0` (sibling `uv.lock`; PyPI 50.0.0 changelog has no Fernet recipe change; official [Fernet spec](https://github.com/fernet/spec/blob/master/Spec.md) and pinned [cryptography 49.0.0 fernet.py](https://github.com/pyca/cryptography/blob/49.0.0/src/cryptography/fernet.py)). The package does not read env, config, PostgreSQL, or HTTP. HTTP vault existence SQL lives in `postgresadmin` (`SavedRoleNames`) and does not import this package. POST reveal calls `DeriveVaultKey`/`Decrypt` from `postgresadmin` after Open loads `REDGRES_LEGACY_VAULT_SECRET_FILE`. PG-003 create adds `secrets.Encrypt` (Fernet inverse of Decrypt; no TTL; no `fernet-go`; no new module) and is the first vault **write**. GET masked connection metadata remains the PG-004/PG-005 Partial freeze in [API.md](API.md): config keys `REDGRES_POSTGRES_PUBLIC_HOST` and `REDGRES_POSTGRES_DIRECT_PORT` load in `internal/config`; `REDGRES_POSTGRES_POOLED_PORT` is also the pooled project URL port. The HTTP handler and URL builder are implemented (`Inventory.Connection`, `MaskedProjectConnectionURL`, GET `/api/v1/postgres/databases/{db}/connection`). POST reveal is the PG-005 Partial freeze in [API.md](API.md).
- Logging: standard `log/slog`, structured to journald with redaction.

No ORM is planned. PostgreSQL administrative SQL depends heavily on catalog queries, identifier quoting, autocommit-only operations, and explicit privilege semantics; direct audited SQL is clearer.

Service-version support is governed by [COMPATIBILITY.md](COMPATIBILITY.md) and [ADR-008](decisions/ADR-008-service-version-policy.md). Redgres detects the connected PostgreSQL, Redis, and PgBouncer versions and required capabilities before enabling administrative mutations. The supported matrix is release-owned and cannot be widened by runtime configuration.

PostgreSQL server adoption/install, extension host packages, per-database extension state, preload/restart configuration and PgBouncer service lifecycle are separate deployment concerns governed by [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md) and [ADR-009](decisions/ADR-009-postgres-adoption-and-extensions.md). The browser application does not become an arbitrary package/extension manager; approved changes run through the versioned installer/CLI and direct PostgreSQL connection.

## 5. Frontend stack

- Wave 0 frontend pins (locked in `web/package-lock.json`): React `19.2.8`, Vite `8.2.2`, `@vitejs/plugin-react` `6.1.0`, Vitest `4.1.11`, TypeScript `7.0.2`. Node build tool is Active LTS **24.19.0** (`web/.nvmrc`). Local Node 25.x is unsupported and is not release evidence.
- Vite writes to `internal/web/dist/app` so `//go:embed all:dist` still compiles from the tracked `dist/.gitkeep`. `build.modulePreload.polyfill` is `false` so the HTML has no inline script under `script-src 'self'`.
- Wave 1 login/shell ships with `tokens.css` and existing React 19.2.8. TanStack Query, Tailwind CSS, Radix, and a client router remain deferred; the parent owns `web/package.json` / lockfile.
- Shared application shell and semantic tokens follow [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md); feature folders do not define independent navigation, palettes, or breakpoints.
- Small local state only; no credential in global stores, URL, localStorage, sessionStorage, IndexedDB, analytics, or error reporting.
- Production build embedded through Go `embed`; Node.js is build-time only.
- The asset source is selected by `internal/config` (`REDGRES_DEV_ASSET_DIR`) and passed to `internal/web`, which reads no environment. The development filesystem source is rejected in production. Two independent bounds apply to it: the `httpapi` allow-list restricts served *names* to `index.html` and `assets/*` through `fs.ValidPath`, and `os.Root` restricts resolved *targets* to the directory so a symlink or junction inside it cannot escape.

Feature folders mirror API domains: `overview`, `postgres`, `redis-users`, `audit`, `system`, `auth`, and `docs`.

### Application dependency version policy

- At the initial implementation baseline and each planned dependency refresh, resolve every direct dependency/tool to the newest stable, security-supported release compatible with the complete stack and supported platforms.
- “Latest” is resolved during a reviewed change and then pinned. Production/CI never follows floating package ranges, unpinned tool downloads, container `latest` tags, prereleases, or nightly builds.
- Go modules/checksums, npm lockfiles, Node build-tool version, linters/generators, container images/digests, and deployment tools are reproducible inputs. Node.js uses a supported LTS line and remains build-time only.
- Automated dependency tooling may open small grouped update pull requests, but does not merge automatically. Major updates and security-sensitive runtime changes receive explicit review, release-note/migration analysis, complete tests, vulnerability/license checks, and rollback consideration.
- If the newest upstream release is incompatible or unproven, Redgres pins the newest tested safe release and records the reason/upgrade issue. Being newest never outranks correctness, security, or recoverability.
- PostgreSQL, Redis, and PgBouncer version choices remain governed by [COMPATIBILITY.md](COMPATIBILITY.md); PostgreSQL/PgBouncer lifecycle and optional capability changes additionally follow [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).

## 6. Control-plane state

SQLite contains:

- owner users and Argon2id PHC strings stored as UTF-8 bytes in `owners.password_hash`;
- hashed session and CSRF tokens plus expirations;
- login attempts;
- redacted audit events;
- operation records for long-running actions;
- schema migration metadata;
- optional non-secret preferences.

SQLite must not contain PostgreSQL project passwords, Redis project passwords, complete credential URLs, Cloudflare tokens, private keys, or Redis administrator URLs.

Use WAL mode, foreign keys, busy timeout, bounded connection count, schema migrations, and the SQLite backup API for consistent backups.

## 7. PostgreSQL adapter

Use at least two connection profiles:

- Administrative direct profile: direct PostgreSQL 5432, never PgBouncer, because create/drop database, role management, session termination, catalog inspection, and some transactions are incompatible with transaction pooling.
- Pooled-observation profile: optional health verification of PgBouncer 6432; Redgres does not route its own administrative DDL through PgBouncer.

Use cases own policy; adapter owns SQL mechanics. Protected target validation must run immediately before mutations, not only when the UI renders.

Long operations (clone, backup verification) create an operation record and run under bounded context/cancellation. A per-target lock prevents overlapping rotate/drop/clone operations. PG-006 Partial rotate is **in-request**: an in-process per-owner TryLock only (no SQLite `operations` row, no `GET /api/v1/operations/{id}`). PG-010 Partial duplicate is **in-request 201**: an in-process TryLock on source name + new database + new owner (no SQLite `operations` row, no `202`, no `GET /api/v1/operations/{id}`). Official PostgreSQL 17/18 `CREATE DATABASE` cannot run inside a transaction block and fails if another session is connected to the template database ([17](https://www.postgresql.org/docs/17/sql-createdatabase.html), [18](https://www.postgresql.org/docs/18/sql-createdatabase.html)). `REASSIGN OWNED` is forbidden because it also reassigns shared objects (databases, tablespaces) ([17](https://www.postgresql.org/docs/17/sql-reassign-owned.html), [18](https://www.postgresql.org/docs/18/sql-reassign-owned.html)). Compensation never mutates the source. Handler timeout is 30s; clones that exceed it remain a later-operations Complete limitation. PG-008 Partial row delete is **in-request 200**: `REDGRES_FEATURE_POSTGRES_ROW_DELETE` (default off), session + `postgres.destructive` + CSRF, in-handler AUTH-006, server-discovered single-column PK, parameterized `DELETE … WHERE pk IN (…)`, no SQLite `operations` row, no `202`, no TryLock. PG-009 Partial truncate is **in-request 200**: `REDGRES_FEATURE_POSTGRES_TRUNCATE` (default off), session + `postgres.destructive` + CSRF, in-handler AUTH-006, existing `listTablesSQL` then one quoted `TRUNCATE TABLE … RESTART IDENTITY` of that complete BASE TABLE set (no `CASCADE`, no per-table loop), in-process TryLock on database name only (not rotate/duplicate maps; serialize with PG-011 drop map), no SQLite `operations` row, no `202`. Truncated table list (`>500`) is `409` with no SQL. PG-011 Partial drop is **in-request 200**: `REDGRES_FEATURE_POSTGRES_DROP` (default off, `envBool`), session + `postgres.destructive` + CSRF, in-handler AUTH-006, terminate excluding current backend then quoted `DROP DATABASE` (no `WITH (FORCE)`, no transaction), optional `DROP ROLE` + vault DELETE only when ownership is proven safe, in-process TryLock on database name only (new map; serialize with truncate; not rotate/duplicate maps), no SQLite `operations` row, no `202`. Official PostgreSQL 17/18 `DROP DATABASE` is the same command, including `WITH (FORCE)` which Redgres must **not** use ([17](https://www.postgresql.org/docs/17/sql-dropdatabase.html), [18](https://www.postgresql.org/docs/18/sql-dropdatabase.html)). Backup freshness is not an HTTP gate this Partial (**BF-1**). Official PostgreSQL 17/18 `TRUNCATE` is the same command ([17](https://www.postgresql.org/docs/17/sql-truncate.html), [18](https://www.postgresql.org/docs/18/sql-truncate.html)); omit `ONLY` because `TRUNCATE ONLY` on a partitioned parent errors on both majors ([17](https://www.postgresql.org/docs/17/ddl-partitioning.html), [18](https://www.postgresql.org/docs/18/ddl-partitioning.html)). Official PostgreSQL 17/18 primary-key catalog is `information_schema.table_constraints` (`constraint_type` includes `PRIMARY KEY`) joined to `key_column_usage` (`ordinal_position`) ([17 table_constraints](https://www.postgresql.org/docs/17/infoschema-table-constraints.html), [17 key_column_usage](https://www.postgresql.org/docs/17/infoschema-key-column-usage.html), [18 table_constraints](https://www.postgresql.org/docs/18/infoschema-table-constraints.html), [18 key_column_usage](https://www.postgresql.org/docs/18/infoschema-key-column-usage.html)).

## 8. Redis adapter

Redgres connects using a dedicated ACL administrator loaded from a root-readable credential file. The browser never receives it. Adapter methods expose narrowly scoped behavior rather than a generic command API.

Preset command sets are versioned in code and tested against every supported Redis series. Externally managed ACL users with unsupported patterns/categories are read-only or require an explicit adoption workflow; Redgres must not rewrite an ACL it cannot faithfully understand.

## 9. Credential lifecycle

### PostgreSQL

Generated credential → create/alter PostgreSQL role → persist encrypted vault entry → return once/no-store. Because the role and vault are separate stores, the use case records operation state and defines compensation:

- Create: if vault write fails, remove only the newly created database/role after dependency checks, or mark credential recovery required.
- Rotate: keep the generated password only in process memory; if vault write fails after role alteration, retry vault persistence (3) without re-ALTER and block additional rotation with an in-process per-owner lock for the duration of the request. Report a recoverable incident without logging the password: HTTP 503 copy that the PostgreSQL password was changed but the vault could not be saved; the next POST rotate is recovery. A future dual-secret/transaction design needs its own ADR. Frozen `POST /api/v1/postgres/databases/{db}/credentials/rotate` (session + `postgres.credentials` + CSRF) uses `Inventory.Rotate`: eligibility re-read, `GeneratePassword`, `secrets.Encrypt`, `ALTER ROLE … WITH PASSWORD … CONNECTION LIMIT 20`, parameterized `ON CONFLICT` upsert. Create INSERT stays without upsert. No `ensure_vault`. No SQLite storage of the project password.
- Duplicate: create a new restricted login and `CREATE DATABASE … TEMPLATE {source} OWNER {new_owner}` in-request; persist encrypted vault INSERT (no upsert); return 201 no-store. Frozen `POST /api/v1/postgres/databases/{db}/duplicate` (session + `postgres.provision` + CSRF) uses `Inventory.Duplicate`. Compensation drops only created clone/role/vault row. Never `REASSIGN OWNED`. Never mutate the source on failure. No operations row.
- Row delete: flagged in-request DML; server discovers PK; bounded parameterized `DELETE`; AUTH-006 on the request body. Frozen `DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` (session + `postgres.destructive` + CSRF + `REDGRES_FEATURE_POSTGRES_ROW_DELETE`) uses `Inventory.DeleteRows`. No operations row. No vault decrypt.
- Truncate: flagged in-request table-set DML; list BASE TABLEs then one `TRUNCATE TABLE … RESTART IDENTITY`; AUTH-006 on the request body. Frozen `POST /api/v1/postgres/databases/{db}/truncate` (session + `postgres.destructive` + CSRF + `REDGRES_FEATURE_POSTGRES_TRUNCATE`) uses `Inventory.Truncate`. No operations row. No vault decrypt. No `CASCADE`.
- Drop: flagged in-request `DROP DATABASE` after terminate-excluding-current-backend; AUTH-006 on the request body. Frozen `DELETE /api/v1/postgres/databases/{db}` (session + `postgres.destructive` + CSRF + `REDGRES_FEATURE_POSTGRES_DROP`) uses `Inventory.Drop`. Optional `DROP ROLE` only when not `OwnerDenied` and `OwnedDatabaseCount == 0`; vault row DELETE only if the role was dropped. No operations row. No decrypt. No `WITH (FORCE)`. Admin pool stays on the administrative database.

### Redis

Generated credential → `ACL SETUSER` → return once/no-store. Redgres does not persist the Redis user password. The optional `credential.urls.primary` value is built from `REDGRES_REDIS_PUBLIC_HOST` and `REDGRES_REDIS_PUBLIC_PORT` only; it never copies the administrator URL. Rotation is irreversible for the old password. If the response is lost, rotate again. If `SETUSER` succeeds but the audit insert fails, the credential is not returned.

## 10. Configuration

Configuration loads from flags/environment plus root-protected credential files. Production validation fails closed for defaults, missing TLS settings, insecure remote Redis URLs, public UI bindings, weak owner bootstrap, and contradictory feature flags.

Recommended prefixes:

- `REDGRES_*` — service/base URL/state/session/log settings.
- `REDGRES_POSTGRES_*` — administrative DSN components, public host/ports, protected databases, optional expected-major identity guard.
- `REDGRES_REDIS_*` — administrator URL file, public host/port, optional expected-series identity guard.
- `REDGRES_FEATURE_*` — destructive action gates.

Secrets should be passed as file paths where practical, never command-line values visible in process listings.

## 11. Failure model

- PostgreSQL unavailable: Redis management and audit remain available; PostgreSQL card reports degraded.
- Redis unavailable: PostgreSQL management and audit remain available; Redis card reports degraded.
- SQLite unavailable: authentication, mutations, and audit-dependent actions fail closed; health reports failure.
- Cloudflare unavailable: loopback services remain healthy but remote browser access is unavailable; raw DB services are independent.
- PgBouncer unavailable: direct PostgreSQL remains separately testable; pooled URL health is degraded.

No component status should collapse independent failures into a single generic “offline.”
