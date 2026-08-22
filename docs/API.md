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
| GET | `/api/v1/healthz` | Liveness/state DB health, no auth secrets |
| GET | `/api/v1/status` | Authenticated component status |
| GET | `/api/v1/search?q=&limit=` | Authenticated bounded search over manageable resource metadata/navigation; no secrets or destructive execution |
| GET | `/api/v1/audit?cursor=&limit=` | Paginated redacted audit history |
| GET | `/api/v1/operations/{id}` | Long-operation state/result summary |

## Static asset delivery

The Go binary serves the embedded Vite build for non-API `GET` requests:

- `index.html` is served with `Cache-Control: no-store` and is the SPA fallback for unknown non-API paths.
- Hashed `/assets/*` files are served with `Cache-Control: public, max-age=31536000, immutable`.
- If embedded assets are absent (clean checkout before `npm run build`), non-API `GET` returns `503` `dependency_unavailable`. There is no directory listing and no empty 200.

Search requires a normalized minimum query length, a strict maximum length/limit, request cancellation/timeouts, and stable grouped result types. It returns only fields already safe for authenticated inventory views, excludes protected/hidden targets and credential material, rate-limits abusive use, and never accepts an action/command to execute. Documentation/navigation entries may be client-side; server resource results still enforce the same manageability policy as their source list endpoints.

## PostgreSQL endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/postgres/databases` | Manageable databases only |
| POST | `/api/v1/postgres/databases` | Create database/role; one-time credentials; `no-store` |
| GET | `/api/v1/postgres/databases/{db}` | Details/security metadata |
| GET | `/api/v1/postgres/databases/{db}/connection` | Masked URLs and saved status only |
| POST | `/api/v1/postgres/databases/{db}/connection/reveal` | Full saved URLs; `no-store`; optionally fresh reauth by policy |
| POST | `/api/v1/postgres/databases/{db}/credentials/rotate` | Rotate; typed confirmation; `no-store` |
| POST | `/api/v1/postgres/databases/{db}/duplicate` | Starts bounded operation; may return 202 + operation ID |
| DELETE | `/api/v1/postgres/databases/{db}` | Exact confirmation + owner password |
| GET | `/api/v1/postgres/databases/{db}/tables` | Schemas/tables |
| GET | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | Bounded rows/search |
| DELETE | `/api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` | PK values + confirmations/reauth |
| POST | `/api/v1/postgres/databases/{db}/truncate` | Explicit target confirmation + reauth |
| GET | `/api/v1/postgres/security` | Cluster/project security overview |

Database/role names in URL segments are decoded then validated. Transport validation never replaces PostgreSQL identifier quoting.

## Redis endpoints

| Method | Path | Notes |
|---|---|---|
| GET | `/api/v1/redis/status` | Health/performance summary |
| GET | `/api/v1/redis/users` | Managed and visible ACL users |
| GET | `/api/v1/redis/users/{username}` | User/preset/rules |
| POST | `/api/v1/redis/users` | Create; one-time credential; `no-store` |
| PATCH | `/api/v1/redis/users/{username}` | Permissions/prefix only |
| POST | `/api/v1/redis/users/{username}/enable` | Enable |
| POST | `/api/v1/redis/users/{username}/disable` | Disable |
| POST | `/api/v1/redis/users/{username}/credentials/rotate` | One-time credential; `no-store` |
| DELETE | `/api/v1/redis/users/{username}` | Exact confirmation + owner password |
| GET | `/api/v1/redis/presets` | Versioned available presets/commands for UI/docs |

There is no generic Redis command endpoint.

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
