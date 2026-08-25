# HTTP API contract

Status: target contract. Final schemas should be machine-described (OpenAPI or equivalent) and checked in CI.

## Conventions

- Prefix: `/api/v1`.
- JSON only for API requests/responses.
- Session cookie: opaque HttpOnly cookie; browser sends `X-CSRF-Token` on mutations.
- Every response includes `request_id`.
- `X-Request-ID` is a server-generated 128-bit value (32 lowercase hex characters) echoed as `request_id`. An inbound `X-Request-ID` is not trusted or copied (log-injection defense; intentional divergence from the Redis source).
- All `/api/v1/*` responses set `Cache-Control: no-store`.
- Body limit: 64 KiB by default; lower endpoint limits are allowed.
- Unknown JSON fields are rejected.
- List endpoints use cursor pagination where records can grow; row browsing uses bounded offset/keyset semantics documented per endpoint.
- Credentials never appear in GET responses.

Success envelope may contain resource-specific keys. Error shape is stable:

```json
{
  "error": {
    "code": "validation_error",
    "message": "Safe operator-facing message",
    "fields": {}
  },
  "request_id": "..."
}
```

`fields` is present only for field-level validation errors.

Core error codes: `unauthorized`, `forbidden`, `csrf_invalid`, `rate_limited`, `validation_error`, `protected_resource`, `conflict`, `not_found`, `method_not_allowed`, `reauth_required`, `dependency_unavailable`, `operation_in_progress`, `internal`.

## Authentication/platform endpoints

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/auth/login` | Authenticate owner and issue session + CSRF |
| POST | `/api/v1/auth/logout` | Delete session |
| GET | `/api/v1/session` | Return owner identity, rotated CSRF, enabled capabilities/tool links |
| GET | `/api/v1/healthz` | Liveness/state DB health, no auth secrets |
| GET | `/api/v1/status` | Authenticated component status |
| GET | `/api/v1/search?q=&limit=` | Authenticated bounded search over manageable resource metadata/navigation; no secrets or destructive execution |
| GET | `/api/v1/audit?cursor=&limit=` | Paginated redacted audit history (implemented) |
| GET | `/api/v1/operations/{id}` | Long-operation state/result summary |

Session cookie name: `redgres_session` (opaque 64-hex token, `Path=/`, `HttpOnly`, `SameSite=Strict`, `Secure` from `REDGRES_COOKIE_SECURE`). Mutations send `X-CSRF-Token`. There is no HTTP bootstrap route.

**POST `/api/v1/auth/login`** requires a matching `Origin` or `Referer` (no CSRF header). Body: `{"username":"admin","password":"…"}`. Unknown fields are rejected. Success `200`:

```json
{"owner":{"username":"admin"},"csrf_token":"<64 hex>","request_id":"<32 hex>"}
```

Generic failure is `401` `unauthorized` with message `Invalid username or password.` Lockout is `429` `rate_limited` plus `Retry-After`. Bad origin is `403` `csrf_invalid`.

**POST `/api/v1/auth/logout`** requires session + origin + CSRF. Success `200`: `{"ok":true,"request_id":"…"}` and clears the cookie (`MaxAge=-1`).

**GET `/api/v1/session`** requires a session cookie and does not require CSRF. It rotates the CSRF token. Success `200`:

```json
{
  "owner": {"username": "admin"},
  "csrf_token": "<rotated 64 hex>",
  "capabilities": [
    "platform.read", "audit.read",
    "postgres.read", "postgres.provision", "postgres.credentials", "postgres.destructive",
    "redis.read", "redis.provision", "redis.credentials", "redis.destructive"
  ],
  "tool_links": {},
  "request_id": "…"
}
```

`tool_links` is an object, never `null`. Unconfigured (both `REDGRES_PGADMIN_URL` and `REDGRES_REDISINSIGHT_URL` empty/unset) is `{}`. Configured keys are present only when that URL is set: string hrefs, never `null`, never `""`. Allowed keys: `pgadmin`, `redisinsight`. Example when both are set:

```json
"tool_links": {
  "pgadmin": "https://pgadmin.example.com",
  "redisinsight": "https://redis-insight.example.com"
}
```

This GET is the only href source. Login JSON is unchanged and has no `tool_links`. GET `/session` still rotates CSRF and is not a mutation (no audit). `Cache-Control: no-store` is unchanged. The payload never includes passwords, session tokens, or CSRF in `tool_links`.

**GET `/api/v1/status`** requires a session cookie and the `platform.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny this route today; the session check is what enforces access.

The handler always returns `200` when it runs, even if every component is down. Partial failure is represented per component; it is not a `503`. `503` `dependency_unavailable` happens only when `requireSession` cannot read SQLite (the same control-plane storage failure as other authenticated routes). Missing session is `401` `unauthorized` with no `components` key. `POST`, `PUT`, `PATCH`, and `DELETE` are `405` `method_not_allowed`. The response is `Cache-Control: no-store`. There are no query parameters; the path is exactly `/api/v1/status`.

Success `200`:

```json
{
  "components": [
    { "id": "redgres_state", "state": "ok" },
    { "id": "postgres_direct", "state": "not_configured" },
    { "id": "pgbouncer", "state": "not_configured" },
    { "id": "redis", "state": "not_configured" },
    { "id": "tool_links", "state": "not_configured" }
  ],
  "request_id": "<32 lowercase hex>"
}
```

`components` is always an array of length 5 in this fixed order, never `null`. `state` is one of `ok`, `unavailable`, `not_configured`, `not_implemented`. `reason` is omitted except when `state` is `unavailable`, in which case it is always `"unreachable"`. The payload never includes host, port, DSN, password, token, SQL, driver text, `err.Error()`, version, uptime, or URLs.

Independent checks, sequential. Ping probes use a 2s timeout; `tool_links` is config presence only (no timeout, no network):

| `id` | Probe |
|---|---|
| `redgres_state` | SQLite `PingContext`. `ok` or `unavailable` + `unreachable`. |
| `postgres_direct` | Absent adapter → `not_configured`. Else `Inventory.Ping`: `ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Ping uses `pgxpool.Ping` on the admin pool. List/details are not used as health. |
| `pgbouncer` | Absent/empty `REDGRES_POSTGRES_POOLED_PORT` or nil ping → `not_configured`. Else `Inventory.PingPooled`: `ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Probe connects to virtual database `pgbouncer` on the pooled port (same host/user/password/sslmode as the admin PostgreSQL connection) with pooled `ConnConfig.DefaultQueryExecMode = QueryExecModeSimpleProtocol` (not `QueryExecModeExec`) and issues `SHOW VERSION` (prefer `Exec` with no args; do not `Scan`; do not use `pgxpool.Ping`, whose `-- ping` is invalid console syntax). Success is no error; the version string is discarded; empty rows are ok. Independent of `postgres_direct`. This component must not emit `not_implemented`. List/details/security/tables/rows/DDL stay on 5432. |
| `redis` | Absent adapter → `not_configured`. Else `Service.Ping`: `ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Ping uses go-redis `Ping` only. INFO, DBSIZE, latency, and ACL are not used as health. |
| `tool_links` | Not a probe. Both optional URLs empty/unset → `not_configured`. One or both set → `ok`. Never `unavailable` (no fetch/ping). Never `not_implemented`. Independent of `postgres_direct` / `pgbouncer` / `redis`. Status JSON never includes the URLs; hrefs are GET `/session` only. |

This GET is not a mutation and does not write an audit event.

**GET `/api/v1/search`** requires a session cookie and the `platform.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny this route today; the session check is what enforces access.

Query `q` is required after trim. Minimum 1 Unicode code point; more than 128 returns `400` `validation_error` with `fields.q` `too_long`. Missing, empty, or whitespace-only `q` returns `fields.q` `too_short`. The submitted `q` is never echoed in the error body, never written to slog, and never audited. `limit` is optional; a non-integer value returns `400` with `fields.limit` `invalid`. Values `<= 0` or `> 50` clamp to `20`. The effective limit is echoed.

When `q` and `limit` are valid the handler always returns `200` with both resource groups, even if PostgreSQL or Redis is down. Partial failure is per group; it is not a `503`. `503` `dependency_unavailable` happens only when `requireSession` cannot read SQLite. Missing session is `401` `unauthorized` with no `groups` key. `POST`, `PUT`, `PATCH`, and `DELETE` are `405` `method_not_allowed`. The response is `Cache-Control: no-store`. PostgreSQL and Redis are probed sequentially under one 2s timeout. A nil Redis adapter still returns the Redis group as `not_configured`; a nil PostgreSQL adapter still probes Redis.

Success `200`:

```json
{
  "groups": [
    {
      "id": "postgres_databases",
      "label": "PostgreSQL databases",
      "service": "postgres",
      "status": "ok",
      "truncated": false,
      "hits": [
        { "id": "postgres_database:project_a", "type": "postgres_database", "label": "project_a" }
      ]
    },
    {
      "id": "redis_acl_users",
      "label": "Redis ACL users",
      "service": "redis",
      "status": "ok",
      "truncated": false,
      "hits": [
        { "id": "redis_acl_user:project_a", "type": "redis_acl_user", "label": "project_a" }
      ]
    }
  ],
  "limit": 20,
  "request_id": "<32 lowercase hex>"
}
```

`groups` is always an array of length 2 in this fixed order, never `null`. Hits contain only `id`, `type`, and `label`. The payload never includes owner, size, credential, password, DSN, URL, SQL, `err.Error()`, audit `events`/`metadata`, or CSRF tokens.

| `id` | Behavior |
|---|---|
| `postgres_databases` | Absent adapter → `not_configured`, `hits: []`. Else `Inventory.Search` (existing List manageability, case-insensitive name substring, 2s timeout): success → `ok` plus matching names; `ErrNotConfigured` → `not_configured`; any other error → `unavailable` and empty hits. Protected databases are omitted the same way as list. Hits only when `status` is `ok`. Empty hits are `[]`, never `null`. Group status is never `not_implemented`. There is no `reason` field. |
| `redis_acl_users` | Absent adapter → `not_configured`, `hits: []`. Else `redisadmin.Search` (ACL LIST via `ListUsers`, case-insensitive username substring, same 2s timeout): success → `ok` plus matching usernames; `ErrNotConfigured` → `not_configured`; any other error → `unavailable` and empty hits. Hits only when `status` is `ok`. Empty hits are `[]`, never `null`. Group status is `not_configured`, `ok`, or `unavailable` — never `not_implemented`. There is no `reason` field. ACL password hashes (`#…`) and plaintext passwords (`>…`) are not returned. Protected Redis users (`default`, `admin`, `redact_admin`, and the configured Redis administrator) are omitted from search hits even though **GET `/api/v1/redis/users`** still lists them. Search does not create, rotate, or `ACL SETUSER`. |

Navigation and documentation matches are client-side `filterNav` only; they are not in this response. This GET is not a mutation and does not write an audit event.

**GET `/api/v1/healthz`** is unchanged: unauthenticated liveness that pings the state DB only. Success `200` is `{"status":"ok","request_id":"…"}` with `Cache-Control: no-store`. It has no `components` array, does not require a session, does not ping PostgreSQL, Redis, or PgBouncer, and does not read or fetch tool-link URLs.

**GET `/api/v1/audit`** requires a session cookie and the `audit.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny this route today; the session check is what enforces access.

Success `200`:

```json
{
  "events": [
    {
      "id": 1421,
      "actor": "admin",
      "action": "owner.login",
      "target": "admin",
      "outcome": "success",
      "request_id": "aabbccddeeff00112233445566778899",
      "client_ip": "127.0.0.1",
      "created_at": "2026-08-25T04:11:09.123456789Z"
    }
  ],
  "has_more": true,
  "next_cursor": "YTE6MTQyMQ",
  "limit": 50,
  "request_id": "…"
}
```

Events are newest first. `events` is always an array; an empty page is `[]`, never `null`. `next_cursor` is present only when `has_more` is true, so `has_more` and a non-empty `next_cursor` always agree.

The audit `metadata` column is **never returned and never selected**. Write-time redaction (`internal/audit.redactMetadata`) is a substring heuristic, so historical rows may hold whatever it allowed; excluding the column from the query keeps that content out of any response by construction rather than by filtering. A later slice may project an explicit allow-list of metadata keys; it must never return the raw column.

`cursor` is opaque and must be treated as such by clients: it is base64url (unpadded) of a versioned `a1:<id>` payload. Paging orders by `id`, the SQLite rowid alias, because it is unique and assigned during the insert. `created_at` is not an ordering key: it is `TEXT` in RFC3339Nano form whose fractional second drops trailing zeros, so SQLite's string comparison is not chronological (`…05Z` sorts above `…05.5Z` while being earlier), and it is sampled before the insert. `id` is exposed as a stable per-event identifier for client keying only, not for ordering or arithmetic; ordering position is carried solely by `next_cursor`.

The cursor is exclusive: a page returns events strictly older than it. A well-formed cursor whose row no longer exists is **not** an error; it returns whatever is older, possibly an empty page. Default `limit` is 50; `limit<=0` or `limit>500` clamps to 50 and the response echoes the effective `limit`. A non-integer `limit` is `400` `validation_error` with `fields.limit`, and a malformed `cursor` is `400` `validation_error` with `fields.cursor`; neither queries, and the submitted cursor value is never echoed back. `actor`, `target`, and `client_ip` are nullable in storage and are emitted as `""` when null. `created_at` is returned verbatim as stored. There is no `total`, because the table grows without bound. A storage failure is `503` `dependency_unavailable` with the fixed control-plane storage message and no driver, path, or SQL text.

## Static asset delivery

The Go binary serves the embedded Vite build for non-API `GET` requests:

- `index.html` is served with `Cache-Control: no-store` and is the SPA fallback for unknown non-API paths.
- Hashed `/assets/*` files are served with `Cache-Control: public, max-age=31536000, immutable`.
- If embedded assets are absent (clean checkout before `npm run build`), non-API `GET` returns `503` `dependency_unavailable`. There is no directory listing and no empty 200.

Search requires a normalized minimum query length, a strict maximum length/limit, request cancellation/timeouts, and stable grouped result types. It returns only fields already safe for authenticated inventory views, excludes protected/hidden targets and credential material, rate-limits abusive use, and never accepts an action/command to execute. Documentation/navigation entries may be client-side. PostgreSQL search hits use the same manageability policy as **GET `/api/v1/postgres/databases`**. Redis ACL search hits are the exception: they omit protected usernames that **GET `/api/v1/redis/users`** still lists.

## PostgreSQL endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/postgres/databases` | Manageable databases only (implemented) |
| POST | `/api/v1/postgres/databases` | Create database/role; vaulted credentials (`one_time: false`); `no-store`; `postgres.provision` + CSRF (PG-003 Partial freeze; no AUTH-006) |
| GET | `/api/v1/postgres/databases/{db}` | Details/security metadata (implemented; vault existence `present`/`missing`/`not_available`) |
| GET | `/api/v1/postgres/databases/{db}/connection` | Masked URLs and saved status only (implemented; PG-004/PG-005 Partial) |
| POST | `/api/v1/postgres/databases/{db}/connection/reveal` | Saved password + URLs; `no-store`; `postgres.credentials` + CSRF (PG-005 Partial freeze; no AUTH-006) |
| POST | `/api/v1/postgres/databases/{db}/credentials/rotate` | Rotate; typed confirmation; vaulted credentials (`one_time: false`); `no-store`; `postgres.credentials` + CSRF (PG-006 Partial freeze; no AUTH-006) |
| POST | `/api/v1/postgres/databases/{db}/duplicate` | Duplicate; new unique owner; vaulted credentials (`one_time: false`); `no-store`; `postgres.provision` + CSRF (PG-010 Partial freeze; 201 in-request; no AUTH-006; no 202) |
| DELETE | `/api/v1/postgres/databases/{db}` | Exact confirmation + owner password |
| GET | `/api/v1/postgres/databases/{db}/tables` | BASE TABLE names (implemented; cap 500) |
| GET | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | Bounded rows/search (implemented; offset/limit; `q` max 128) |
| GET | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` | PK column names (PG-008 Partial freeze; `postgres.read`; no CSRF; no flag) |
| DELETE | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | Selected rows; typed table confirmation + owner reauth (PG-008 Partial freeze; `postgres.destructive` + CSRF + `REDGRES_FEATURE_POSTGRES_ROW_DELETE`) |
| POST | `/api/v1/postgres/databases/{db}/truncate` | Truncate all listed BASE TABLEs; typed database confirmation + owner reauth (PG-009 Partial freeze; `postgres.destructive` + CSRF + `REDGRES_FEATURE_POSTGRES_TRUNCATE`) |
| GET | `/api/v1/postgres/security` | Cluster security overview (implemented; vault existence query) |

Database/role names in URL segments are decoded then validated. Transport validation never replaces PostgreSQL identifier quoting.

**Implemented now:** `GET /api/v1/postgres/databases`, `GET /api/v1/postgres/databases/{db}`, `GET /api/v1/postgres/databases/{db}/connection`, `GET /api/v1/postgres/databases/{db}/tables`, `GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows`, and `GET /api/v1/postgres/security` require a session and the `postgres.read` capability. `POST /api/v1/postgres/databases/{db}/connection/reveal` requires a session, `postgres.credentials`, and CSRF (PG-005 Partial). `POST /api/v1/postgres/databases` requires a session, `postgres.provision`, and CSRF (PG-003 Partial). `POST /api/v1/postgres/databases/{db}/credentials/rotate` requires a session, `postgres.credentials`, and CSRF (PG-006 Partial). `POST /api/v1/postgres/databases/{db}/duplicate` requires a session, `postgres.provision`, and CSRF (PG-010 Partial). `GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` requires a session and `postgres.read` (PG-008 Partial freeze; no CSRF; no flag). `DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` requires a session, `postgres.destructive`, CSRF, and `REDGRES_FEATURE_POSTGRES_ROW_DELETE` (PG-008 Partial freeze; AUTH-006). `POST /api/v1/postgres/databases/{db}/truncate` requires a session, `postgres.destructive`, CSRF, and `REDGRES_FEATURE_POSTGRES_TRUNCATE` (PG-009 Partial freeze; AUTH-006). List and table list are unpaginated and hard-capped at 500 (`truncated: true` if more rows exist). Details include owner, size, collation/ctype, locale fields, connection count, and security flags. Details `saved_credential` is a vault **existence** result for the database owner (`present` / `missing` / `not_available` with reason `vault_unavailable`). Ciphertext is never selected or returned. `vault_not_implemented` is not used. Table list returns `{schema,name}` for `information_schema` `BASE TABLE` rows outside `pg_catalog` and `information_schema`. Schema/table names on the table list are result columns. Row browse quotes schema, table, and column names with `pgx.Identifier` and parameterizes values. Query `q` is optional (no minimum); more than 128 Unicode code points returns `400` `validation_error` with `fields.q` and does not query. `q` `ILIKE`s columns whose `data_type` contains `text`, `character`, or `citext` (same predicate as `database-app` `fetch_table_data` at `1c3e8e2`; `citext` stored as `USER-DEFINED` is therefore usually not searched). `%` and `_` in `q` remain LIKE wildcards. Default `limit` is 50; `limit<=0` or `limit>500` clamps to 50; `offset<0` clamps to 0; non-integer `limit`/`offset` is `400`. Response is `{columns,rows,total,offset,limit,request_id}`. A missing or non-`BASE TABLE` schema/table, `pg_catalog`/`information_schema`, or a table with no columns is `404` `not_found` (same message as a missing database). An existing table with columns and zero matching rows is `200` with `rows: []` and `total: 0`. Cell values use JSON-safe encoding (`null`, bool, finite numbers, `numeric` as string, `bytea` as PostgreSQL `\x` hex text, timestamps as RFC3339). Encode/connect/query failure is `503` `dependency_unavailable` and never a healthy empty page. Protected names, protected owners, templates, and `datallowconn=false` are omitted from the list and return the same `404` `not_found` as a missing database (not `protected_resource`); table list and rows do not open a per-database connection for those names. Invalid identifiers return `400` `validation_error` without querying. `GET /api/v1/healthz` does not ping PostgreSQL.

**GET `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` (PG-008 Partial freeze).** Session cookie and `postgres.read` (not `postgres.destructive`). No CSRF. No feature flag. Register next to GET `/rows`:

```go
r.With(s.requireSession, s.requireCapability("postgres.read")).Get("/api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key", s.handlePostgresPrimaryKey)
```

Other methods on this path are `405` `method_not_allowed`. Path identifiers use existing `decodePathIdentifier` (db, schema, table). Invalid name → `400` `validation_error`, no catalog. Manageability matches GET rows: protected, template, `datallowconn=false`, missing database, missing/non-`BASE TABLE`/`pg_catalog`/`information_schema` → `404` `not_found` (same operator copy as GET rows), no leak. Nil adapter → `503` `dependency_unavailable`. Responses are `Cache-Control: no-store`. This GET is not a mutation and does not write an audit event. Do not reopen GET `/rows`.

Server discovers the primary key from `information_schema` (parameterized; not `schema.table::regclass`; not sibling `LIMIT 1`). Confirm `BASE TABLE` first (existing `confirmBaseTableSQL`). Join `table_constraints` to `key_column_usage` on `constraint_catalog`, `constraint_schema`, `constraint_name`, `table_schema`, and `table_name`; filter `constraint_type = 'PRIMARY KEY'` and `table_schema = $1` and `table_name = $2`; `ORDER BY kcu.ordinal_position`. Official PostgreSQL 17: [table_constraints](https://www.postgresql.org/docs/17/infoschema-table-constraints.html), [key_column_usage](https://www.postgresql.org/docs/17/infoschema-key-column-usage.html). Official PostgreSQL 18: [table_constraints](https://www.postgresql.org/docs/18/infoschema-table-constraints.html), [key_column_usage](https://www.postgresql.org/docs/18/infoschema-key-column-usage.html). Catalog column names use `QuoteCatalogIdentifier` (empty/NUL fail closed). HTTP path names stay `ValidateIdentifier`.

Success `200`:

```json
{ "primary_key": ["id"], "request_id": "<32 lowercase hex>" }
```

`primary_key` is never `null`. No primary key → `[]`. Composite → every column in ordinal order. Missing table is `404`, not `200` with `[]`.

**DELETE `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` (PG-008 / AUTH-006 Partial freeze).** Session cookie, `postgres.destructive` (not `postgres.read`, not `postgres.provision`, not `postgres.credentials`), and CSRF (`requireMutation`). Register next to GET `/rows`:

```go
r.With(s.requireSession, s.requireCapability("postgres.destructive"), s.requireMutation).Delete("/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows", s.handlePostgresRowsDelete)
```

POST/PUT/PATCH on `/rows` stay `405` `method_not_allowed`. GET `/rows` is unchanged (`postgres.read`, no CSRF). There is no `POST /api/v1/auth/reauth` and no short-lived reauth grant (that needs a future ADR). Handler timeout is **30s**. No in-process TryLock. No `202`. No `migrations/002_operations.sql`. Responses are `Cache-Control: no-store`. Feature flag is **`REDGRES_FEATURE_POSTGRES_ROW_DELETE`** only (default unset = off). There is no `ENABLE_DESTRUCTIVE_ACTIONS` and no `REDGRES_FEATURE_POSTGRES_DELETE`. This Partial is in-request **200**. Do not bump pgx (`v5.10.0`).

`DisallowUnknownFields` applies. Unknown fields (including `primary_key_column`, `password`, `confirmation`, `database_confirmation`) → `400` `validation_error` (`Unknown field`), no audit, no PostgreSQL.

Body only:

```json
{
  "table_confirmation": "orders",
  "owner_password": "<owner password>",
  "primary_key_values": ["1", "2"]
}
```

`table_confirmation` is the path `{table}`. `owner_password` is the Redgres owner password (AUTH-006). `primary_key_values` are scalars (JSON string, number, or bool). Reject `null`, object, and array elements.

**Check order:**

1. Middleware: session, `postgres.destructive`, CSRF.
2. Flag unset or false → `403` `forbidden`, copy **Row delete is turned off.**, **no JSON decode**, no audit, no PostgreSQL. Invalid flag value fails `config.Load` (names the env var; never echoes the value).
3. Path `decodePathIdentifier` for `{db}`, `{schema}`, `{table}`. Invalid → `400`, no catalog.
4. JSON decode. Invalid JSON / unknown field → `400`, no audit, no PostgreSQL.
5. `table_confirmation` **exact** match to path `{table}`. Mismatch or empty → `400` `validation_error` with `fields.table_confirmation`, copy **Type the exact table name to confirm deletion**, no audit, no PostgreSQL. Do not also require the database name.
6. `primary_key_values` length 1–500; every element a JSON string, number, or bool. Empty, `>500`, `null` elements, objects, arrays → `400` `validation_error` with `fields.primary_key_values`, copy **Select between 1 and 500 primary key values.**, no audit, no PostgreSQL.
7. AUTH-006: `auth.Reauthenticate` (`LookupOwnerByUsername` + `Verify` on `owner_password`). Wrong password → `403` `reauth_required`, **Owner password is incorrect**, failure audit `postgres.rows.delete` with metadata `database` + `schema` + `table` only, **no SQL**. Missing owner → `VerifyUnknown` then `401` `unauthorized`, no audit, no SQL. Other SQLite lookup errors → `503` `dependency_unavailable`. **Do not** increment AUTH-005 `login_attempts`. **Do not** return `429`. Never log, audit, or return `owner_password`.
8. `Service.DeleteRows`: manageability like GET rows (protected, template, `datallowconn=false`, missing DB, missing/non-`BASE TABLE` → `404` `not_found`, same operator copy; failure audit allowed; **no DML**). Confirm BASE TABLE. Discover PK with the GET primary-key SQL. Width ≠ 1 (none or composite) → `400` `validation_error` with `fields.primary_key`, copy **This table does not have a single-column primary key.**, no `DELETE`. Quoted `DELETE FROM {schema}.{table} WHERE {pk} IN ($1…$n)` (`pgx.Identifier{schema,table}.Sanitize()`; PK column `QuoteCatalogIdentifier`; values parameterized only). Return `RowsAffected`.
9. Success audit then **200**. Audit-fail after DML → `503` `dependency_unavailable` (rows stay deleted; same fail-closed family as Redis DELETE).

Success `200`:

```json
{ "deleted": 2, "request_id": "<32 lowercase hex>" }
```

`deleted` is PostgreSQL `RowsAffected` for that statement (may be less than requested if some keys are absent). No sibling `message`. No PK values, SQL, password, or URLs.

**Audit:** `postgres.rows.delete`. Success metadata: `database`, `schema`, `table`, `deleted` (count). Failure metadata (reauth wrong password and post-reauth 404/400 PK/503): `database`, `schema`, `table` only. Never PK values, SQL, password, URLs.

Sibling deltas (do not copy): FastAPI no CSRF; `ENABLE_DESTRUCTIVE_ACTIONS`; client `primary_key_column`; `schema.table::regclass` + `LIMIT 1` without `ORDER BY`; HTTP 500 `str(e)`; `console.log` of PK values; `{deleted, message}` toast envelope; empty-array success `{deleted: 0}`.

PG-008 stays Partial. AUTH-006 stays Partial: Redis `DELETE /api/v1/redis/users/{username}` plus this DELETE (PG-009 truncate is a separate freeze). PostgreSQL drop reauth is not this slice. Live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright viewports, `go test -race`, CI, and Node 24.19.0 remain outstanding. Gate 4 does not apply (no vault decrypt). Do not mark Complete.

**UI copy (frozen with this DELETE contract):** Row grid checkboxes only when GET primary-key returns **exactly one** column. Hide checkboxes when `primary_key` is `[]` or length ≠ 1. **Delete selected** is `--danger` (not PostgreSQL identity blue), shown when a single-column PK exists (including when the flag is off; HTTP `403` uses **Row delete is turned off.**). Hidden while rows loading. Disabled while delete is in flight, while a credential ticket is open, or while zero rows are selected. Dialog `role=dialog`, title **Delete selected rows**, same focus trap as Redis Delete. Copy: type the exact **table** name and owner password; states the selected count and `schema.table` via `displayText()`; **Cannot be undone.** Fields: table confirm `autocomplete=off`; owner password `type=password` `autocomplete=current-password`. Confirm Delete disabled until confirmation equals the path table **and** password length > 0. Client confirmation is not authorization. DELETE uses CSRF, `encodeURIComponent` for db/schema/table, JSON `{ table_confirmation, owner_password, primary_key_values }` only. HTTP 200: close dialog, clear password/confirmation/selection, reload the current row page. `reauth_required`: stay on dialog, announce error, **clear password**, keep confirmation. 401: session-expired, clear secrets. 400 confirmation/PK stay on dialog. 404 / generic 503 inspector families. Flag-off 403 stays on dialog with **Row delete is turned off.** Memory only; clear selection on database/table change, Back, logout. Search / login / Security overview never DELETE rows.

**POST `/api/v1/postgres/databases/{db}/truncate` (PG-009 / AUTH-006 Partial freeze).** Session cookie, `postgres.destructive` (not `postgres.read`, not `postgres.provision`, not `postgres.credentials`), and CSRF (`requireMutation`). Register next to duplicate, **before** `GET .../databases/{db}`:

```go
r.With(s.requireSession, s.requireCapability("postgres.destructive"), s.requireMutation).Post("/api/v1/postgres/databases/{db}/truncate", s.handlePostgresDatabaseTruncate)
```

Other methods on `/truncate` are `405` `method_not_allowed`. Path `{db}` uses existing `decodePathIdentifier`. Invalid name → `400` `validation_error`, no catalog. There is no `POST /api/v1/auth/reauth` and no short-lived reauth grant. Handler timeout is **30s**. In-process TryLock on the **database name** only (request-scoped memory; do not share rotate/duplicate lock maps). Lock fail → `409` `operation_in_progress`, copy **A truncate is already in progress.** No `202`. No `migrations/002_operations.sql`. Responses are `Cache-Control: no-store`. Feature flag is **`REDGRES_FEATURE_POSTGRES_TRUNCATE`** only (default unset = off), parsed with `envBool` (same rules as row delete). There is no `ENABLE_DESTRUCTIVE_ACTIONS`. This Partial is in-request **200**. Do not bump pgx (`v5.10.0`). Official PostgreSQL 17/18 `TRUNCATE`: [17](https://www.postgresql.org/docs/17/sql-truncate.html), [18](https://www.postgresql.org/docs/18/sql-truncate.html). Backup freshness is **not** an HTTP gate (table-set mutation, not DROP DATABASE). `docs/OPERATIONS.md` remains the operator runbook. Gate 4 does not apply.

Body **only**:

```json
{
  "database_confirmation": "project_a",
  "owner_password": "<owner password>"
}
```

`database_confirmation` is the path `{db}`. Do not require a typed table list. `DisallowUnknownFields`. Unknown fields (including `confirmation`, `table_confirmation`, `password`, `tables`, `cascade`) → `400` `validation_error` (`Unknown field`), no audit, no PostgreSQL.

**Check order:**

1. Middleware: session, `postgres.destructive`, CSRF.
2. Flag unset or false → `403` `forbidden`, copy **Truncate is turned off.**, **no JSON decode**, no audit, no PostgreSQL. Invalid flag value fails `config.Load` (names the env var; never echoes the value).
3. Path `decodePathIdentifier` `{db}`. Invalid → `400`, no catalog.
4. JSON decode.
5. `database_confirmation` **exact** match to path `{db}`. Mismatch or empty → `400` `validation_error` with `fields.database_confirmation`, copy **Type the exact database name to confirm truncate**, no audit, no PostgreSQL.
6. AUTH-006: `auth.Reauthenticate` (`LookupOwnerByUsername` + `Verify` on `owner_password`). Wrong password → `403` `reauth_required`, **Owner password is incorrect**, failure audit `postgres.database.truncate` with metadata **`database` only**, **no SQL**. Missing owner → `VerifyUnknown` then `401` `unauthorized`, no audit, no SQL. Other SQLite lookup errors → `503` `dependency_unavailable`. **Do not** increment AUTH-005 `login_attempts`. **Do not** return `429`. Never log, audit, or return `owner_password`.
7. TryLock. Then `Service.Truncate`.
8. Success audit then **200**. Audit-fail after `TRUNCATE` → `503` `dependency_unavailable` (tables stay truncated; same fail-closed family as Redis DELETE / PG-008).

**Manageability:** same as GET tables: protected names (`postgres`, `template0`, `template1`, `database_console_vault`, configured admin/state databases, deny-list, protected owners), template, `datallowconn=false`, missing database → `404` `not_found` (same operator copy as details), **not** `403` `protected_resource`. Failure audit allowed after reauth; **no `TRUNCATE`**. Nil adapter → `503`.

**Table list before any `TRUNCATE`:** reuse existing `listTablesSQL` (do not invent catalog SQL; do not filter `information_schema.enforced`):

```sql
SELECT table_schema, table_name
FROM information_schema.tables
WHERE table_type = 'BASE TABLE'
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name
LIMIT 501
```

Catalog names use `QuoteCatalogIdentifier` / `pgx.Identifier{schema,table}.Sanitize()`; empty/NUL **fail closed** (entire request, no `TRUNCATE`). HTTP `{db}` stays `ValidateIdentifier`. Connect through existing `connectTarget`.

If `len > 500` (GET-tables truncated): **no `TRUNCATE`**. HTTP **`409` `conflict`**, copy **Table list is truncated. Truncate cannot run.** Failure audit `database` only.

Zero tables: still require AUTH-006; then **200** with `truncated: 0`, `failed: []`, `total_tables: 0`.

**SQL (frozen):** one statement naming every listed `{schema}.{table}`:

```sql
TRUNCATE TABLE {quoted schema.table, ...} RESTART IDENTITY
```

No `CASCADE` (RESTRICT default). No `ONLY`. No `CONTINUE IDENTITY`. No per-table loop. Identifiers only (table names are not query parameters). pgx **v5.10.0**. Sibling per-table `CASCADE` + swallowed exceptions is forbidden. `failed` is `[]` on HTTP 200. Statement failure after listing → `503` `dependency_unavailable` (“PostgreSQL is unavailable”), failure audit `database` only, no 200 with a partial count.

Success `200`:

```json
{
  "truncated": 12,
  "failed": [],
  "total_tables": 12,
  "request_id": "<32 lowercase hex>"
}
```

`failed` is never `null`. `truncated` is the **count of truncated tables**, not the GET-list cap flag. No sibling `message`. No SQL, password, URLs, `err.Error()`, or table contents.

**Audit:** `postgres.database.truncate`. Success metadata: `database`, `truncated` (count), `total_tables`. Failure metadata (wrong password, 404, truncated-list, 503, lock): `database` only. Never passwords, SQL, URLs, table contents, raw errors.

Sibling deltas (do not copy): FastAPI `DELETE /databases/{name}/data` with session only; no CSRF; `ENABLE_DESTRUCTIVE_ACTIONS`; click-through modal (no typed name, no owner password); per-table `CASCADE` + swallowed exceptions; `{truncated, total_tables, message}`; HTTP 500 `str(e)`.

PG-009 stays Partial. AUTH-006 stays Partial: Redis `DELETE /api/v1/redis/users/{username}` plus PG-008 row DELETE plus this POST. PostgreSQL drop reauth is not this slice. Live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright viewports, `go test -race`, CI, and Node 24.19.0 remain outstanding. Gate 4 does not apply. Do not mark Complete.

**UI copy (frozen with this POST contract):** Inspector **Truncate** is `--danger` (not PostgreSQL identity blue) when a manageable database is selected and details are loaded (not gated on rotation-eligible). Shown when the flag is off (HTTP `403` uses **Truncate is turned off.**). Hidden while details loading. Disabled while truncate/reveal/rotate/duplicate/create/row-delete is in flight or a credential ticket is open. Place in the inspector **danger** action row, not Security overview, not page-header Create, not the row-grid **Delete selected** control. Dialog `role=dialog`, title **Truncate project data**, same focus trap as Redis Delete / PG-008. Copy: type the exact **database** name and owner password; this removes all rows from all tables in `{db}` via `displayText()`; tables remain; the database is not dropped; **Sequences owned by those tables restart.**; **Cannot be undone.** Do not claim backup verification. Fields: database confirm `autocomplete=off`; owner password `type=password` `autocomplete=current-password`. Confirm Truncate disabled until confirmation equals the selected database **and** password length > 0. Client confirmation is not authorization. POST uses CSRF, `encodeURIComponent(db)`, JSON `{ database_confirmation, owner_password }` only. HTTP 200: close dialog, clear password/confirmation, reload tables; if a table is selected, reload the current row page. `reauth_required`: stay, announce error, **clear password**, keep confirmation. 401: session-expired, clear secrets. Flag-off 403 stays on dialog with **Truncate is turned off.** 409 truncated-list / in-progress stay on dialog. 404 / generic 503 inspector families. Memory only; clear on database change, Back, logout. Search / login / Security overview never POST truncate.

**GET `/api/v1/postgres/security`** requires a session cookie and the `postgres.read` capability, and does not require CSRF. There are no query parameters. The path is exact. Other methods are `405` `method_not_allowed`. Missing session is `401` `unauthorized` with no `summary`, `databases`, `connections`, `saved_credential`, or `truncated` keys. Nil adapter or catalog/query failure is `503` `dependency_unavailable` (same operator copy as other PostgreSQL GETs) and is never a `200` with empty arrays. Responses are `Cache-Control: no-store`. This GET is not a mutation and does not write an audit event. Responses never include passwords, URLs, hashes, role OIDs, raw `datacl`, SQL, `err.Error()`, `connection_limit`, `size`, `size_bytes`, `has_saved_password`, `can_rotate`, or URL templates. `summary.missing_password_count` is present only when the vault existence query succeeded (see below).

This cluster view is a documented delta from `GET /api/v1/postgres/databases`: it lists **all non-template** databases, including `postgres` and `database_console_vault`, so PUBLIC CONNECT on protected targets is visible. Templates (`datistemplate`) are omitted. `protected` is `true` when the row is not `policy.Manageable` (same policy as list/details, including configured deny-lists and the admin database/role). List/details still omit those names (`404`).

**Vault existence (PG-005/PG-012 Partial freeze).** Capability stays `postgres.read` (not `postgres.credentials`). No new paths. No decrypt, `REDGRES_LEGACY_VAULT_SECRET_FILE`, `ensure_vault` DDL, URL templates, or `internal/secrets` on the request path. SQL is `SELECT role_name FROM public.project_credentials WHERE role_name = ANY($1)` (or parameterized `IN` of unique owners, capped by the existing 500-database list) after `connectTarget` to `database_console_vault`. The query must not mention `encrypted_password` or `updated_at`. Sibling `except Exception: return set()` is not copied.

GET `/api/v1/postgres/databases/{db}` `saved_credential` is always present on 200:

| `status` | `reason` | Meaning |
|---|---|---|
| `present` | `""` | Vault query succeeded; owner is in the returned `role_name` set |
| `missing` | `""` | Vault query succeeded; owner is not in the set |
| `not_available` | `vault_unavailable` | Vault DB missing, table missing, CONNECT/query denied, or timeout. **200 still**, not 503 |

Never `vault_not_implemented`. Never ciphertext, `updated_at`, role lists, or passwords.

GET `/api/v1/postgres/security` cluster `saved_credential` is vault-query health, not a password:

| `status` | `reason` | Meaning |
|---|---|---|
| `ok` | `""` | Existence query succeeded |
| `not_available` | `vault_unavailable` | Same failure class; **200** with existing `databases`/`connections` |

Catalog List or connection-group failure remains **503**. `summary.missing_password_count` is a JSON number (including `0`) only when cluster status is `ok`; **omit** the key when `vault_unavailable`. Count is **pre-cap**, over the same non-template `databases` array already returned (includes `postgres` and `database_console_vault`). Missing means the row’s `owner` is not in the returned role_name set.

Documented sibling delta (`security_ops.get_security_overview` at `1c3e8e2`): sibling excludes `database_console_vault` from the database list (`datname <> VAULT_DATABASE`) and uses boolean `has_saved_password`. Redgres keeps listing the vault DB and **does not** add `has_saved_password`. Count may be +1 vs sibling.

**UI copy (frozen with this contract):** Details: Saved / Not saved / Not available. Never render `reason` strings. Security: if `ok`, fact **Missing vault entries** = count; if unavailable, **Saved credential** = Not available. No Reveal/Rotate/Create controls on Security overview. Inspector Rotate is the PG-006 freeze.

**GET `/api/v1/postgres/databases/{db}/connection` (PG-004/PG-005 Partial freeze).** Session cookie and `postgres.read` (not `postgres.credentials`). No CSRF. No query parameters. Register the path **before** `GET .../databases/{db}`. Other methods on `/connection` (no `/reveal`) are `405` `method_not_allowed`. This GET is not a mutation, does not decrypt, and does not write an audit event. Responses are `Cache-Control: no-store`. POST reveal is a separate route freeze below.

Capability and manageability match details: invalid identifier is `400` `validation_error` with no catalog/vault call; protected, template, `datallowconn=false`, and missing names are `404` `not_found` (same operator copy as details); nil adapter or catalog lookup failure is `503` `dependency_unavailable` (“PostgreSQL is unavailable”). Vault connect/query failure is **200** with `saved_credential.status` `not_available` and `reason` `vault_unavailable`, not 503. Missing session is `401` `unauthorized` with no `database`, `owner`, `saved_credential`, or `masked_*` keys.

`saved_credential` is the same existence result as details (`present` / `missing` / `not_available`). SQL stays `SELECT role_name FROM public.project_credentials WHERE role_name = ANY($1)` after `connectTarget` to `database_console_vault`. No `encrypted_password`, `updated_at`, decrypt, `internal/secrets`, or `REDGRES_LEGACY_VAULT_SECRET_FILE`. Sibling `has_saved_password`, `username`, `credential_status`, `direct_url`, `pooled_url`, `url`, `raw_url`, and `masked_url` are **not** used. Never `YOUR_PASSWORD`.

Always on **200:** `database`, `owner`, `saved_credential` `{status, reason}`, `request_id`.

Emit `masked_direct_url` only when **all** of: `saved_credential.status` is `present`, `REDGRES_POSTGRES_PUBLIC_HOST` is set, `REDGRES_POSTGRES_DIRECT_PORT` is set.

Emit `masked_pooled_url` only when **all** of: `present`, `REDGRES_POSTGRES_PUBLIC_HOST` is set, `REDGRES_POSTGRES_POOLED_PORT` is set.

Otherwise **omit the key** (not `null`, not `""`). Missing and `not_available` never receive fake URLs. The mask token `********` appears only inside an emitted URL, never as a standalone field. URLs never copy `REDGRES_POSTGRES_HOST` or admin `REDGRES_POSTGRES_PORT`. Project URLs hardcode `sslmode=require` and do not copy admin `REDGRES_POSTGRES_SSLMODE`. No `sslrootcert` query parameter.

URL shape: `postgresql://{enc_owner}:********@{JoinHostPort(public_host,port)}/{enc_database}?sslmode=require`. Percent-encode **owner** and **database** so the only unescaped bytes are RFC 3986 unreserved (`ALPHA / DIGIT / "-" / "." / "_" / "~"`). Space → `%20` (not `+`). Do not use `url.QueryEscape`. Do not use `url.PathEscape` (it leaves `:` unescaped; sibling `quote(..., safe="")` encodes `app/user:name` as `app%2Fuser%3Aname`). Host uses `net.JoinHostPort` (IPv6 brackets). The password slot is the literal eight asterisks. The builder takes no password argument on this GET path.

**200 present, both public ports set:**

```json
{
  "database": "project_a",
  "owner": "project_a_role",
  "saved_credential": { "status": "present", "reason": "" },
  "masked_direct_url": "postgresql://project_a_role:********@db.example.com:5432/project_a?sslmode=require",
  "masked_pooled_url": "postgresql://project_a_role:********@db.example.com:6432/project_a?sslmode=require",
  "request_id": "<opaque>"
}
```

**200 present, public host unset:** omit both URL keys; still `saved_credential: present`.

**200 missing / not_available:** omit both URL keys; `saved_credential` as on details.

PG-004 and PG-005 stay Partial. GET `/connection` still does not decrypt. Gate 4, live PostgreSQL 17/18, and viewport evidence remain outstanding. Do not mark Complete.

**UI copy (frozen with this GET contract):** Inspector labels **Direct URL** / **Pooled URL** only when the corresponding string is present; copy via `text-button` (no auto-copy, no toast of the URL). Absent keys: no URL rows. Details **Saved credential** remains Saved / Not saved / Not available. Loading: “Loading connection.” 503: “PostgreSQL is unavailable.” 401: session-expired; paint no URLs. Never render `reason` strings. No Create on this GET. Inspector **Reveal** is the POST reveal freeze. Inspector **Rotate** is the PG-006 freeze. Clear masked URLs on selection change, Back, and logout. Memory only.

**POST `/api/v1/postgres/databases/{db}/connection/reveal` (PG-005 Partial freeze).** Session cookie, `postgres.credentials` (not `postgres.read`), and CSRF (`requireMutation`). Register next to GET `/connection` **before** `GET .../databases/{db}`. Other methods on `/reveal` are `405` `method_not_allowed`. Collection POST `/api/v1/postgres/databases` is the PG-003 freeze below (not this reveal route). POST `/connection` (no `/reveal`) stays `405`. There is no feature flag. There is no `POST /api/v1/auth/reauth` and no typed username confirmation (AUTH-006 does not apply). Empty body: do not decode a password; extra JSON is ignored and is never treated as `owner_password`. Path validation matches GET connection (`decodePathIdentifier`). Responses are `Cache-Control: no-store`.

**Check order:**

1. Middleware: session, `postgres.credentials`, CSRF.
2. Path database parse. Invalid → `400` `validation_error`, no catalog, no vault, no audit, raw param not echoed.
3. `Service.Reveal`: manageability matches GET connection (protected/template/`datallowconn=false`/missing → `404` `not_found`, same operator copy as details; nil adapter → `503` `dependency_unavailable`). Empty owner → `404` `not_found`. Then SELECT ciphertext for that owner. Missing vault row → `404` `not_found` (sibling missing-password), **no decrypt**. Vault DB/table/CONNECT/timeout → `503` `dependency_unavailable` (do **not** copy sibling `ensure_vault` DDL; this is not GET’s 200 `not_available`). Secret file unset/unreadable/empty, or derived key missing → `503`. Invalid/wrong-key/tampered token or empty plaintext after decrypt → `503` `dependency_unavailable`. Operator copy is the same “PostgreSQL is unavailable” family; never echo path, env var value, secret, token, ciphertext, or plaintext. Never `err.Error()`.
4. Success audit `postgres.credential.reveal` with metadata `database` and `owner` only (never password, URL, token, ciphertext). Then `200`. Audit insert fail after decrypt → `503` and **do not** return the credential (fail-closed).

**Config:** `REDGRES_LEGACY_VAULT_SECRET_FILE` is an optional path on `config.Config`. It is **not** part of `PostgresConfigured` / `postgresAnySet`. When set, fail-closed file rules match `REDGRES_POSTGRES_PASSWORD_FILE` (`TrimRight` `\r\n`; production not group/world-readable; empty rejected; errors name `REDGRES_LEGACY_VAULT_SECRET_FILE` and never echo contents). `postgresadmin.Open` reads it when PostgreSQL is configured and the path is non-empty, calls `secrets.DeriveVaultKey`, wipes the raw secret, and stores **only the derived Fernet key** on the service (not on `httpapi.Server`). Unset file → Open succeeds; Reveal `503`. Production `serve` does not require the vault secret file (startup “fail if vault rows exist and secret is missing” is out of this slice).

**SQL (new Catalog method, not `SavedRoleNames`):** `SELECT encrypted_password FROM public.project_credentials WHERE role_name = $1` after `connectTarget` to `database_console_vault`. No `updated_at`. No `ensure_vault`. Decrypt with existing `secrets.Decrypt` (no TTL, no new Go module, no Encrypt API). GET `/connection`, details, and security still use existence-only `SavedRoleNames` and must not select ciphertext.

**URLs:** reuse `encodeRFC3986Unreserved` (sibling `quote(..., safe="")`). Hardcode `sslmode=require`. Public host/ports only; never copy admin `REDGRES_POSTGRES_HOST` / `REDGRES_POSTGRES_PORT`. `urls.direct` only if `REDGRES_POSTGRES_PUBLIC_HOST` and `REDGRES_POSTGRES_DIRECT_PORT` are set; `urls.pooled` only if public host + `REDGRES_POSTGRES_POOLED_PORT`. Omit the key (not `null`). Do not emit Redis `urls.primary`. Password is still returned when both URL keys are omitted.

Sibling deltas (do not copy): FastAPI has no CSRF — Redgres still requires AUTH-004. Sibling maps decrypt `InvalidToken` to `404` — Redgres uses `503`. Sibling may `ensure_vault` — Redgres must not. Sibling `{direct_url, pooled_url, has_saved_password}` envelope is **not** used.

Success `200`:

```json
{
  "resource": { "type": "postgres_database", "name": "project_a" },
  "credential": {
    "username": "project_a_role",
    "password": "<plaintext>",
    "one_time": false,
    "urls": {
      "direct": "postgresql://project_a_role:<enc>@db.example.com:5432/project_a?sslmode=require",
      "pooled": "postgresql://project_a_role:<enc>@db.example.com:6432/project_a?sslmode=require"
    }
  },
  "request_id": "<32 lowercase hex>"
}
```

Always on 200: `resource`, `credential.username` (= owner), `credential.password`, `credential.one_time` JSON `false`, `request_id`. Omit `urls` entirely when neither direct nor pooled can be built; omit one URL key when only one public port is set.

PG-005 stays Partial. Gate 4 copied production ciphertext, live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright viewports, and production startup vault-secret probe remain outstanding. POST create is a separate PG-003 freeze. POST rotate is a separate PG-006 freeze. Do not mark Complete.

**UI copy (frozen with this POST contract):** Databases inspector **Reveal** is a `text-button` (not `--danger`) when GET connection `saved_credential.status` is `present`. Hidden for `missing` / `not_available` / loading / error / while details loading. Disabled while reveal is in flight or a credential ticket is open. No confirm dialog. Client Reveal is not authorization. POST uses CSRF, `encodeURIComponent(db)`, empty body. HTTP 200 opens a PostgreSQL credential ticket (`role=alertdialog`): title **This PostgreSQL password is still saved.** Copy: Redgres can show this password again from the encrypted vault. It is not a one-time Redis credential. Fields: username, password, **Direct URL** / **Pooled URL** copy buttons only when `urls.direct` / `urls.pooled` are present. No auto-copy. Dismiss: **I have copied it — dismiss**. 401: session-expired, clear secrets, no leftover password. 404 / 503: same not-found / PostgreSQL-unavailable copy families as other Databases inspector errors; do not paint a ticket. Memory only; clear on dismiss, selection change, Back, logout. Security overview still has **no** Reveal (existing test stays). Login never POSTs `/connection/reveal`. Search never POSTs reveal. Redis create/rotate tickets stay one-time “shown now” copy. Inspector **Rotate** is the PG-006 freeze below.

**POST `/api/v1/postgres/databases/{db}/credentials/rotate` (PG-006 Partial freeze).** Session cookie, `postgres.credentials` (not `postgres.read`, not `postgres.provision`, not `postgres.destructive`), and CSRF (`requireMutation`). Register **before** `GET .../databases/{db}` (next to GET `/connection` and POST `/connection/reveal`):

```go
r.With(s.requireSession, s.requireCapability("postgres.credentials"), s.requireMutation).Post("/api/v1/postgres/databases/{db}/credentials/rotate", s.handlePostgresCredentialsRotate)
```

GET/PUT/PATCH/DELETE on this path are `405` `method_not_allowed`. There is no `POST /api/v1/postgres/databases/{db}/rotate` alias. `POST /api/v1/postgres/databases/{db}` (no suffix) stays `405`. Collection POST `/api/v1/postgres/databases` is the PG-003 freeze. There is no feature flag (`REDGRES_FEATURE_POSTGRES_ROTATE` / sibling `ENABLE_PASSWORD_ROTATION` do not exist). There is no `POST /api/v1/auth/reauth` and no `owner_password` (AUTH-006 does not apply). Handler timeout is **30s**. Responses are `Cache-Control: no-store`. Path validation matches GET connection (`decodePathIdentifier`).

**Body** (`DisallowUnknownFields`). Only:

```json
{ "confirmation": "project_a" }
```

`confirmation` must equal the decoded path `{db}` exactly. Mismatch or empty → `400` `validation_error` with `fields.confirmation`, copy **Type the database name exactly to confirm rotation**. No audit, no catalog, no ALTER. Unknown fields (`password`, `owner_password`, `role_password`) → `400` `validation_error` (`Unknown field`), no audit, no ALTER. Client password is never accepted.

**Check order:**

1. Middleware: session, `postgres.credentials`, CSRF.
2. JSON decode. Invalid JSON / unknown field → `400`, no audit, no ALTER.
3. Confirmation match. Mismatch/empty → `400` as above.
4. Nil adapter / PostgreSQL not configured → `503` `dependency_unavailable`. Failure audit if names are valid.
5. Catalog lookup (re-read immediately before mutation). Protected, template, `datallowconn=false`, missing name, or empty owner → `404` `not_found` (same operator copy as details/reveal). Superuser owner, non-login owner, `OwnerDenied`, or `pg_*` on a looked-up database → `403` `protected_resource`, message **This PostgreSQL name is protected**. Eligibility is the PG-012 formula: owner ≠ `""` && `policy.Manageable` && `rolcanlogin` && `!rolsuper`. Vault `present` / `missing` / `not_available` does **not** gate rotate (upsert INSERT is allowed when the row is missing).
6. Missing/unreadable vault key (`Open` stored none) → **503 before ALTER**.
7. Vault reachability probe (`SavedRoleNames` / connect to `database_console_vault`; **no ciphertext**) → **503 before ALTER** if the vault DB/table is unreachable. Do not `ensure_vault`.
8. Generate password with existing `postgresadmin.GeneratePassword()` (24-byte `crypto/rand` + `RawURLEncoding`). **Do not** copy sibling `secrets.token_hex(32)`. **Do not** import `redisadmin`. Encrypt in memory with `secrets.Encrypt`.
9. In-process **per-owner TryLock**. Fail → `409` `operation_in_progress`, copy **A password rotation is already in progress for this role.** No second ALTER. The lock is request-scoped process memory only (no SQLite `operations` row; ADR-005 `002_operations.sql` is out of this slice).
10. `ALTER ROLE {owner} WITH PASSWORD {literal} CONNECTION LIMIT 20` (`QuoteIdentifier` + existing `quoteStringLiteral`; simple-protocol `Exec`; constant **20**). Do not add `REDGRES_POSTGRES_CONNECTION_LIMIT`.
11. Parameterized vault upsert (retries **3**, no re-ALTER). If upsert still fails after ALTER → **503** `dependency_unavailable`, copy **The PostgreSQL password was changed but the vault could not be saved. Rotate again.** Failure audit. **Do not return the credential.** The next POST rotate is allowed (new password) — that is recovery. Password stays in process memory only.
12. Success audit then **200**. Audit-fail after cluster+vault success → **503** fail-closed, **do not** return the credential (operator can Reveal).

```sql
ALTER ROLE {owner} WITH PASSWORD {literal} CONNECTION LIMIT 20
```

```sql
INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (role_name)
DO UPDATE SET encrypted_password = EXCLUDED.encrypted_password, updated_at = now()
```

Create INSERT stays **without** `ON CONFLICT`. **No `ensure_vault` DDL.** Never `CREATE DATABASE database_console_vault`. Identifiers via `QuoteIdentifier`. Password is interpolated only after PostgreSQL string-literal quoting (wrap in `'…'`, `'` → `''`). Never concatenate unquoted identifiers or password. Never `err.Error()`. Never echo password, token, ciphertext, SQL, or env contents.

**Audit:** success `postgres.credential.rotate` with metadata `database` and `owner` only.

Sibling deltas (do not copy): FastAPI no CSRF; `ensure_vault`; `secrets.token_hex(32)`; feature flag default false; `{database, username, direct_url, pooled_url, warning}` envelope; hardcoded `adminpg`.

Success **200** (not 201; same envelope as create/reveal; `one_time` JSON **false** because the password is vaulted):

```json
{
  "resource": { "type": "postgres_database", "name": "project_a" },
  "credential": {
    "username": "app_project_a",
    "password": "<plaintext>",
    "one_time": false,
    "urls": {
      "direct": "postgresql://app_project_a:<enc>@db.example.com:5432/project_a?sslmode=require",
      "pooled": "postgresql://app_project_a:<enc>@db.example.com:6432/project_a?sslmode=require"
    }
  },
  "request_id": "<32 lowercase hex>"
}
```

Always on 200: `resource`, `credential.username` (= owner), `credential.password`, `credential.one_time` JSON `false`, `request_id`. Reuse `ProjectConnectionURL` / `encodeRFC3986Unreserved`; public host/ports; `sslmode=require`. Omit `urls` keys like reveal. No sibling `warning` field (the warning is UI). No Redis `urls.primary`.

PG-006 stays Partial. Live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright, Gate 4, Python Gate 2 if not executed, `POST /api/v1/auth/reauth`, `ensure_vault`, dual-secret ADR, and production vault-secret probe remain outstanding. Do not mark Complete.

**UI copy (frozen with this POST contract):** Databases inspector **Rotate** is a `text-button` (not `--danger`). Show when details are loaded and `owner` is non-empty and `security.owner_can_login` is true and `security.owner_is_superuser` is false (missing flags → hide), including when `saved_credential.status` is `missing`. Hidden while details loading. Disabled while rotate/reveal/create is in flight or a credential ticket is open. Confirm `role=dialog`, title **Rotate password?**, same focus trap as Redis rotate. Typed database name; **Rotate now** disabled until the confirmation equals the selected database name exactly. Copy: old credentials stop working; the new password is saved in the vault; update every application. Client confirmation is not authorization. POST uses CSRF, `encodeURIComponent(db)`, JSON `{ "confirmation": "<db>" }`. HTTP 200: close dialog, open the **existing** PostgreSQL vault-repeatable ticket (title **This PostgreSQL password is still saved.**) plus ticket `form-warning`: **Update every application using this project user. The previous password stops working.** 401: session-expired, clear secrets, no leftover password. 400/403: stay on the dialog and announce the error. 404 / generic 503: inspector not-found / PostgreSQL-unavailable families; do not paint a ticket. Vault-out-of-sync 503 uses the distinct copy above (password already changed). Memory only; clear on dismiss, selection change, Back, logout. Security overview / search / login never POST rotate. Redis tickets stay one-time “shown now.” Header sentence “Passwords are not revealed.” is out of this freeze.

**POST `/api/v1/postgres/databases/{db}/duplicate` (PG-010 Partial freeze).** Session cookie, `postgres.provision` (not `postgres.read`, not `postgres.credentials`, not `postgres.destructive`), and CSRF (`requireMutation`). Register **before** `GET .../databases/{db}` (next to POST rotate):

```go
r.With(s.requireSession, s.requireCapability("postgres.provision"), s.requireMutation).Post("/api/v1/postgres/databases/{db}/duplicate", s.handlePostgresDatabasesDuplicate)
```

GET/PUT/PATCH/DELETE on this path are `405` `method_not_allowed`. Collection POST `/api/v1/postgres/databases` is the PG-003 freeze. `POST /api/v1/postgres/databases/{db}` (no suffix) stays `405`. There is no feature flag (`REDGRES_FEATURE_POSTGRES_DUPLICATE` / sibling `ENABLE_DESTRUCTIVE_ACTIONS` do not apply). There is no `POST /api/v1/auth/reauth` and no `owner_password` (AUTH-006 does not apply). Handler timeout is **30s**. Compensation after successful DDL uses a **detached** context if the request context is canceled (same as create). Responses are `Cache-Control: no-store`. Path `{db}` is the **source** (`decodePathIdentifier`). This Partial is **201 in-request**. It does not return `202`, does not allocate an operation ID, and does not read or write `GET /api/v1/operations/{id}` or `migrations/002_operations.sql` (ADR-005 later file). Large TEMPLATE clones that exceed 30s remain a Complete / later-operations limitation.

**Body** (`DisallowUnknownFields`). Only:

```json
{ "database": "project_a_copy", "owner": "app_project_a_copy" }
```

`database` is the **new** name. `owner` is the **new** unique project role. No client password. No `create_owner_role`. No `confirmation`. Unknown fields (`new_owner_password`, `create_owner_role`, `password`, `confirmation`, `owner_password`) → `400` `validation_error` (`Unknown field`), no audit, no DDL.

**Check order:**

1. Middleware: session, `postgres.provision`, CSRF.
2. Path `{db}` decode. Invalid identifier → `400` `validation_error`, no catalog.
3. JSON decode. Invalid JSON / unknown field → `400`, no audit, no DDL.
4. Validate new `database` and `owner` with existing `ValidateIdentifier`. Invalid → `400` with `fields.database` and/or `fields.owner`. Copy: **Invalid database name** / **Invalid role name.**
5. Source equals new `database` → `400` `validation_error` with `fields.database`, copy **The copy must use a new database name.** No catalog mutation.
6. Protected **new** names (`Policy.DatabaseDenied` / `OwnerDenied`) → `403` `protected_resource`, **This PostgreSQL name is protected**, no DDL.
7. Nil adapter / PostgreSQL not configured → `503` `dependency_unavailable`. Failure audit if names are valid.
8. Missing/unreadable vault key (`Open` stored none) → **503 before any DDL**.
9. `Service.Duplicate`: source lookup like details. Protected, template, `datallowconn=false`, missing name, or empty owner → `404` `not_found` (same operator copy as details). Source owner superuser, non-login, or `OwnerDenied` → `403` `protected_resource`. New owner equals source owner → `400` `validation_error` with `fields.owner`, copy **Choose a different project user than the source database owner.** New database or role already exists → `409` `conflict` with the same fields/messages as create. Vault reachability probe (`SavedRoleNames` / connect to `database_console_vault`; **no ciphertext**; **no `ensure_vault`**) → **503 before DDL**.
10. In-process TryLock on **source name + new database + new owner**. Fail → `409` `operation_in_progress`, copy **A database duplicate is already in progress.** No second TEMPLATE clone. The lock is request-scoped process memory only. `writePostgresError` must not reuse the rotate 409 sentence for this error.
11. Snapshot source `pg_get_userbyid(datdba)` and `COALESCE(datacl::text, '')` (sibling `_database_ownership_snapshot` at `1c3e8e2`).
12. `CREATE ROLE` using the same SQL as create (`GeneratePassword`, `CONNECTION LIMIT 20`, `QuoteIdentifier` + `quoteStringLiteral`). `GRANT {owner} TO {admin} WITH INHERIT TRUE, SET TRUE` when `cfg.PostgresUser` is not `postgres` and is not the new role.
13. `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()` — **source `datname` only**, parameterized.
14. `CREATE DATABASE {new} TEMPLATE {source} OWNER {new_owner}` (quoted identifiers; simple-protocol `Exec`; **not** inside a transaction). Official PostgreSQL 17 and 18: `CREATE DATABASE` cannot be executed inside a transaction block; no other sessions may be connected to the template database while it is copied ([CREATE DATABASE](https://www.postgresql.org/docs/17/sql-createdatabase.html), [18](https://www.postgresql.org/docs/18/sql-createdatabase.html)).
15. `REVOKE CONNECT ON DATABASE {new} FROM PUBLIC` and `GRANT CONNECT ON DATABASE {new} TO {new_owner}` on the **new** database only.
16. Assert source fingerprint unchanged. Mismatch → compensate (clone + new role + vault row only) then `503` `dependency_unavailable`, copy **The source database ownership or CONNECT ACL changed during duplicate. The clone was rolled back.** Never drop or terminate the source as cleanup.
17. `secrets.Encrypt` + parameterized vault **INSERT** (create SQL; **no** `ON CONFLICT`; **no** `ensure_vault`). Unique violation → **409** + compensate.
18. Per-object `ALTER … OWNER TO` on the **clone only**. Never `REASSIGN OWNED` (official PostgreSQL: that command also reassigns **shared** objects including databases and tablespaces — [REASSIGN OWNED](https://www.postgresql.org/docs/17/sql-reassign-owned.html)). Never `SET ROLE` to the old owner (sibling `_alter_owner_as`). Skip protected / `OwnerDenied` / `pg_*` / **superuser** current owners (no `GRANT … SET TRUE` for skipped owners). Catalog schema/relation/type/routine names are quoted with `pgx.Identifier.Sanitize` **without** the HTTP `ValidateIdentifier` allow-list; empty and NUL fail closed (compensate clone/role/vault). Do not `continue` past an unquotable catalog name. HTTP path/body names stay on `ValidateIdentifier` + `QuoteIdentifier`. Then clone-local `GRANT ALL` / `ALTER DEFAULT PRIVILEGES` on `public` as sibling. Catalog-derived `pg_get_function_identity_arguments` may be interpolated; user input must not.
19. Assert source fingerprint again (same mismatch handling as step 16).
20. Success audit then **201**. Audit-fail after cluster+vault success → **503** fail-closed, **do not** return the credential (operator can Reveal).

```sql
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()
```

```sql
CREATE DATABASE {new} TEMPLATE {source} OWNER {new_owner}
```

```sql
INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at)
VALUES ($1, $2, now())
```

Create INSERT stays **without** `ON CONFLICT`. Rotate upsert is unchanged. Identifiers via `QuoteIdentifier`. Never concatenate unquoted identifiers or password. Never `err.Error()`. Never echo password, token, ciphertext, SQL, or env contents.

**Compensation:** only the **created** clone database, **created** role (if `OwnedDatabaseCount == 0`), and **inserted** vault row. Reuse create’s terminate+`DROP DATABASE IF EXISTS` **on the clone name only**. **Never** drop, alter, or `pg_terminate_backend` the source as cleanup.

**Audit:** success `postgres.database.duplicate` with metadata `database` (new name), `owner` (new role), and `source` (path `{db}`) only.

Sibling deltas (do not copy): FastAPI no CSRF; `ENABLE_DESTRUCTIVE_ACTIONS`; client `new_owner_password`; `ensure_vault`; `{duplicated, new_database, warning, transferred_from, source_owner}` envelope; HTTP 500 + `str(e)`; weaker protected set.

Success **201** (same envelope as create/reveal; `one_time` JSON **false**; `resource.name` is the **new** database; `credential.username` is the **new** owner):

```json
{
  "resource": { "type": "postgres_database", "name": "project_a_copy" },
  "credential": {
    "username": "app_project_a_copy",
    "password": "<plaintext>",
    "one_time": false,
    "urls": {
      "direct": "postgresql://app_project_a_copy:<enc>@db.example.com:5432/project_a_copy?sslmode=require",
      "pooled": "postgresql://app_project_a_copy:<enc>@db.example.com:6432/project_a_copy?sslmode=require"
    }
  },
  "request_id": "<32 lowercase hex>"
}
```

Always on 201: `resource`, `credential.username` (= new owner), `credential.password`, `credential.one_time` JSON `false`, `request_id`. Reuse `ProjectConnectionURL` / `encodeRFC3986Unreserved`; public host/ports; `sslmode=require`. Omit `urls` keys like create. No sibling `warning` field (the warning is UI). No Redis `urls.primary`. No extra `database` object (UI refreshes GET list).

PG-010 stays Partial. Live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright, Gate 4, Python Gate 2 if not executed, `002_operations.sql`, `GET /api/v1/operations/{id}`, `POST /api/v1/auth/reauth`, `ensure_vault`, dual-secret ADR, and production vault-secret probe remain outstanding. Do not mark Complete.

**UI copy (frozen with this POST contract):** Databases inspector **Duplicate** is a `text-button` (not `--danger`), not a header/nav action. Show when details are loaded and `owner` is non-empty and `security.owner_can_login` is true and `security.owner_is_superuser` is false (missing flags → hide). Hidden while details loading. Disabled while duplicate/create/reveal/rotate is in flight or a credential ticket is open. Dialog `role=dialog`, title **Duplicate database**, same focus trap as Create database. Fields: **New database name**, **Project user**. Suggest owner `app_${database}` until the owner field is edited. No password field. Helper: Redgres generates the password and saves it in the encrypted vault. Warning (`form-warning`, not `--danger`): a unique project user is required; **N active connections to {source} will be terminated** (`N` from loaded details `connection_count`, including 0; `{source}` via `displayText`); object owners inside the copy change; source ownership is verified unchanged. Submit **Duplicate** disabled until both identifiers are valid and the new database name is not equal to the source. Client confirmation is not authorization. POST uses CSRF, `encodeURIComponent(source)`, JSON `{ "database", "owner" }`. HTTP 201: close dialog, refresh list, open the **existing** PostgreSQL vault-repeatable ticket (title **This PostgreSQL password is still saved.**). After dismiss, select the **new** database in memory. 401: session-expired, clear secrets, no leftover password. 400/403/409: stay on the dialog and announce the error. 404 / generic 503: inspector not-found / PostgreSQL-unavailable families; do not paint a ticket. Isolation-rollback 503 uses the distinct copy above. Memory only; clear on dismiss, selection change, Back, logout. Security overview / search / login never POST duplicate. No `postgres-duplicate` nav id. Redis tickets stay one-time “shown now.” Header sentence “Passwords are not revealed.” is out of this freeze.

**POST `/api/v1/postgres/databases` (PG-003 Partial freeze).** Session cookie, `postgres.provision` (not `postgres.read`, not `postgres.credentials`, not `postgres.destructive`), and CSRF (`requireMutation`). Register next to GET `/api/v1/postgres/databases`:

```go
r.With(s.requireSession, s.requireCapability("postgres.provision"), s.requireMutation).Post("/api/v1/postgres/databases", s.handlePostgresDatabasesCreate)
```

PUT/PATCH/DELETE on the collection are `405` `method_not_allowed`. POST `/api/v1/postgres/databases/{db}` (no suffix) stays `405`. POST rotate is the PG-006 freeze and POST duplicate is the PG-010 freeze (both registered before GET `{db}`). There is no feature flag (`REDGRES_FEATURE_POSTGRES_CREATE` does not exist). There is no `POST /api/v1/auth/reauth` and no `owner_password` (AUTH-006 does not apply). Handler timeout is **30s**. Compensation after successful DDL uses a **detached** context if the request context is canceled. Responses are `Cache-Control: no-store`.

**Body** (`DisallowUnknownFields`). Only:

```json
{ "database": "project_a", "owner": "app_project_a" }
```

No client password. No `create_role`. No `role_password`. Unknown fields (`role_password`, `create_role`, `password`) → `400` `validation_error` (`Unknown field`), no audit, no DDL.

**Check order:**

1. Middleware: session, `postgres.provision`, CSRF.
2. JSON decode. Invalid JSON / unknown field → `400`, no audit, no DDL.
3. Validate `database` and `owner` with existing `ValidateIdentifier` (`^[A-Za-z_][A-Za-z0-9_]*$`, max 63). Invalid → `400` `validation_error` with `fields.database` and/or `fields.owner`. Copy: **Invalid database name** / **Invalid role name.** Raw values not echoed.
4. Protected database or role (`Policy.DatabaseDenied` / `OwnerDenied`, including `pg_*` prefix, admin DB/user, `database_console_vault`, templates as database names) → `403` `protected_resource`, message **This PostgreSQL name is protected**, no DDL, no secret audit.
5. Nil adapter / PostgreSQL not configured → `503` `dependency_unavailable`. Failure audit if names are valid.
6. Missing/unreadable vault key (`Open` stored none) → **503 before any DDL**.
7. `Service.Create`: existence checks; `CREATE ROLE`; `GRANT` SET if needed; `CREATE DATABASE`; revoke/grant CONNECT; `secrets.Encrypt`; parameterized vault INSERT; success audit; **201**.
8. Any failure after `CREATE ROLE` → compensate, then typed error. Never `err.Error()`. Operator copy for dependency failure is the “PostgreSQL is unavailable” family; never echo password, token, ciphertext, SQL, or env contents.

**Role/database SQL** (sibling `database_ops.create_database` at `1c3e8e2`; no PgBouncer role). Identifiers via `QuoteIdentifier` (`pgx.Identifier.Sanitize`). Password is interpolated only after PostgreSQL string-literal quoting (wrap in `'…'`, `'` → `''`). Never concatenate unquoted identifiers or password. `CREATE DATABASE` is not run inside a transaction (PostgreSQL forbids it); use `Acquire` + `Exec` (simple protocol if the extended protocol cannot run this DDL).

```sql
CREATE ROLE {owner} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT 20 PASSWORD {literal}
```

Then `CREATE DATABASE {db} OWNER {owner}`, `REVOKE CONNECT ON DATABASE {db} FROM PUBLIC`, `GRANT CONNECT ON DATABASE {db} TO {owner}`. Connection limit is the constant **20**. Do not add `REDGRES_POSTGRES_CONNECTION_LIMIT`.

`GRANT {owner} TO {admin} WITH INHERIT TRUE, SET TRUE` only when `cfg.PostgresUser` is not `postgres` and is not the new role (sibling `_ensure_console_can_set_role`). On GRANT failure after `CREATE ROLE`, drop that role.

Password: 24-byte `crypto/rand` then `base64.RawURLEncoding` (same algorithm as Redis `GeneratePassword`; **do not import `redisadmin`**). Duplicate the helper in `postgresadmin`.

**Encrypt / vault.** Implement `secrets.Encrypt` as the Fernet inverse of existing `Decrypt` (no TTL, no new Go module, no `fernet-go`). Roundtrip tests against `internal/secrets/testdata/python49.json` plaintext. DATA_AND_SECRETS Gate 2 is in-scope for writes; Gate 4 stays out. **No `ensure_vault` DDL.** Never `CREATE DATABASE database_console_vault`. Missing vault DB/table/CONNECT after cluster objects exist → compensate then **503**.

```sql
INSERT INTO public.project_credentials (role_name, encrypted_password, updated_at)
VALUES ($1, $2, now())
```

No `ON CONFLICT` (that is rotate). Unique violation → **409** + compensate.

**Compensation** (only resources this call created):

1. If vault row inserted: `DELETE FROM public.project_credentials WHERE role_name = $1` (that role only).
2. If database created: `pg_terminate_backend` where `datname = $1 AND pid <> pg_backend_pid()`; `DROP DATABASE IF EXISTS` quoted name.
3. If role created **and** that role owns **0** databases **and** not `OwnerDenied`: `DROP ROLE IF EXISTS`.
4. Never drop pre-existing databases/roles. Never drop `postgres` / templates / vault DB.

**409 `conflict`:** existing database → message **A PostgreSQL database with this name already exists**, `fields.database`. Existing role → **A PostgreSQL role with this name already exists**, `fields.owner`. No DDL.

**Audit:** success `postgres.database.create` with metadata `database` and `owner` only. Audit-fail after success → **503** and **do not** return the credential (leave cluster+vault; operator can Reveal).

Sibling deltas (do not copy): FastAPI no CSRF; `ensure_vault`; client password; `create_role` reuse; `{created, direct_url, has_saved_password}` envelope.

Success **201** (same envelope as reveal; `one_time` JSON **false** because the password is vaulted). “One-time credentials” in the endpoint table means this HTTP payload is `no-store` and shown now, not Redis `one_time: true`:

```json
{
  "resource": { "type": "postgres_database", "name": "project_a" },
  "credential": {
    "username": "app_project_a",
    "password": "<plaintext>",
    "one_time": false,
    "urls": {
      "direct": "postgresql://app_project_a:<enc>@db.example.com:5432/project_a?sslmode=require",
      "pooled": "postgresql://app_project_a:<enc>@db.example.com:6432/project_a?sslmode=require"
    }
  },
  "request_id": "<32 lowercase hex>"
}
```

Always on 201: `resource`, `credential.username` (= owner), `credential.password`, `credential.one_time` JSON `false`, `request_id`. Reuse `ProjectConnectionURL` / `encodeRFC3986Unreserved`; public host/ports; `sslmode=require`. Omit `urls` keys like reveal. No Redis `urls.primary`. No extra `database` object (UI refreshes GET list).

PG-003 stays Partial. Live PostgreSQL 17/18, COMPATIBILITY.md §6, Playwright, Gate 4, Python Gate 2 if not executed, role-reuse (`create_role: false`), and production vault-secret probe remain outstanding. POST rotate is a separate PG-006 freeze. Do not mark Complete.

**UI copy (frozen with this POST contract):** Databases page header **Create database** (not the global topbar). Hidden while the list is in error / 503. Shown for HTTP 200 including empty list. Disabled while submit is in flight or a credential ticket is open. Dialog `role=dialog`, title **Create database**, same focus trap as Redis create. Fields: **Database name**, **Project user**. Suggest owner `app_${database}` until the owner field is edited (sibling `generateOwnerName` at `1c3e8e2`; suggestion is UI-only). No password field. Helper: Redgres generates the password and saves it in the encrypted vault. One line: direct 5432 vs pooled 6432; TLS required; PUBLIC CONNECT revoked; 20-connection role limit. POST uses CSRF and JSON `{ database, owner }`. HTTP 201: close dialog, refresh list, open the **existing** PostgreSQL credential ticket (title **This PostgreSQL password is still saved.** — same as Reveal). After dismiss, select the new database in memory. 401: session-expired, clear secrets. 403 protected / 409 / 400: stay on the dialog and announce the error. 503: PostgreSQL unavailable. Nav `postgres-create` must render this form, not “This adapter is not available yet.” Security overview / search / login never POST create. Inspector still has no Create. Header sentence “Passwords are not revealed.” is out of this freeze. Redis create/rotate tickets stay one-time “shown now.”

Database rows reuse existing catalog facts (`datname`, owner, `public_can_connect`, owner superuser/login/createdb/createrole/replication, `active_connections` from the existing `pg_stat_activity` count). Sort by database name ascending. Connection groups match database-app `get_security_overview` at `1c3e8e2`: `backend_type = 'client backend' AND pid <> pg_backend_pid()`, grouped by `datname`, `usename`, `client_addr`, `application_name`, `state`. `database` is `datname` or `"(none)"`; `user` is `usename` or `"(unknown)"`; `client` is `COALESCE(client_addr::text, 'local')`; `application` is `application_name` or `"—"`; `state` is `state` or `"unknown"`; `count` is the group size. No query text.

`databases` and `connections` are arrays, never `null`. Each list is hard-capped at 500. `truncated` is a single flag: `true` if **either** the database list or the connection-group list exceeds 500. Summary counts are **before** those caps: `database_count` is the non-template database count; `public_connect_count` is how many of those have `public_can_connect`; `active_connection_count` is the sum of group `count`s; `connection_group_count` is the number of groups. The returned arrays are the first 500 of each sorted list.

Success `200`:

```json
{
  "summary": {
    "database_count": 2,
    "public_connect_count": 1,
    "active_connection_count": 3,
    "connection_group_count": 2,
    "missing_password_count": 1
  },
  "saved_credential": { "status": "ok", "reason": "" },
  "databases": [
    {
      "name": "postgres",
      "owner": "postgres",
      "protected": true,
      "public_can_connect": false,
      "owner_is_superuser": true,
      "owner_can_login": true,
      "owner_createdb": true,
      "owner_createrole": true,
      "owner_replication": true,
      "active_connections": 1,
      "rotation_eligible": false
    }
  ],
  "connections": [
    {
      "database": "postgres",
      "user": "postgres",
      "client": "local",
      "application": "redgres",
      "state": "idle",
      "count": 1
    }
  ],
  "truncated": false,
  "request_id": "<32 hex>"
}
```

When cluster `saved_credential.status` is `not_available`, `reason` is `vault_unavailable`, `missing_password_count` is omitted, and `databases`/`connections` remain present.

**GET `/api/v1/postgres/security` rotation eligibility (PG-012 Partial freeze).** Path, capability, CSRF, audit, and `Cache-Control: no-store` are unchanged. No new route on this GET. POST rotate is a separate PG-006 freeze; this GET still does not register it. Do **not** emit sibling `can_rotate`. Vault `present`/`missing`/`not_available` does not change eligibility. `owner_createdb` / `owner_createrole` / `owner_replication` do not change it. No new catalog SQL: derive in the service layer from fields already on each `databases[]` row.

Every `databases[]` object always includes `rotation_eligible` as a JSON boolean (never omit, never `null`):

```text
rotation_eligible =
  owner != "" &&
  !protected &&
  owner_can_login &&
  !owner_is_superuser
```

`protected` remains `!policy.Manageable(...)` (deny-listed databases/roles, admin database/user, `pg_*` owners, `datallowconn=false`; templates are omitted from this list). Empty owner is `false`. A manageable project owner that can log in and is not a superuser is `true` (example `project_a` / `project_a_role`: `"rotation_eligible": true`). The `postgres` example above is `false`.

Sibling `database-app` `security_ops.get_security_overview` at `1c3e8e2` uses JSON key `can_rotate` = `rolcanlogin && !rolsuper && owner ∉ {postgres, adminpg, database_console, onelife_pg_admin}`. Redgres does **not** emit `can_rotate` and does **not** hardcode `adminpg` (operators add it via `REDGRES_POSTGRES_PROTECTED_ROLES`). Redgres uses the existing Policy plus empty-owner → false.

Still forbidden on this GET: passwords, URLs, hashes, role OIDs, raw `datacl`, SQL, `err.Error()`, `connection_limit`, `size`, `size_bytes`, `has_saved_password`, **`can_rotate`**, URL templates. No summary count of eligible databases. No details-GET `rotation_eligible` field. Eligibility on this GET is diagnostic only; inspector Rotate is the PG-006 freeze.

**UI copy (frozen with this contract):** Keep the Security overview header: “All non-template databases, including protected names. Passwords are not revealed. Rotation is not available.” Add ledger/stack column **Rotation eligible** as the **last** column (after Connections). Values Yes / No / — (missing). No Rotate, Reveal, or Create controls.

PG-012 stays Partial: existence GET, `missing_password_count` (when the vault query succeeds), and read-only `rotation_eligible` are this freeze. POST rotate is a separate PG-006 freeze (Databases inspector only). Gate 4 copied production ciphertext, live PostgreSQL 17/18, and viewport evidence remain outstanding. POST reveal is Databases inspector only (PG-005 Partial); Security overview has no Reveal or Rotate; `REDGRES_LEGACY_VAULT_SECRET_FILE` is POST-reveal `Open`, not this GET. Do not mark Complete.

## Redis endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/redis/status` | Health/performance summary (implemented) |
| GET | `/api/v1/redis/users` | ACL user list (implemented; inspect-only) |
| GET | `/api/v1/redis/users/{username}` | ACL user inspect (implemented; inspect-only) |
| POST | `/api/v1/redis/users` | Create isolated named-preset or custom allow-list user (`cache-read-write` default); one-time credential; `no-store` (implemented) |
| PATCH | `/api/v1/redis/users/{username}` | Named-preset or custom allow-list prefix/grants; password unchanged (implemented) |
| POST | `/api/v1/redis/users/{username}/enable` | Enable (implemented) |
| POST | `/api/v1/redis/users/{username}/disable` | Disable (implemented) |
| POST | `/api/v1/redis/users/{username}/credentials/rotate` | One-time credential; `no-store` (implemented) |
| DELETE | `/api/v1/redis/users/{username}` | Exact confirmation + owner password (REDIS-008/AUTH-006 Partial freeze) |
| GET | `/api/v1/redis/presets` | Versioned named presets/commands for UI/docs (implemented; no Redis) |
| GET | `/api/v1/redis/commands` | Unique-sorted union of named-preset commands (implemented; no Redis) |

There is no generic Redis command endpoint.

**GET `/api/v1/redis/status`** requires a session cookie and the `redis.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny this route today; the session check is what enforces access.

The handler always returns `200` when it runs, even if Redis is down. Redis failure is represented as `state` `unavailable`; it is not a `503`. `503` `dependency_unavailable` happens only when `requireSession` cannot read SQLite. Missing session is `401` `unauthorized` with no `state`, `metrics`, or `reason` keys. `POST`, `PUT`, `PATCH`, and `DELETE` are `405` `method_not_allowed`. The response is `Cache-Control: no-store`. There are no query parameters; the path is exactly `/api/v1/redis/status`. The Status probe uses a 2s timeout (same bound as `platform.Collect`).

Success `200` when Redis is reachable:

```json
{
  "state": "ok",
  "metrics": {
    "version": "8.2.1",
    "uptime_seconds": 123,
    "connected_clients": 4,
    "used_memory_bytes": 1048576,
    "max_memory_bytes": 0,
    "ops_per_sec": 12,
    "db_size": 50,
    "latency_ms": 1.25
  },
  "request_id": "<32 lowercase hex>"
}
```

| `state` | Keys |
|---|---|
| `not_configured` | `state`, `request_id` only (nil adapter or `ErrNotConfigured`) |
| `unavailable` | `state`, `reason`, `request_id`. No `metrics`. |
| `ok` | `state`, `metrics` (all eight keys always present), `request_id`. No `reason`. |

`reason` is present only when `state` is `unavailable`, and is exactly `unreachable`, `auth_failed`, or `permission_denied`. Metrics mapping: `version` ← `redis_version`; `uptime_seconds` ← `uptime_in_seconds`; `connected_clients` ← `connected_clients`; `used_memory_bytes` ← `used_memory`; `max_memory_bytes` ← `maxmemory` (`0` means unlimited and is still present); `ops_per_sec` ← `instantaneous_ops_per_sec`; `db_size` ← `DBSIZE` of the URL-selected database; `latency_ms` ← Ping wall-clock RTT as float64 microseconds/1000 (not the Redis `LATENCY` command). Ping OK but `INFO` or `DBSIZE` failing is `unavailable` with no metrics; missing or unparseable required INFO keys are `unreachable` with no zero-filled `version`. Redis errors are classified on tokens `NOAUTH`, `WRONGPASS`, and `NOPERM`; responses never include `err.Error()`. The payload never includes host, port, DSN, password, token, URL, or `skip_verify`. This GET is not a mutation and does not write an audit event.

GET `/api/v1/status` is unchanged: Redis there remains Ping-only, with `reason` only `unreachable`, and no version/uptime/metrics.

**GET `/api/v1/redis/users`** and **GET `/api/v1/redis/users/{username}`** require a session cookie and the `redis.read` capability, and do not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny these routes today; the session check is what enforces access. Paths are exact. The probe uses a 2s timeout. Responses are `Cache-Control: no-store`. These GETs are not mutations and do not write an audit event. `PUT`, `PATCH`, and `DELETE` on `/api/v1/redis/users` are `405` `method_not_allowed`. `POST` and `PUT` on `/api/v1/redis/users/{username}` are `405` `method_not_allowed`. `PATCH` on `/api/v1/redis/users/{username}` is the named-preset or custom allow-list permissions update. `DELETE` on `/api/v1/redis/users/{username}` is ACL delete (REDIS-008/AUTH-006 Partial freeze below). Missing session is `401` `unauthorized` with no `state`, `users`, `user`, `reason`, or `credential` keys. The GET adapter issues Redis `ACL LIST` only (go-redis `ACLList`). It does not call `ACL GETUSER` or `ACL USERS`.

The handlers always return `200` when they run against a configured or unconfigured adapter, even if Redis is down, except for missing users (`404`) and invalid usernames (`400`). Redis failure is `state` `unavailable`; it is not a `503`. `503` `dependency_unavailable` happens only when `requireSession` cannot read SQLite. Responses never include `err.Error()`, host, URL, `acl_rule`, password, passwords, hash, raw ACL line, or `nopass` as a credential.

List `state` keys:

| `state` | Keys |
|---|---|
| `not_configured` | `state`, `users` (empty array), `request_id` (nil adapter or `ErrNotConfigured`) |
| `unavailable` | `state`, `reason`, `request_id`. No `users`. |
| `ok` | `state`, `users` (never null), `truncated`, `request_id`. No `reason`. |

Detail `state` keys:

| `state` | Keys |
|---|---|
| `not_configured` | `state`, `request_id` only. No `user`. |
| `unavailable` | `state`, `reason`, `request_id`. No `user`. |
| `ok` | `state`, `user`, `request_id`. No `reason`. |

`reason` is present only when `state` is `unavailable`, and is exactly `unreachable`, `auth_failed`, or `permission_denied` (same classification tokens as GET `/api/v1/redis/status`: `NOAUTH`/`WRONGPASS` → `auth_failed`, `NOPERM` → `permission_denied`, else `unreachable`).

Success list `200`:

```json
{
  "state": "ok",
  "users": [
    {
      "username": "project_a",
      "enabled": true,
      "key_pattern": "project_a:*",
      "preset": "cache-read-write",
      "protected": false,
      "rule_fidelity": "exact"
    }
  ],
  "truncated": false,
  "request_id": "<32 lowercase hex>"
}
```

List rows are summaries: `username`, `enabled`, `key_pattern`, `preset`, optional `queue_kind` only when `preset` is `queue-worker`, `protected`, `rule_fidelity`. `commands` and `categories` are omitted. Users are sorted by `username` ascending. The array is hard-capped at 500 (`truncated: true` if more ACL users exist). Protected users (`default`, `admin`, `redact_admin`, and the admin URL username compared with `EqualFold`) are listed.

Success detail `200`:

```json
{
  "state": "ok",
  "user": {
    "username": "project_a",
    "enabled": true,
    "key_pattern": "project_a:*",
    "preset": "cache-read-write",
    "protected": false,
    "rule_fidelity": "exact",
    "commands": ["echo", "get", "ping"],
    "categories": []
  },
  "request_id": "<32 lowercase hex>"
}
```

Detail includes `commands` and `categories` (empty arrays when none). `queue_kind` is omitted unless `preset` is `queue-worker`. `preset` is `cache-read-write` | `read-only` | `queue-worker` | `custom`. `rule_fidelity` is `exact` | `limited`. Category-only or otherwise unmodelable rules are labeled `custom` / `limited` rather than inferred as a named preset. Protected users are visible (`200`, `protected: true`), not `404`. A missing username is `404` `not_found` with message `Not found` (same as a missing PostgreSQL database). Username path segments are `PathUnescape`d then validated: 1–64 Unicode code points, `[A-Za-z0-9_-]` only; empty, `/`, `..`, and controls are rejected. Lookup against parsed ACL names is exact and case-sensitive. Invalid usernames return `400` `validation_error` without echoing the raw parameter and without querying Redis.

**POST `/api/v1/redis/users`** requires a session cookie, the `redis.provision` capability, and CSRF (`requireMutation`). The capability set is currently a static single-owner grant, so the session and CSRF checks enforce access today. The probe uses a 2s timeout. `DisallowUnknownFields` applies. The body may contain `username`, `key_pattern`, optional `preset`, optional `queue_kind`, and optional `commands`. Missing or empty `preset` is `cache-read-write` (existing `{username,key_pattern}` still `201`). `queue_kind` must be `lists`, `streams`, or `sorted-sets` when `preset` is `queue-worker`, and is forbidden otherwise (`400` `validation_error` with `fields.queue_kind`). Named presets (`cache-read-write`, `read-only`, `queue-worker`, including defaulted empty `preset`) omit `commands` or send `[]`; a non-empty `commands` array on a named/defaulted preset is `400` with `fields.commands` before Redis. Custom bodies are `{ "username", "key_pattern", "preset": "custom", "commands": ["echo", "get", "ping"] }` with no `queue_kind`. Empty or omitted `commands` on `custom` is `400` with `fields.commands` (not `fields.preset`). `queue_kind` on `custom` is `400` with `fields.queue_kind`. Custom command validation matches PATCH: lowercase/trim/unique-sort; `^[a-z][a-z0-9-]*$`; ⊆ `AllowedCommands()` (the unique-sorted union of `NamedPresets()[].Commands`). `@…`, `+`, `-`, `~`, `>`, `|`, spaces, `reset`, `flushall`, `acl`, `config`, `eval`, and unknown names are `400` `fields.commands` before Redis. Raw `commands` length above `maxACLCommands` (256) is rejected before unique-sort; after unique-sort the count must be at least 1 and at most 256. Any other `preset` is `400` with `fields.preset`. `categories`, `enabled`, and `password` remain unknown fields (`400`). The server always creates the user `on`. Valid named presets are `cache-read-write`, `read-only`, and `queue-worker`.

Create username validation is `^[a-z0-9][a-z0-9_-]{2,47}$` (3–48 lowercase). Inspect GET usernames stay 1–64 `[A-Za-z0-9_-]`. `key_pattern` is normalized as redis-ui `NormalizePrefix`: trim `:*` / `:`; reject whitespace, controls, and wildcards `*?[]`; 2–80 characters; `^[a-z0-9][a-z0-9_:-]{0,78}[a-z0-9]$` or `^[a-z0-9]{2}$`; applied as `prefix:*`. Protected names (`default`, `admin`, `redact_admin`, and the configured Redis administrator compared with `EqualFold`) return `403` `protected_resource`. The password is 192-bit URL-safe (`crypto/rand` + `base64.RawURLEncoding`, 24 bytes → 32 characters) and is never client-supplied.

The adapter issues one Redis `ACL LIST` to detect an exact username match (`409` `conflict` because `ACL SETUSER` upserts), then one go-redis `ACLSetUser` with `reset`, `on`, `>password`, one `~prefix:*`, `resetchannels`, `-@all`, and `+CMD` (uppercase) for the resolved command set (named inspect set, or custom through `AllowedCommands()`). There is no `+@all` and no `nocommands` (PATCH-only). It does not grant ACL/CONFIG/FLUSH/SCRIPT/EVAL. It does not call `ACL GETUSER`, `ACL USERS`, `ACLGenPass`, generic `Do`, or `CLIENT KILL`. Create is `CreateUser(ctx, username, keyPattern, preset, queueKind string, commands []string)`; empty `preset` means `cache-read-write`. Named/empty-preset create does not use `resolveUpdateGrants` (that rejects empty `preset`). `preset` `custom` uses `resolveCustomCommands` with empty `queueKind`. If a custom command set equals a named inspect set, `user.preset` may be that named preset (`inferPreset`); the handler does not force `preset: custom`. The password is generated in the service and is never taken from the POST body.

Success `201` when `SETUSER` succeeded:

```json
{
  "resource": { "type": "redis_user", "name": "project_a" },
  "user": { "username": "project_a", "enabled": true, "key_pattern": "project_a:*", "preset": "cache-read-write", "protected": false, "rule_fidelity": "exact" },
  "credential": { "username": "project_a", "password": "<one-time>", "one_time": true },
  "request_id": "<32 hex>"
}
```

`credential.urls` is omitted unless both `REDGRES_REDIS_PUBLIC_HOST` and `REDGRES_REDIS_PUBLIC_PORT` are set. Then `urls.primary` is `rediss://` + URL-encoded userinfo + `host:port` + `/0`. The URL never copies administrator userinfo or host. There is no silent port default. Responses are `Cache-Control: no-store`. Errors never include `err.Error()`, passwords, URLs, or hashes. Create `user` is a summary: `commands` are omitted. `user.queue_kind` is present only when `preset` is `queue-worker`.

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing session. No `credential`, `user`, or `state`. |
| 403 | `csrf_invalid` | Origin or CSRF failure. |
| 400 | `validation_error` | Unknown field, invalid JSON, or `fields.username` / `fields.key_pattern` / `fields.preset` / `fields.queue_kind` / `fields.commands` (raw illegal values are not echoed). |
| 403 | `protected_resource` | Reserved or configured administrator username. No SETUSER. |
| 409 | `conflict` | Exact username already present in `ACL LIST`. |
| 503 | `dependency_unavailable` | Nil adapter, `ErrNotConfigured`, Redis failure, public-URL build failure, or audit insert fail after SETUSER. No `reason` key. Never Redis `ERR` / `err.Error()` / `>password`. |

The action is `redis.user.create`; target is the username; outcome is success or failure; metadata is `username`, `preset` (the actual/inferred preset), and `key_pattern`, plus `queue_kind` only when the actual preset is `queue-worker`. Passwords, URLs, CSRF values, `>` modifiers, and the command list are never audited. If `SETUSER` succeeds but the audit insert fails, the handler returns `503` and does not return the credential. If `SETUSER` fails, a failure audit is written and the client receives typed `503` `dependency_unavailable` only — never Redis `ERR` text, `err.Error()`, or a `>password` modifier.

**PATCH `/api/v1/redis/users/{username}`** requires a session cookie, the `redis.provision` capability, and CSRF (`requireMutation`). It is not `redis.destructive` and does not require owner reauthentication. Username path validation matches GET inspect (`parseRedisUsernameParam`: 1–64 `[A-Za-z0-9_-]`), not the create regex. Invalid usernames return `400` `validation_error` without querying Redis, without writing audit, and without echoing the raw parameter. The probe uses a 2s timeout. `DisallowUnknownFields` applies. The body always requires `key_pattern` (`NormalizePrefix`) and `preset`. Named presets are `cache-read-write` | `read-only` | `queue-worker`; `queue_kind` is required iff `preset` is `queue-worker` and forbidden otherwise. Named bodies omit `commands` or send an empty array; a non-empty `commands` array on a named preset is `400` with `fields.commands`. Custom bodies are `{ "key_pattern", "preset": "custom", "commands": ["echo", "get", "ping"] }` with no `queue_kind`. Empty or omitted `commands` on `custom` is `400` with `fields.commands`. Every custom command is lowercased, trimmed, and unique-sorted; it must match `^[a-z][a-z0-9-]*$` and be in `AllowedCommands()` (the unique-sorted union of `NamedPresets()[].Commands`). `@…`, `+`, `-`, `~`, `>`, `|`, spaces, `reset`, `flushall`, `acl`, `config`, `eval`, and unknown names are `400` `fields.commands` before Redis. Raw `commands` length above `maxACLCommands` (256) is rejected before unique-sort; after unique-sort the count must be at least 1 and at most 256. Omitted, empty, or any other `preset` is `400` with `fields.preset`; empty `preset` does not default to `cache-read-write`. `categories`, `username`, `enabled`, and `password` remain unknown fields (`400`). `PUT` and `DELETE` on `/api/v1/redis/users/{username}` stay `405`. `PATCH` on the collection stays `405`.

The service is `UpdatePermissions(ctx, username, keyPattern, preset, queueKind string, commands []string)`. Empty `preset` does not default. Named presets require empty `commands` (`ErrInvalidCommands` otherwise). `preset` `custom` requires empty `queueKind` and resolves grants only through `AllowedCommands()` (`ErrInvalidCommands` / `ErrInvalidQueueKind` / `ErrInvalidPreset`). Protected names return `403` `protected_resource` without SETUSER. A missing user returns `404` `not_found` with message `Not found` without SETUSER. The adapter loads the user via `ACL LIST` (`GetUser`), then issues one `ACLSetUser` with `resetkeys`, one `~prefix:*`, `resetchannels`, `nocommands`, `-@all`, and `+CMD` (uppercase) for the resolved command set. It does not send `reset`, `resetpass`, `>…`, `on`, or `off`, and does not call `ACL GETUSER`, `ACL USERS`, `ACLGenPass`, generic `Do`, or `CLIENT KILL`. There is no `+@all`. It does not grant ACL/CONFIG/FLUSH/SCRIPT/EVAL. Enabled state and password hashes remain unchanged. `custom` / `limited` / disabled users may be PATCHed to a named preset or to a custom allow-list subset. If a custom command set equals a named inspect set, inspect `preset` may be that named preset; the handler does not force `preset: custom`. The same body still calls SETUSER and still returns `200`.

Success `200` when SETUSER and the success audit both succeed:

```json
{
  "user": { "username": "project_a", "enabled": true, "key_pattern": "project_b:*", "preset": "read-only", "protected": false, "rule_fidelity": "exact", "commands": ["..."], "categories": [] },
  "request_id": "<32 hex>"
}
```

`user` is inspect detail (same fields as GET detail / enable). There is no `state`, `credential`, `reason`, or password key. `user.queue_kind` is present only when `preset` is `queue-worker`. Responses are `Cache-Control: no-store`. Errors never include `err.Error()`, Redis `ERR` text, `>password`, passwords, URLs, or hashes.

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing session. No `user`. |
| 403 | `csrf_invalid` | Origin or CSRF failure. |
| 400 | `validation_error` | Invalid username path (no Redis, no audit, raw param not echoed), unknown field, invalid JSON, or `fields.key_pattern` / `fields.preset` / `fields.queue_kind` / `fields.commands`. |
| 403 | `protected_resource` | Reserved or configured administrator. No SETUSER. Failure audit. |
| 404 | `not_found` | Username absent from `ACL LIST`. Message `Not found`. No SETUSER. Failure audit. |
| 503 | `dependency_unavailable` | Nil adapter, `ErrNotConfigured`, Redis failure, or audit insert fail after SETUSER. No `reason` key. Never Redis `ERR` / `err.Error()` / `>password`. |

The action is `redis.user.update`; target is the username; metadata is `username`, `preset`, and `key_pattern`, plus `queue_kind` only when the actual preset is `queue-worker`. Passwords, URLs, CSRF values, `>` modifiers, and the command list are never audited. If SETUSER succeeds but the audit insert fails, the handler returns `503` and does not return `user`.

**POST `/api/v1/redis/users/{username}/enable`** and **POST `/api/v1/redis/users/{username}/disable`** require a session cookie, the `redis.provision` capability, and CSRF (`requireMutation`). They are not `redis.destructive`. Username path validation matches GET inspect (`parseRedisUsernameParam`). There is no JSON body. `GET`, `PUT`, `PATCH`, and `DELETE` on these two paths are `405` `method_not_allowed`. `POST /api/v1/redis/users/{username}` (no suffix) stays `405`.

The adapter loads the user via `ACL LIST` (`GetUser`), then issues one `ACLSetUser` with only `on` or `off`. It does not send `reset`, `resetpass`, `resetkeys`, `resetchannels`, `nocommands`, `-@all`, `~…`, or `>…`. Protected names return `403` without SETUSER. A missing user returns `404` without SETUSER. `custom` / `limited` users may be toggled. Already-on enable and already-off disable still call SETUSER and still return `200`.

Success `200` is inspect detail (same `user` fields as GET detail) plus `request_id`. No `state`, `credential`, `reason`, or password keys. `Cache-Control: no-store`.

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing session. No `user`. |
| 403 | `csrf_invalid` | Origin or CSRF failure. |
| 400 | `validation_error` | Invalid username path. No Redis. No audit. Raw param not echoed. |
| 403 | `protected_resource` | Reserved or configured administrator. No SETUSER. Failure audit. |
| 404 | `not_found` | Username absent from `ACL LIST`. Message `Not found`. No SETUSER. Failure audit. |
| 503 | `dependency_unavailable` | Nil adapter, `ErrNotConfigured`, Redis failure, or audit insert fail after SETUSER. No `reason` key. Never Redis `ERR` / `err.Error()` / `>password`. |

Actions are `redis.user.enable` / `redis.user.disable`; target is the username; metadata is `username` only. If SETUSER succeeds but the audit insert fails, the handler returns `503` and does not return `user`. Disable blocks new AUTH; it does not kill existing connections.

**POST `/api/v1/redis/users/{username}/credentials/rotate`** requires a session cookie, the `redis.credentials` capability, and CSRF (`requireMutation`). It is not `redis.destructive` and does not require owner reauthentication. Username path validation matches GET inspect (`parseRedisUsernameParam`: 1–64 `[A-Za-z0-9_-]`), not the create regex. There is no JSON body; a body, if present, is ignored and never supplies the password. `GET`, `PUT`, `PATCH`, and `DELETE` on this path are `405` `method_not_allowed`. There is no `POST /api/v1/redis/users/{username}/rotate` alias. `POST /api/v1/redis/users/{username}` (no suffix) stays `405`. The probe uses a 2s timeout. Responses are `Cache-Control: no-store`.

The service generates the password with `GeneratePassword()` (24 bytes, `base64.RawURLEncoding`, 32 characters — same as create) and does not accept a client password. The adapter loads the user via `ACL LIST` (`GetUser`) first because `ACL SETUSER` upserts, then issues one `ACLSetUser` with only `resetpass` and `>password`. It does not send `reset`, `resetkeys`, `resetchannels`, `nocommands`, `-@all`, `on`/`off`, or `~…`, and does not call `ACL GETUSER`, `ACL USERS`, `ACLGenPass`, generic `Do`, or `CLIENT KILL`. Protected names return `403` `protected_resource` without SETUSER. A missing user returns `404` `not_found` with message `Not found` without SETUSER. `custom` / `limited` / disabled users may be rotated.

Success `200` when SETUSER and the success audit both succeed:

```json
{
  "resource": { "type": "redis_user", "name": "project_a" },
  "user": { "username": "project_a", "enabled": true, "key_pattern": "project_a:*", "preset": "cache-read-write", "protected": false, "rule_fidelity": "exact", "commands": ["echo", "get", "ping"], "categories": [] },
  "credential": { "username": "project_a", "password": "<one-time>", "one_time": true },
  "request_id": "<32 hex>"
}
```

`user` is inspect detail (same fields as GET detail / enable). There is no `state` key. `credential.urls` is omitted unless both `REDGRES_REDIS_PUBLIC_HOST` and `REDGRES_REDIS_PUBLIC_PORT` are set. Then `urls.primary` is `rediss://` + URL-encoded userinfo + `host:port` + `/0`. The URL never copies administrator userinfo or host. There is no silent port default. Errors never include `err.Error()`, Redis `ERR` text, `>password`, passwords, URLs, or hashes. There is no `reason` key.

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing session. No `credential`, `user`, or `state`. |
| 403 | `csrf_invalid` | Origin or CSRF failure. |
| 400 | `validation_error` | Invalid username path. No Redis. No audit. Raw param not echoed. |
| 403 | `protected_resource` | Reserved or configured administrator. No SETUSER. Failure audit. |
| 404 | `not_found` | Username absent from `ACL LIST`. Message `Not found`. No SETUSER. Failure audit. |
| 503 | `dependency_unavailable` | Nil adapter, `ErrNotConfigured`, Redis failure, public-URL build failure, or audit insert fail after SETUSER. No `reason` key. Never Redis `ERR` / `err.Error()` / `>password`. |

The action is `redis.user.rotate`; target is the username; metadata is `username` only. Passwords, URLs, and CSRF values are never audited. If SETUSER succeeds but the audit insert fails, the handler returns `503` and does not return `credential`, `password`, or `user`. Rotation invalidates the previous password immediately. If the one-time response is lost, rotate again.

**GET `/api/v1/redis/presets`** requires a session cookie and the `redis.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant, so the session check enforces access today. There is no Redis call and no audit event. The handler returns `200` even when the Redis adapter is nil or not configured. Other methods are `405` `method_not_allowed`. Missing session is `401` `unauthorized` with no `presets` or `state` keys. This GET is not a credential-bearing route.

The catalog is the five named create grant sets in this order: `cache-read-write`, `read-only`, then `queue-worker` `lists`, `streams`, and `sorted-sets`. `commands` are unique-sorted lowercase and byte-equal to the create `+CMD` grant set. There is no `custom` entry and no `state` or `reason`. `presets` is an array, never `null`. `queue_kind` is omitted except on `queue-worker` rows.

Success `200`:

```json
{
  "presets": [
    { "preset": "cache-read-write", "commands": ["..."] },
    { "preset": "read-only", "commands": ["..."] },
    { "preset": "queue-worker", "queue_kind": "lists", "commands": ["..."] },
    { "preset": "queue-worker", "queue_kind": "streams", "commands": ["..."] },
    { "preset": "queue-worker", "queue_kind": "sorted-sets", "commands": ["..."] }
  ],
  "request_id": "<32 lowercase hex>"
}
```

**GET `/api/v1/redis/commands`** requires a session cookie and the `redis.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant, so the session check enforces access today. There is no Redis call and no audit event. The handler returns `200` even when the Redis adapter is nil or not configured. Other methods are `405` `method_not_allowed`. Missing session is `401` `unauthorized` with no `commands` or `state` keys. This GET is not a credential-bearing route.

`commands` is the unique-sorted lowercase union of `NamedPresets()[].Commands` and is byte-equal to `AllowedCommands()`. The array is never `null`. There is no `state`, `reason`, or `preset` key.

Success `200`:

```json
{
  "commands": ["append", "bitcount", "bitpos"],
  "request_id": "<32 lowercase hex>"
}
```

**DELETE `/api/v1/redis/users/{username}` (REDIS-008 / AUTH-006 Partial freeze).** Session cookie, `redis.destructive` capability, and CSRF (`requireMutation`). It is not `redis.provision` or `redis.credentials`. Username path validation matches GET inspect (`parseRedisUsernameParam`: 1–64 `[A-Za-z0-9_-]`). Invalid usernames return `400` `validation_error` without querying Redis, without writing audit, and without echoing the raw parameter. The probe uses a 2s timeout (`redisUsersTimeout`). Responses are `Cache-Control: no-store`. There is no `REDGRES_FEATURE_REDIS_USER_DELETE` flag. There is no `POST /api/v1/auth/reauth` and no short-lived reauth grant (that needs a future ADR). Collection `DELETE /api/v1/redis/users` stays `405`. Item `POST` and `PUT` (no suffix) stay `405`. Enable/disable/rotate suffixes are unchanged. Do not bump go-redis (`v9.22.0`).

`DisallowUnknownFields` applies. Unknown fields (including `password`, `username`, `reason`) return `400` `validation_error` with message `Unknown field`.

Body:

```json
{ "username_confirmation": "project_a", "owner_password": "<owner password>" }
```

**Check order:**

1. Middleware: session, capability, CSRF.
2. Path username parse.
3. JSON decode.
4. `username_confirmation` **exact** match to the parsed path username. Mismatch or empty → `400` `validation_error` with `fields.username_confirmation`, message `Type the exact Redis username to confirm deletion`, no Redis, **no audit**.
5. AUTH-006: `LookupOwnerByUsername(sess.Username)` + `Verify`. Wrong password → `403` `reauth_required`, message `Owner password is incorrect`, failure audit action `redis.user.delete` with metadata `username` only (never `reason: reauth`, never the password), **no Redis**. Missing owner (`sql.ErrNoRows`) → `VerifyUnknown` then `401` `unauthorized`, no Redis, no audit. Other SQLite lookup errors → `503` `dependency_unavailable`. **Do not** increment AUTH-005 login-attempt rows. **Do not** return `429` from this handler.
6. `Service.DeleteUser`: protected (`default`, `admin`, `redact_admin`, configured admin `EqualFold`) → `403` `protected_resource`, message `This Redis user is protected`, failure audit, **no** `ACL DELUSER`. Then `GetUser` (`ACL LIST`); missing → `404` `not_found`, message `Not found`, failure audit, no DELUSER. Then one `ACLDelUser` (go-redis `ACLDelUser(ctx, username) *IntCmd` → `(int64, error)`). `n == 0` → `404` `Not found`, failure audit. Redis/adapter errors → `503` `dependency_unavailable` (same operator copy as other Redis mutations), never `err.Error()`, never `owner_password`.
7. Success audit (`redis.user.delete`, metadata `username` only) then `200`. Audit insert fail after DELUSER → `503` `dependency_unavailable` (user already gone; same fail-closed pattern as create/rotate).

Redis commands on this path: `ACL LIST` (existence) + `ACL DELUSER`. No `ACL SETUSER`, `CLIENT KILL`, `KEYS`, `DEL`, `FLUSH*`, generic `Do`. Official Redis `ACL DELUSER` deletes the user, terminates that user’s connections, cannot remove `default`, and returns the count deleted. Keys are not deleted.

Success `200`:

```json
{ "request_id": "<32 lowercase hex>" }
```

No `ok`, `user`, `state`, `credential`, `reason`. Sibling redis-ui `{ok: true}` is **not** copied.

AUTH-006 stays Partial: Redis ACL delete plus PG-008 row delete plus PG-009 truncate (separate freezes). PostgreSQL drop reauth is not REDIS-008. REDIS-008 stays Partial: MemoryClient/HTTP/jsdom; not COMPATIBILITY.md §6; not Playwright viewports. Do not mark Complete.

**UI copy (frozen with this contract):** Inspector **Delete** for non-protected users when list `state` is `ok`; hidden for protected / `not_configured` / `unavailable` / while detail loading. Danger surface (`--danger`, not Redis identity `--redis`). Disabled while delete/enable/rotate/edit is in flight or a credential ticket is open. Dialog `role=dialog` title **Delete Redis user**. Copy: type the exact username and owner password; this removes the ACL user; **existing Redis connections for that user are terminated; keys are not deleted; cannot be undone**. Fields: username confirm `autocomplete=off`; owner password `type=password` `autocomplete=current-password`. Confirm **Delete** disabled until confirmation equals the selected username **and** password length > 0. Client confirmation is not authorization. CSRF + `encodeURIComponent(username)` + JSON body. HTTP 200: close dialog, **clear password and confirmation**, clear inspector selection, refresh list. 401: session-expired, clear secrets, no leftover password. `reauth_required`: stay on dialog, announce error, **clear password field**, keep confirmation. 403 protected / 404 / 503: same copy families as rotate. State in memory only; clear on logout, section change, inspect another user, dismiss. Search still never deletes. Login never DELETE.

## Credential payload

Credential-bearing response shape:

```json
{
  "resource": { "type": "redis_user", "name": "project_a" },
  "credential": {
    "username": "project_a",
    "password": "shown-only-in-this-response",
    "urls": { "primary": "rediss://..." },
    "one_time": true
  },
  "request_id": "..."
}
```

PostgreSQL may report `one_time: false` only when the same credential remains intentionally recoverable from the encrypted vault. The response still receives no-store handling.

## Audit actions

Stable action vocabulary includes:

```text
owner.login, owner.logout
postgres.database.create, postgres.database.duplicate, postgres.database.drop
postgres.database.truncate, postgres.rows.delete
postgres.credential.reveal, postgres.credential.rotate
redis.user.create, redis.user.update, redis.user.enable, redis.user.disable
redis.user.rotate, redis.user.delete
platform.backup.request, platform.verify.request
```

Audit metadata may include names, preset, prefix, counts, operation ID, and safe failure category. It must not include passwords, full URLs, ciphertext, session/CSRF values, token files, raw database errors, SQL text containing values, or Authorization/Cookie headers.
