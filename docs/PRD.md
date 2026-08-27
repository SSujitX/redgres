# Product requirements document

Status: Target specification
Audience: product, engineering, security, operations, testing
Product: Redgres Console

## 1. Problem

PostgreSQL and Redis administration currently live in two separate applications with different stacks, authentication models, interfaces, deployment methods, and operational documentation. Operators need one coherent console and one deployment/operations model without losing working behavior or weakening credential protection.

## 2. Goal

Build a self-hosted, open-source control plane that safely manages one PostgreSQL cluster and one Redis instance from a single browser application and can be installed, verified, backed up, updated, and rolled back on one Ubuntu server.

## 3. Personas and critical journeys

### Owner/operator

- Signs in once and sees PostgreSQL, Redis, platform health, and recent audit activity.
- Creates a PostgreSQL project database and restricted login, then copies direct and pooled URLs.
- Creates a Redis project user with a safe preset and isolated key prefix, then copies a one-time URL.
- Rotates, disables, or deletes credentials with clear impact and confirmation.
- Diagnoses connectivity without exposing administrator secrets.
- Verifies backup freshness and follows an exact restore runbook.

### Application developer

- Receives only the project-specific URL needed by the application.
- Can choose direct PostgreSQL (`5432`) or pooled PostgreSQL (`6432`) correctly.
- Uses Redis over TLS (`6380`) with a scoped ACL user and prefix.
- Never receives Redgres, PostgreSQL admin, Redis admin, Cloudflare, or backup credentials.

## 4. Functional requirements

Requirement IDs are stable. Implementation and test evidence belongs in [TRACEABILITY.md](TRACEABILITY.md).

### Authentication and platform

| ID | Requirement | Acceptance criteria |
|---|---|---|
| AUTH-001 | Owner bootstrap is CLI-only. | No browser bootstrap route exists. CLI refuses an empty/weak password and refuses accidental overwrite of an existing owner. |
| AUTH-002 | Passwords use Argon2id. | Parameters are versioned; verification uses constant-time comparison behavior supplied by the library; hashes, never passwords, are stored. |
| AUTH-003 | Sessions are server-side and opaque. | Browser cookie contains only a random token; SQLite contains only its hash; cookie is `HttpOnly`, `Secure` in production, `SameSite=Strict`, path `/`, and has idle and absolute expiry. |
| AUTH-004 | Mutations require CSRF and same-origin checks. | Missing/invalid CSRF or origin returns a stable 403 error; login is also origin-checked. |
| AUTH-005 | Login attempts are rate-limited. | Repeated failures per normalized username and client IP produce 429 with `Retry-After`; successful login clears failures. |
| AUTH-006 | Sensitive/destructive actions can require reauthentication. | PostgreSQL drop/truncate/row delete and Redis delete require typed target confirmation and fresh owner-password verification. |
| PLAT-001 | Dashboard reports component health. | Redgres state DB, PostgreSQL direct admin path, PgBouncer, Redis, and optional tool links are represented independently; partial failure is visible. |
| PLAT-002 | Every mutation writes an audit event. | Actor, action, target, outcome, request ID, client IP, redacted metadata, and time are recorded; no secret detector finds credential material. |
| PLAT-003 | Operators can inspect paginated audit history. | Cursor pagination is stable and failure events are visible without raw internal errors. |
| PLAT-004 | Operators can search and navigate globally. | Authenticated bounded search groups PostgreSQL databases, Redis ACL users, navigation, and documentation with explicit service/type context; it works by keyboard and mobile sheet, excludes credentials/protected data, and never executes destructive actions directly. |

### PostgreSQL

| ID | Requirement | Acceptance criteria |
|---|---|---|
| PG-001 | List manageable project databases. | Templates, `postgres`, vault/state/admin databases, and configured protected databases are excluded from ordinary management lists. |
| PG-002 | Show database details. | Owner, size, collation/ctype, connection count, security status, and saved-credential status are available without revealing passwords. |
| PG-003 | Create a project database and restricted login. | Identifiers are validated/quoted; role is `LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION`, has a configured connection limit, database ownership, and no PUBLIC CONNECT. Failure compensation removes only resources created by the operation. |
| PG-004 | Return direct and pooled URLs. | New credentials yield `postgresql://` URLs for the configured host on 5432 and 6432 with URL-encoded credentials and TLS parameters; credential-bearing response is `no-store`. |
| PG-005 | Preserve and reveal saved project passwords. | Existing Fernet records decrypt in Go; GET returns only masked metadata; explicit POST reveal returns `no-store` and is audited without the secret. |
| PG-006 | Rotate a project-owner password. | Protected/admin/superuser/non-login roles are rejected; new password is cryptographically random; PostgreSQL and vault state use a recoverable state machine with failure reporting; applications are warned that old credentials stop working. |
| PG-007 | List schemas/tables and page through rows. | Only validated databases/schemas/tables are queried; pagination and bounded search avoid unbounded reads; values are encoded safely. |
| PG-008 | Delete selected rows safely. | Feature is off by default; target must be a manageable project database; server discovers and validates the real primary key; request is size-bounded and reauthenticated; result is audited. |
| PG-009 | Truncate project data safely. | Feature is off by default; explicit confirmation and reauthentication are required; all target tables are determined before mutation; partial failures are reported precisely. |
| PG-010 | Duplicate a database safely. | New database and unique restricted owner are required; source connections/impact are disclosed; source ownership and ACL fingerprint are checked before/after; no `REASSIGN OWNED` against shared roles; cleanup touches only created targets. |
| PG-011 | Drop a project database safely. | Protected databases cannot be dropped in any mode; feature is off by default; exact name + owner reauthentication required; target sessions exclude the current backend; optional role removal occurs only if ownership/dependencies prove safe. |
| PG-012 | Expose security overview. | Public CONNECT, owner privilege risks, missing vault entries, active connections, and rotation eligibility are shown without role hashes or passwords. |

### Redis

| ID | Requirement | Acceptance criteria |
|---|---|---|
| REDIS-001 | Show Redis status. | Connectivity, version, uptime, clients, used/max memory, ops/s, DB size, and latency are shown; auth/permission/connectivity failures are distinct and secret-safe. |
| REDIS-002 | List and inspect ACL users. | ACL rules are parsed into status, prefix, explicit commands, inferred preset, and protected status; unreadable category-only rules are labeled custom/limited rather than misrepresented. |
| REDIS-003 | Create an isolated ACL user. | Username and prefix are validated; protected names rejected; password is random; `reset`, on/off, password, one key pattern, `resetchannels`, `-@all`, and explicit allowed commands are applied. |
| REDIS-004 | Support safe presets. | `cache-read-write`, `read-only`, and queue-worker variants for Lists, Streams, and Sorted Sets have versioned explicit command lists and integration tests using representative workloads. |
| REDIS-005 | Support custom permissions safely. | Every command/category expands through a versioned allow-list; administrative, scripting/module, config, shutdown, flush, ACL, dangerous keyspace, and future unknown commands fail closed. |
| REDIS-006 | Update permissions without rotating password. | Key scope and command grants change; existing password remains valid; protected users cannot be modified. |
| REDIS-007 | Enable/disable and rotate. | State changes preserve permissions; rotate invalidates the previous password immediately; credential response is one-time and `no-store`. |
| REDIS-008 | Delete with guardrails. | Protected users cannot be deleted; exact username and owner reauthentication required; success/failure audited. |

### Operations

| ID | Requirement | Acceptance criteria |
|---|---|---|
| OPS-001 | Install on a fresh server. | Fresh mode validates a clean target, accepts only PostgreSQL/Redis selections supported by the current Redgres release, resolves exact pinned artifacts, creates users/directories, configures services, records versions/digests, and passes verification. |
| OPS-002 | Adopt existing database services. | Existing mode auto-detects PostgreSQL, Redis, and PgBouncer versions/capabilities, checks optional expected-version assertions, inventories and backs up first, preserves cluster/data/config unless an explicit step is approved, and passes non-destructive compatibility checks. |
| OPS-003 | Verify the complete platform. | Command checks services, bindings, DNS, certificates, TLS rejection/acceptance, auth boundaries, HTTP health, tunnel routes, and backup prerequisites without printing secrets. |
| OPS-004 | Produce recoverable backups. | PostgreSQL logical backup, atomic Redis persistence snapshot/AOF/ACL capture, consistent SQLite backup, configuration manifest, checksums, retention, and encrypted off-host copy are defined. |
| OPS-005 | Update and roll back application releases. | Releases are immutable; `current` symlink switch is atomic; health gate runs; rollback never reverses data/schema/credential changes automatically. |
| OPS-006 | Enforce a release-owned service compatibility policy. | The UI/installer exposes the supported choices and recommendations from [COMPATIBILITY.md](COMPATIBILITY.md); unsupported, prerelease, unparseable, or mismatched service versions fail before mutation; configuration cannot widen support; service major/series upgrades are never implicit. |
| OPS-007 | Adopt or provision PostgreSQL capabilities safely. | PostgreSQL and PgBouncer have independent existing/fresh modes; existing mode defaults to inventory-and-preserve; optional extension packages/preloads/per-database enablement use an explicit release-supported plan; unrequested databases and `template1` remain unchanged; required restart and schema mutation are previewed, backed up, approved, verified, and reported as specified in [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md). |
| OPS-008 | Provide a self-closing first-run bootstrap for the console. | After install, the Redgres UI is reachable at a temporary `8989` listener source-restricted to the operator's IP; it auto-rebinds to loopback and removes the firewall rule once Tunnel + Access are verified; pgAdmin and RedisInsight never receive a public listener (ADR-012). |
| OPS-009 | Connect a domain from the UI (Domain & Network wizard). | The owner enters hostnames and authorizes Cloudflare (minimal-scope OAuth app or per-zone token); the server creates the tunnel, proxied/DNS-only records, Let's Encrypt DB certificates (DNS-01, auto-renew), and the Access policy, verifies them live, and reports changes without guessing zones or touching other zones; non-Cloudflare domains get manual-record instructions. |

## 5. Non-functional requirements

| ID | Requirement | Target |
|---|---|---|
| NFR-001 | Availability | Single-node best effort; service restarts automatically. No false HA claim. |
| NFR-002 | Performance | UI API p95 under 500 ms for local control operations excluding long PostgreSQL clone/backup operations; list endpoints bounded and paginated. |
| NFR-003 | Capacity | One operator-oriented instance, up to 500 PostgreSQL project databases/roles and 5,000 Redis ACL users; validate with synthetic tests before claiming. |
| NFR-004 | Accessibility | WCAG 2.2 AA target for core workflows; complete keyboard operation, visible focus, labels, error announcements, reduced motion. |
| NFR-005 | Browser support | Current and previous major Chromium, Firefox, and Safari. |
| NFR-006 | Observability | Structured logs to journald, request IDs, redaction, service health, bounded audit retention policy. |
| NFR-007 | Supply chain | Direct application/build dependencies start from the latest stable security-supported compatible releases, are exactly pinned in reproducible manifests/lockfiles, receive reviewed automated update proposals, and pass complete tests, SBOM/license checks, vulnerability scanning, and rollback/migration review before release. Floating `latest`, prereleases, and unreviewed automatic major upgrades are forbidden. |
| NFR-008 | Portability | Linux amd64/arm64 application builds; primary supported deployment is Ubuntu 24.04 LTS on one server. |
| NFR-009 | Maintainability | Modular monolith with dependency boundaries; no package may reach PostgreSQL/Redis except its adapter. |
| NFR-010 | Data durability | SQLite WAL mode plus consistent backups; PostgreSQL/Redis durability follows documented service configuration and tested recovery objectives. |
| NFR-011 | Service compatibility | Every claimed PostgreSQL/Redis combination has reproducible integration, install/adoption, backup, and restore evidence tied to exact artifacts. |
| NFR-012 | Responsive interface | Every core owner workflow functions from 320 CSS px through wide desktop and at 200% zoom without viewport-level horizontal scrolling; sidebar, icon-rail, drawer, search, tables, dialogs, and login follow [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md). |

## 6. UX requirements

- The authenticated shell uses a responsive left sidebar/icon rail/mobile drawer, sticky topbar search, and top-right owner menu as specified in [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md).
- Navigation: Overview, PostgreSQL, Redis ACL, Audit, System, Documentation. PostgreSQL and Redis use distinct service identity while sharing interaction patterns.
- The unauthenticated login route is responsive, reveals no dependency state, and never renders the authenticated shell.
- Credentials use a modal/ticket with copy actions, explicit “shown now” wording, no auto-copy, no browser persistence, and forced clearing.
- Dangerous actions use a dedicated danger surface, state exact blast radius, require typed target, and require reauthentication where specified.
- Long operations return an operation ID and progress state; HTTP requests must not remain open indefinitely.
- Status colors are never the only signal.
- Feature work reuses shared design tokens/shell primitives and passes the documented responsive, keyboard, zoom, and visual review gate.

## 7. Release gates

The migration release cannot replace legacy consoles until all are true:

1. Source baselines are pinned and parity matrix is complete.
2. Critical source defects listed in [SOURCE_SYSTEMS.md](SOURCE_SYSTEMS.md) are fixed or intentionally superseded with tests.
3. Vault compatibility passes fixture and copied-record tests without modifying source records.
4. The complete matrix in [COMPATIBILITY.md](COMPATIBILITY.md), TLS, permission, destructive-action, backup, and restore suites pass using exact recorded artifacts.
5. Existing-server install rehearsal succeeds on a clone/staging host.
6. Cloudflare Access, loopback bindings, DNS-only raw endpoints, certificates, and UFW rules are observed on the live host.
7. Legacy fallback remains available through the observation window.
8. Operator signs off on backup restore and rollback rehearsal.
