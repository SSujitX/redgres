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
  ├── postgres endpoints ─► postgres use cases ► PostgreSQL adapter (pgxpool)
  │                              └──────────────► vault adapter (PostgreSQL/Fernet)
  └── redis endpoints ────► redis use cases ───► Redis ACL adapter (go-redis)
```

Dependency direction is inward: transport depends on use cases; use cases depend on interfaces; infrastructure adapters implement interfaces. `postgresadmin` and `redisadmin` do not import each other. Cross-system dashboard aggregation belongs in a platform/status service.

## 4. Backend stack

- Go: `go 1.27.0` in `go.mod` (installed/local and CI via `go-version-file`). Official [Go 1.27 release notes](https://go.dev/doc/go1.27) (2026-08) keep the Go 1 compatibility promise. Wave 0 originally considered `go 1.26.7` as the previous-line newest patch; the operator installed 1.27 and Wave 0 builds/tests passed against it with `modernc.org/sqlite` v1.57.0 and `chi` v5.3.2.
- Router: `github.com/go-chi/chi/v5` `v5.3.2`.
- PostgreSQL: `github.com/jackc/pgx/v5` and `pgxpool` (not in Wave 0).
- Redis: `github.com/redis/go-redis/v9` (not in Wave 0).
- SQLite: `modernc.org/sqlite` `v1.57.0`.
- Passwords: `golang.org/x/crypto` `v0.55.0` (`argon2.IDKey`, version `0x13`). Interactive owner bootstrap uses `golang.org/x/term` `v0.45.0`. Both are official `go.googlesource.com` modules; `openpgp` is not imported.
- Fernet: a maintained Go implementation validated against Python `cryptography`, or a small audited compatibility package. Choice requires test vectors and dependency review.
- Logging: standard `log/slog`, structured to journald with redaction.

No ORM is planned. PostgreSQL administrative SQL depends heavily on catalog queries, identifier quoting, autocommit-only operations, and explicit privilege semantics; direct audited SQL is clearer.

Service-version support is governed by [COMPATIBILITY.md](COMPATIBILITY.md) and [ADR-008](decisions/ADR-008-service-version-policy.md). Redgres detects the connected PostgreSQL, Redis, and PgBouncer versions and required capabilities before enabling administrative mutations. The supported matrix is release-owned and cannot be widened by runtime configuration.

PostgreSQL server adoption/install, extension host packages, per-database extension state, preload/restart configuration and PgBouncer service lifecycle are separate deployment concerns governed by [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md) and [ADR-009](decisions/ADR-009-postgres-adoption-and-extensions.md). The browser application does not become an arbitrary package/extension manager; approved changes run through the versioned installer/CLI and direct PostgreSQL connection.

## 5. Frontend stack

- Wave 0 frontend pins (locked in `web/package-lock.json`): React `19.2.8`, Vite `8.2.2`, `@vitejs/plugin-react` `6.1.0`, Vitest `4.1.11`, TypeScript `7.0.2`. Node build tool is Active LTS **24.19.0** (`web/.nvmrc`). Local Node 25.x is unsupported and is not release evidence.
- Vite writes to `internal/web/dist/app` so `//go:embed all:dist` still compiles from the tracked `dist/.gitkeep`. `build.modulePreload.polyfill` is `false` so the HTML has no inline script under `script-src 'self'`.
- TanStack Query, Tailwind CSS, and Radix remain target-only until the Wave 1 frontend slice; the parent owns `web/package.json` / lockfile.
- Shared application shell and semantic tokens follow [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md); feature folders do not define independent navigation, palettes, or breakpoints.
- Small local state only; no credential in global stores, URL, localStorage, sessionStorage, IndexedDB, analytics, or error reporting.
- Production build embedded through Go `embed`; Node.js is build-time only.

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

Long operations (clone, backup verification) create an operation record and run under bounded context/cancellation. A per-target lock prevents overlapping rotate/drop/clone operations.

## 8. Redis adapter

Redgres connects using a dedicated ACL administrator loaded from a root-readable credential file. The browser never receives it. Adapter methods expose narrowly scoped behavior rather than a generic command API.

Preset command sets are versioned in code and tested against every supported Redis series. Externally managed ACL users with unsupported patterns/categories are read-only or require an explicit adoption workflow; Redgres must not rewrite an ACL it cannot faithfully understand.

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
