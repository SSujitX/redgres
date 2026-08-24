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

`tool_links` stays empty until optional tool-link configuration exists.

**GET `/api/v1/status`** requires a session cookie and the `platform.read` capability, and does not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny this route today; the session check is what enforces access.

The handler always returns `200` when it runs, even if every component is down. Partial failure is represented per component; it is not a `503`. `503` `dependency_unavailable` happens only when `requireSession` cannot read SQLite (the same control-plane storage failure as other authenticated routes). Missing session is `401` `unauthorized` with no `components` key. `POST`, `PUT`, `PATCH`, and `DELETE` are `405` `method_not_allowed`. The response is `Cache-Control: no-store`. There are no query parameters; the path is exactly `/api/v1/status`.

Success `200`:

```json
{
  "components": [
    { "id": "redgres_state", "state": "ok" },
    { "id": "postgres_direct", "state": "not_configured" },
    { "id": "pgbouncer", "state": "not_implemented" },
    { "id": "redis", "state": "not_configured" },
    { "id": "tool_links", "state": "not_configured" }
  ],
  "request_id": "<32 lowercase hex>"
}
```

`components` is always an array of length 5 in this fixed order, never `null`. `state` is one of `ok`, `unavailable`, `not_configured`, `not_implemented`. `reason` is omitted except when `state` is `unavailable`, in which case it is always `"unreachable"`. The payload never includes host, port, DSN, password, token, SQL, driver text, `err.Error()`, version, uptime, or URLs.

Independent checks, sequential, each with a 2s timeout:

| `id` | Probe |
|---|---|
| `redgres_state` | SQLite `PingContext`. `ok` or `unavailable` + `unreachable`. |
| `postgres_direct` | Absent adapter → `not_configured`. Else `Inventory.Ping`: `ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Ping uses `pgxpool.Ping` on the admin pool. List/details are not used as health. |
| `pgbouncer` | Always `not_implemented` in this slice. |
| `redis` | Absent adapter → `not_configured`. Else `Service.Ping`: `ErrNotConfigured` → `not_configured`; success → `ok`; any other error → `unavailable` + `unreachable`. Ping uses go-redis `Ping` only. INFO, DBSIZE, latency, and ACL are not used as health. |
| `tool_links` | Always `not_configured` in this slice. |

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

**GET `/api/v1/healthz`** is unchanged: unauthenticated liveness that pings the state DB only. Success `200` is `{"status":"ok","request_id":"…"}` with `Cache-Control: no-store`. It has no `components` array, does not require a session, and does not ping PostgreSQL, Redis, or PgBouncer.

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
| POST | `/api/v1/postgres/databases` | Create database/role; one-time credentials; `no-store` |
| GET | `/api/v1/postgres/databases/{db}` | Details/security metadata (implemented; vault status is `not_available`) |
| GET | `/api/v1/postgres/databases/{db}/connection` | Masked URLs and saved status only |
| POST | `/api/v1/postgres/databases/{db}/connection/reveal` | Full saved URLs; `no-store`; optionally fresh reauth by policy |
| POST | `/api/v1/postgres/databases/{db}/credentials/rotate` | Rotate; typed confirmation; `no-store` |
| POST | `/api/v1/postgres/databases/{db}/duplicate` | Starts bounded operation; may return 202 + operation ID |
| DELETE | `/api/v1/postgres/databases/{db}` | Exact confirmation + owner password |
| GET | `/api/v1/postgres/databases/{db}/tables` | BASE TABLE names (implemented; cap 500) |
| GET | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | Bounded rows/search (implemented; offset/limit; `q` max 128) |
| DELETE | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | PK values + confirmations/reauth |
| POST | `/api/v1/postgres/databases/{db}/truncate` | Explicit target confirmation + reauth |
| GET | `/api/v1/postgres/security` | Cluster/project security overview |

Database/role names in URL segments are decoded then validated. Transport validation never replaces PostgreSQL identifier quoting.

**Implemented now:** `GET /api/v1/postgres/databases`, `GET /api/v1/postgres/databases/{db}`, `GET /api/v1/postgres/databases/{db}/tables`, and `GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` require a session and the `postgres.read` capability. List and table list are unpaginated and hard-capped at 500 (`truncated: true` if more rows exist). Details include owner, size, collation/ctype, locale fields, connection count, and security flags. `saved_credential.status` is always `not_available` with reason `vault_not_implemented` in this slice; no vault query or decrypt occurs. Table list returns `{schema,name}` for `information_schema` `BASE TABLE` rows outside `pg_catalog` and `information_schema`. Schema/table names on the table list are result columns. Row browse quotes schema, table, and column names with `pgx.Identifier` and parameterizes values. Query `q` is optional (no minimum); more than 128 Unicode code points returns `400` `validation_error` with `fields.q` and does not query. `q` `ILIKE`s columns whose `data_type` contains `text`, `character`, or `citext` (same predicate as `database-app` `fetch_table_data` at `1c3e8e2`; `citext` stored as `USER-DEFINED` is therefore usually not searched). `%` and `_` in `q` remain LIKE wildcards. Default `limit` is 50; `limit<=0` or `limit>500` clamps to 50; `offset<0` clamps to 0; non-integer `limit`/`offset` is `400`. Response is `{columns,rows,total,offset,limit,request_id}`. A missing or non-`BASE TABLE` schema/table, `pg_catalog`/`information_schema`, or a table with no columns is `404` `not_found` (same message as a missing database). An existing table with columns and zero matching rows is `200` with `rows: []` and `total: 0`. Cell values use JSON-safe encoding (`null`, bool, finite numbers, `numeric` as string, `bytea` as PostgreSQL `\x` hex text, timestamps as RFC3339). Encode/connect/query failure is `503` `dependency_unavailable` and never a healthy empty page. Protected names, protected owners, templates, and `datallowconn=false` are omitted from the list and return the same `404` `not_found` as a missing database (not `protected_resource`); table list and rows do not open a per-database connection for those names. Invalid identifiers return `400` `validation_error` without querying. `GET /api/v1/healthz` does not ping PostgreSQL. `DELETE .../rows` is not implemented.

## Redis endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/redis/status` | Health/performance summary (implemented) |
| GET | `/api/v1/redis/users` | ACL user list (implemented; inspect-only) |
| GET | `/api/v1/redis/users/{username}` | ACL user inspect (implemented; inspect-only) |
| POST | `/api/v1/redis/users` | Create isolated cache-read-write user; one-time credential; `no-store` (implemented) |
| PATCH | `/api/v1/redis/users/{username}` | Permissions/prefix only |
| POST | `/api/v1/redis/users/{username}/enable` | Enable |
| POST | `/api/v1/redis/users/{username}/disable` | Disable |
| POST | `/api/v1/redis/users/{username}/credentials/rotate` | One-time credential; `no-store` |
| DELETE | `/api/v1/redis/users/{username}` | Exact confirmation + owner password |
| GET | `/api/v1/redis/presets` | Versioned available presets/commands for UI/docs |

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

**GET `/api/v1/redis/users`** and **GET `/api/v1/redis/users/{username}`** require a session cookie and the `redis.read` capability, and do not require CSRF. The capability set is currently a static single-owner grant (the same list `GET /api/v1/session` returns), so the capability check cannot deny these routes today; the session check is what enforces access. Paths are exact. The probe uses a 2s timeout. Responses are `Cache-Control: no-store`. These GETs are not mutations and do not write an audit event. `PUT`, `PATCH`, and `DELETE` on `/api/v1/redis/users` are `405` `method_not_allowed`. `POST`, `PUT`, `PATCH`, and `DELETE` on `/api/v1/redis/users/{username}` are `405` `method_not_allowed`. Missing session is `401` `unauthorized` with no `state`, `users`, `user`, `reason`, or `credential` keys. The GET adapter issues Redis `ACL LIST` only (go-redis `ACLList`). It does not call `ACL GETUSER` or `ACL USERS`.

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

**POST `/api/v1/redis/users`** requires a session cookie, the `redis.provision` capability, and CSRF (`requireMutation`). The capability set is currently a static single-owner grant, so the session and CSRF checks enforce access today. The probe uses a 2s timeout. `DisallowUnknownFields` applies. The body may contain only `username` and `key_pattern`. `preset`, `commands`, `categories`, `queue_kind`, `enabled`, and `password` are rejected as unknown fields. The server always creates the user `on` with preset `cache-read-write`.

Create username validation is `^[a-z0-9][a-z0-9_-]{2,47}$` (3–48 lowercase). Inspect GET usernames stay 1–64 `[A-Za-z0-9_-]`. `key_pattern` is normalized as redis-ui `NormalizePrefix`: trim `:*` / `:`; reject whitespace, controls, and wildcards `*?[]`; 2–80 characters; `^[a-z0-9][a-z0-9_:-]{0,78}[a-z0-9]$` or `^[a-z0-9]{2}$`; applied as `prefix:*`. Protected names (`default`, `admin`, `redact_admin`, and the configured Redis administrator compared with `EqualFold`) return `403` `protected_resource`. The password is 192-bit URL-safe (`crypto/rand` + `base64.RawURLEncoding`, 24 bytes → 32 characters) and is never client-supplied.

The adapter issues one Redis `ACL LIST` to detect an exact username match (`409` `conflict` because `ACL SETUSER` upserts), then one go-redis `ACLSetUser` with `reset`, `on`, `>password`, one `~prefix:*`, `resetchannels`, `-@all`, and `+CMD` for the `cache-read-write` command set. It does not call `ACL GETUSER`, `ACL USERS`, `ACLGenPass`, or generic `Do`.

Success `201` when `SETUSER` succeeded:

```json
{
  "resource": { "type": "redis_user", "name": "project_a" },
  "user": { "username": "project_a", "enabled": true, "key_pattern": "project_a:*", "preset": "cache-read-write", "protected": false, "rule_fidelity": "exact" },
  "credential": { "username": "project_a", "password": "<one-time>", "one_time": true },
  "request_id": "<32 hex>"
}
```

`credential.urls` is omitted unless both `REDGRES_REDIS_PUBLIC_HOST` and `REDGRES_REDIS_PUBLIC_PORT` are set. Then `urls.primary` is `rediss://` + URL-encoded userinfo + `host:port` + `/0`. The URL never copies administrator userinfo or host. There is no silent port default. Responses are `Cache-Control: no-store`. Errors never include `err.Error()`, passwords, URLs, or hashes.

| Status | Code | When |
|---|---|---|
| 401 | `unauthorized` | Missing session. No `credential`, `user`, or `state`. |
| 403 | `csrf_invalid` | Origin or CSRF failure. |
| 400 | `validation_error` | Unknown field, invalid JSON, or `fields.username` / `fields.key_pattern` (raw illegal values are not echoed). |
| 403 | `protected_resource` | Reserved or configured administrator username. |
| 409 | `conflict` | Exact username already present in `ACL LIST`. |
| 503 | `dependency_unavailable` | Nil adapter, `ErrNotConfigured`, or Redis auth/permission/unreachable. No `reason` key. Typed public message only. |

The action is `redis.user.create`; target is the username; outcome is success or failure; metadata is `username`, `preset=cache-read-write`, and `key_pattern`. Passwords, URLs, and CSRF values are never audited. If `SETUSER` succeeds but the audit insert fails, the handler returns `503` and does not return the credential. If `SETUSER` fails, a failure audit is written and the Redis error is returned.

`PATCH`/`DELETE` enable, disable, rotate, delete, and `GET /api/v1/redis/presets` are not implemented in this slice.

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
