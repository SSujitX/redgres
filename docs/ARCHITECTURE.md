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
- PostgreSQL remains host-native with PgBouncer host-native.
- Redis remains Docker-managed with persistent host volumes.
- pgAdmin and RedisInsight remain optional, isolated tools.

## 3. Modular monolith

```text
HTTP transport
  ├── auth endpoints ─────► auth service ─────► control-state repository (SQLite)
  ├── audit endpoints ────► audit service ────► control-state repository (SQLite)
  ├── postgres endpoints ─► postgres use cases ► PostgreSQL adapter (pgxpool)
  │                              └──────────────► vault adapter (PostgreSQL/Fernet)
  └── redis endpoints ────► redis use cases ───► Redis ACL adapter (go-redis)
```

Dependency direction is inward: transport depends on use cases; use cases depend on interfaces; infrastructure adapters implement interfaces. `postgresadmin` and `redisadmin` do not import each other. Cross-system dashboard aggregation belongs in a platform/status service.

## 4. Backend stack

- Go: currently supported stable release, pinned in `go.mod`/CI.
- Router: `github.com/go-chi/chi/v5`.
- PostgreSQL: `github.com/jackc/pgx/v5` and `pgxpool`.
- Redis: `github.com/redis/go-redis/v9`.
- SQLite: `modernc.org/sqlite` to keep a pure-Go build.
- Passwords: `golang.org/x/crypto/argon2` with encoded, versioned parameters.
- Fernet: a maintained Go implementation validated against Python `cryptography`, or a small audited compatibility package. Choice requires test vectors and dependency review.
- Logging: standard `log/slog`, structured to journald with redaction.

No ORM is planned. PostgreSQL administrative SQL depends heavily on catalog queries, identifier quoting, autocommit-only operations, and explicit privilege semantics; direct audited SQL is clearer.

## 5. Frontend stack

- React 19 + TypeScript + Vite.
- TanStack Query for server state.
- Tailwind CSS for design tokens/utilities.
- Radix UI primitives for accessible dialogs, menus, sheets, and tabs.
- Small local state only; no credential in global stores, URL, localStorage, sessionStorage, IndexedDB, analytics, or error reporting.
- Production build embedded through Go `embed`; Node.js is build-time only.

Feature folders mirror API domains: `overview`, `postgres`, `redis-users`, `audit`, `system`, `auth`, and `docs`.

## 6. Control-plane state

SQLite contains:

- owner users and Argon2id hashes;
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

Long operations (clone, backup verification) create an operation record and run under bounded context/cancellation. A per-target lock prevents overlapping rotate/drop/clone operations.

## 8. Redis adapter

Redgres connects using a dedicated ACL administrator loaded from a root-readable credential file. The browser never receives it. Adapter methods expose narrowly scoped behavior rather than a generic command API.

Preset command sets are versioned in code. Redis version compatibility is tested in integration CI. Externally managed ACL users with unsupported patterns/categories are read-only or require an explicit adoption workflow; Redgres must not rewrite an ACL it cannot faithfully understand.

## 9. Credential lifecycle

### PostgreSQL

Generated credential → create/alter PostgreSQL role → persist encrypted vault entry → return once/no-store. Because the role and vault are separate stores, the use case records operation state and defines compensation:

- Create: if vault write fails, remove only the newly created database/role after dependency checks, or mark credential recovery required.
- Rotate: keep the generated password only in process memory; if vault write fails after role alteration, retry vault persistence and block additional rotation. Report a recoverable incident without logging the password. A future dual-secret/transaction design needs its own ADR.

### Redis

Generated credential → `ACL SETUSER` → return once/no-store. Redgres does not persist the Redis user password. Rotation is irreversible for the old password. If the response is lost, rotate again.

## 10. Configuration

Configuration loads from flags/environment plus root-protected credential files. Production validation fails closed for defaults, missing TLS settings, insecure remote Redis URLs, public UI bindings, weak owner bootstrap, and contradictory feature flags.

Recommended prefixes:

- `REDGRES_*` — service/base URL/state/session/log settings.
- `REDGRES_POSTGRES_*` — administrative DSN components, public host/ports, protected databases.
- `REDGRES_REDIS_*` — administrator URL file, public host/port, supported version policy.
- `REDGRES_FEATURE_*` — destructive action gates.

Secrets should be passed as file paths where practical, never command-line values visible in process listings.

## 11. Failure model

- PostgreSQL unavailable: Redis management and audit remain available; PostgreSQL card reports degraded.
- Redis unavailable: PostgreSQL management and audit remain available; Redis card reports degraded.
- SQLite unavailable: authentication, mutations, and audit-dependent actions fail closed; health reports failure.
- Cloudflare unavailable: loopback services remain healthy but remote browser access is unavailable; raw DB services are independent.
- PgBouncer unavailable: direct PostgreSQL remains separately testable; pooled URL health is degraded.

No component status should collapse independent failures into a single generic “offline.”
