# AGENTS.md — mandatory Redgres context

This file governs all work in this repository. Read it completely before planning or editing. Then read [docs/INDEX.md](docs/INDEX.md) and the documents it marks as required for your task.

Repository-local engineering skills are vendored under `.agents/skills`. Read [.agents/README.md](.agents/README.md) before invoking their tracker-based workflows. Their one-time repository setup is not complete until `setup-matt-pocock-skills` creates the required `docs/agents/` configuration.

## Current truth

Redgres has a compiling Wave 0 foundation, owner auth, a browser login/shell, read-only PostgreSQL inventory (API + Databases page), a table-list API plus inspector list, a bounded row-browse API, inspector row paging/search, authenticated cluster security overview (`GET /api/v1/postgres/security` plus Security overview page; protected non-template names included; `saved_credential` is vault role existence — details `present`/`missing`/`not_available`, cluster `ok` plus `missing_password_count`; `rotation_eligible` derived on cluster GET and shown as Yes/No; Security overview has no Rotate; POST reveal is Databases inspector only, not Security overview; POST create is Databases header/nav only, not Security overview), paginated audit history, Overview component status (Redis Ping on `GET /api/v1/status` plus metrics on `GET /api/v1/redis/status`; PgBouncer `SHOW VERSION` Ping on `GET /api/v1/status`, version discarded; default without `REDGRES_POSTGRES_POOLED_PORT` is `not_configured`; optional pgAdmin/RedisInsight hrefs on `GET /api/v1/session` and Overview, `GET /status` `tool_links` is config presence and is never fetched), authenticated bounded global search (PostgreSQL names + non-protected Redis ACL usernames + client navigation), Redis ACL list/inspect (`GET /api/v1/redis/users` and username detail + ACL users page), create isolated ACL users (`POST /api/v1/redis/users`, always `on`, named preset `cache-read-write` / `read-only` / `queue-worker` or custom allow-list `commands` ⊆ `AllowedCommands()`, one-time credential ticket), `GET /api/v1/redis/presets` (static named-preset catalog, no Redis call), `GET /api/v1/redis/commands` (static `AllowedCommands()` allow-list, no Redis call), named-preset PATCH (`PATCH /api/v1/redis/users/{username}`, prefix/grants only, password preserved), custom allow-list PATCH (`preset: custom` plus `commands` ⊆ `AllowedCommands()`, Edit permissions Custom checklist), inspector enable/disable (`POST /api/v1/redis/users/{username}/enable` and `/disable`, `on`/`off` only), rotate (`POST /api/v1/redis/users/{username}/credentials/rotate`, `resetpass` + `>password` only, one-time ticket), and delete (`DELETE /api/v1/redis/users/{username}`: session + `redis.destructive` + CSRF, exact `username_confirmation`, in-handler owner reauth, `ACL LIST` then one `ACL DELUSER`; inspector Delete danger dialog). Redis Ping, INFO/DBSIZE/Ping-RTT metrics, ACL LIST parse, `ACLSetUser` create (named and custom), on/off, rotate, named-preset PATCH, custom allow-list PATCH, and `ACLDelUser` exist when a URL file is configured; AUTH-006 is Redis DELETE plus flagged PostgreSQL row DELETE plus flagged PostgreSQL truncate plus flagged PostgreSQL drop (no `POST /api/v1/auth/reauth`); the adapter is not a full ACL admin (no CLIENT KILL, keys are not deleted). In-process Fernet/KDF compatibility exists in `internal/secrets` against committed Python `cryptography==49.0.0` fixtures (no TTL). HTTP vault existence SQL is `Catalog.SavedRoleNames` (`role_name` only). Authenticated `GET /api/v1/postgres/databases/{db}/connection` returns vault existence plus omitted-or-present masked direct/pooled URLs (`sslmode=require`; public host/ports only; no decrypt). `POST /api/v1/postgres/databases/{db}/connection/reveal` (session + `postgres.credentials` + CSRF) SELECTs `encrypted_password`, decrypts with `internal/secrets` after Open loads optional `REDGRES_LEGACY_VAULT_SECRET_FILE` (derived Fernet key on the service), returns a no-store credential payload (`one_time: false`), and is audited as `postgres.credential.reveal` with metadata `database` and `owner` only; inspector Reveal opens a PostgreSQL vault-repeatable ticket. `POST /api/v1/postgres/databases` (session + `postgres.provision` + CSRF) creates a restricted login (`CONNECTION LIMIT 20`), encrypts the generated password with `secrets.Encrypt`, INSERTs the vault row, returns 201 no-store (`one_time: false`), and is audited as `postgres.database.create` with metadata `database` and `owner` only; Databases header/nav Create opens the same vault-repeatable ticket; nav/search Create does not open or POST while a ticket is open; list GET 401 clears the ticket. `POST /api/v1/postgres/databases/{db}/credentials/rotate` (session + `postgres.credentials` + CSRF) re-reads PG-012 eligibility, generates a password with `postgresadmin.GeneratePassword()`, encrypts with `secrets.Encrypt`, ALTER ROLE (`CONNECTION LIMIT 20`), upserts the vault row (`ON CONFLICT`), returns 200 no-store (`one_time: false`), and is audited as `postgres.credential.rotate` with metadata `database` and `owner` only; inspector Rotate (typed confirmation) opens the vault-repeatable ticket plus a rotate warning; Security overview has no Rotate. `POST /api/v1/postgres/databases/{db}/duplicate` (session + `postgres.provision` + CSRF) creates a unique restricted login (`CONNECTION LIMIT 20`), terminates source sessions, `CREATE DATABASE … TEMPLATE … OWNER …`, encrypts with `secrets.Encrypt`, INSERTs the vault row (no `ON CONFLICT`), transfers clone-local object ownership without `REASSIGN OWNED` (catalog names quoted without the HTTP allow-list; empty/NUL fail closed; superuser object owners skipped), returns 201 no-store (`one_time: false`), and is audited as `postgres.database.duplicate` with metadata `database`, `owner`, and `source` only; inspector Duplicate (not `--danger`) opens the same vault-repeatable ticket after disclosing connection termination; Security overview has no Duplicate; AUTH-006 does not apply (no 202, no operations row). Authenticated `GET /api/v1/postgres/databases/{db}/tables/{schema}/{table}/primary-key` (session + `postgres.read`, no CSRF, no flag) confirms BASE TABLE then returns `{ primary_key, request_id }` from parameterized `information_schema` PK join (none `[]`; composite returns all names). `DELETE /api/v1/postgres/databases/{db}/tables/{schema}/{table}/rows` (session + `postgres.destructive` + CSRF; `REDGRES_FEATURE_POSTGRES_ROW_DELETE` via `envBool`, unset false) requires exact `table_confirmation` and in-handler owner reauth, discovers a single-column PK server-side, and runs parameterized `DELETE … WHERE pk IN ($1…)`; flag off is 403 `Row delete is turned off.` before JSON decode (no audit, no PostgreSQL); `primary_key_column` is an unknown field; inspector checkboxes appear only for a single-column primary key; danger **Delete selected** opens a typed-confirmation owner-password dialog (`role=dialog`, title **Delete selected rows**); flag-off still shows the control and announces **Row delete is turned off.**; Search / login / Security overview never DELETE rows. `POST /api/v1/postgres/databases/{db}/truncate` (session + `postgres.destructive` + CSRF; `REDGRES_FEATURE_POSTGRES_TRUNCATE` via `envBool`, unset false) lists BASE TABLEs then runs one `TRUNCATE TABLE … RESTART IDENTITY` (no `CASCADE`, no `ONLY`); flag off is 403 `Truncate is turned off.` before JSON decode (no audit, no PostgreSQL); AUTH-006 is in-handler `owner_password`; inspector danger **Truncate** (not rotation-eligible gated) opens a typed-confirmation owner-password dialog (`role=dialog`, title **Truncate project data**); flag-off still shows the control and announces **Truncate is turned off.**; Search / login / Security overview never POST truncate. `DELETE /api/v1/postgres/databases/{db}` (session + `postgres.destructive` + CSRF; `REDGRES_FEATURE_POSTGRES_DROP` via `envBool`, unset false) terminates other backends then runs quoted `DROP DATABASE` (no `WITH (FORCE)`); optional `DROP ROLE` + vault DELETE only when the owner is not protected and owns zero databases; flag off is 403 `Drop is turned off.` before JSON decode (no audit, no PostgreSQL); AUTH-006 is in-handler `owner_password`; backup freshness is not an HTTP gate this Partial (BF-1 disclosure only); inspector danger **Drop** (distinct from Truncate) opens a typed-confirmation owner-password dialog (`role=dialog`, title **Drop database**); flag-off still shows the control and announces **Drop is turned off.**; HTTP 200 refreshes the list and clears inspector selection; Search / login / Security overview never DELETE a database. `internal/secrets` Encrypt is the Fernet inverse of Decrypt (no TTL, no new module). Fail-closed `deploy/install.sh` dispatcher prints a `--dry-run` stage plan and does not mutate the host (OPS-001/006 Partial). Skippable `integration/` live tests and a Playwright login harness exist for disposable CI; they are not COMPATIBILITY.md §6 or production. Target documentation is not evidence that remaining OPS/install/cutover features exist.

Agent turns follow `.cursor/rules/06-continuous-orchestration.mdc`: recover unfinished work, then continue the next dependency-ready PRD slice without asking. `/start-redgres` is the explicit human command for a new chat; the loop does not wait for it.

The two source systems are local sibling repositories:

| Source | Local path | Role |
|---|---|---|
| PostgreSQL console | `D:\code\github\database-app` | Behavioral reference for PostgreSQL operations and the existing credential vault |
| Redis console | `D:\code\github\redis-ui` | Preferred Go/React foundation for Redgres auth, sessions, audit, API shape, and Redis ACL management |

Local paths are development references only. Production Redgres must not import, execute, or depend on files from those directories. The repositories may change independently; record the reviewed commit IDs in [docs/SOURCE_BASELINE.md](docs/SOURCE_BASELINE.md) before implementation begins.

## Required reading by task

- Any task: `README.md`, this file, `docs/INDEX.md`, `docs/PROJECT_CHARTER.md`.
- Product or UI: `.agents/skills/redgres-ui-design/SKILL.md`, `docs/PRD.md`, `docs/UX.md`, `docs/UI_DESIGN_SYSTEM.md`, `docs/DOMAIN_AND_NETWORK.md`, `docs/API.md`.
- Backend: `docs/ARCHITECTURE.md`, `docs/COMPATIBILITY.md`, `docs/SOURCE_SYSTEMS.md`, `docs/DATA_AND_SECRETS.md`, `docs/SECURITY.md`.
- Deployment/operations: `docs/COMPATIBILITY.md`, `docs/DEPLOYMENT.md`, `docs/INSTALLER_SPEC.md`, `docs/POSTGRESQL_PROVISIONING.md`, `docs/BACKUP_RECOVERY.md`, `docs/OPERATIONS.md`.
- Migration/cutover: `docs/MIGRATION.md`, `docs/TESTING.md`, every ADR in `docs/decisions/`.

## Non-negotiable invariants

1. Never expose the Redgres, pgAdmin, RedisInsight, or legacy UI origin ports publicly. They bind to loopback and are reached through Cloudflare Tunnel + Access.
2. Cloudflare Tunnel does not proxy ordinary PostgreSQL or Redis clients in this design. Raw database endpoints use DNS-only records and end-to-end TLS.
3. Never log, audit, cache, persist in browser storage, or return in a later GET: passwords, connection URLs containing passwords, session tokens, CSRF tokens, tunnel tokens, DNS API tokens, or private keys.
4. Every credential-bearing response must set `Cache-Control: no-store`; UI state must clear it on dismiss/navigation/selection change.
5. Preserve the existing PostgreSQL Fernet vault until byte-for-byte Go compatibility is proven against fixture and copied production records. Losing or changing the legacy `SESSION_SECRET` destroys the ability to decrypt existing credentials.
6. Destructive PostgreSQL actions are disabled by default and must protect `postgres`, `template0`, `template1`, `database_console_vault`, the configured Redgres state/admin databases, and a configurable deny-list.
7. Redis custom permissions use an explicit tested allow-list. A deny-list is insufficient. Never offer arbitrary Redis command execution.
8. Application rollback may switch release binaries/configuration only. It must never automatically reverse database schema migrations, credential rotations, Redis state, PostgreSQL data, or backups.
9. The installer must be idempotent and must never overwrite, reinitialize, or remove an existing PostgreSQL cluster without explicit fresh-install mode and validated preconditions.
10. A backup is not accepted until checksums and a restore test exist. Same-server copies are not disaster recovery.
11. Service support is the release-owned matrix in `docs/COMPATIBILITY.md`. Never use floating `latest` service artifacts, widen support through configuration, or perform an implicit PostgreSQL major/Redis series upgrade.
12. PostgreSQL server adoption, host extension packages, per-database `CREATE EXTENSION`, preload/restart changes, and PgBouncer are separate lifecycles. Existing mode defaults to preserve; never install/enable all extensions, touch `template1`, or restart PostgreSQL without an explicit reviewed plan and approval.

## Engineering rules

- Stay within the assigned PRD slice/non-goals. Do not add speculative features, abstractions, dependencies, or refactors unrelated to its acceptance criteria.
- Never invent APIs, symbols, flags, configuration, version behavior, source parity, deployment facts, or test results. Verify locally first, then against exact pinned-version source/docs or official primary sources; record material external evidence and versions.
- For new dependencies and planned refreshes, choose the newest stable security-supported mutually compatible release, then pin it exactly. Never interpret “latest” as a floating range/tag, prerelease, or permission for an unreviewed major upgrade.
- Inspect available repository skills and use the smallest relevant set. Skills never override accepted requirements, safety boundaries, or authorization.
- Prefer the Go/React source structure from `redis-ui`; port PostgreSQL behavior behind a `postgresadmin` package. Do not mechanically translate Python line by line.
- Use direct, parameterized SQL through `pgx`; do not add an ORM unless an ADR replaces the current decision.
- Version Redgres endpoints under `/api/v1`. Preserve legacy behavior through explicit adapters or a documented migration, not accidental compatibility.
- All state-changing use cases must produce a secret-safe audit event with actor, action, target, outcome, request ID, client IP, and redacted metadata.
- Validate identifiers at both transport and domain boundaries; quote PostgreSQL identifiers with library primitives. Values always use parameters.
- Use typed errors mapped to stable API error codes. Do not return raw database/server errors to browsers.
- New behavior requires unit tests. PostgreSQL/Redis behavior requires the applicable jobs from the complete matrix in `docs/COMPATIBILITY.md`; evidence records detected full versions and immutable artifacts.
- Never commit runtime SQLite files, `.env`, certificates, token files, binaries, backups, or generated credentials.
- Update documentation and the traceability matrix in the same change as behavior.
- UI work must use the shared design tokens/shell, prove the responsive states in `docs/UI_DESIGN_SYSTEM.md`, and receive independent `redgres-ui-reviewer` evidence before merge.
- Production code must be explicit, typed, bounded, secure by default, and deliberate about errors/cancellation/dependency failure. No placeholder success paths, silent fallback, random copied code, or completion claim based on reputation-language instead of evidence.

## Documentation ownership

Update only the canonical owner for the change; link instead of duplicating policy. Every implementation change updates `docs/TRACEABILITY.md` with exact files and executed evidence.

| Change | Canonical documentation |
|---|---|
| Product behavior or acceptance criteria | `docs/PRD.md`, then `docs/TRACEABILITY.md` |
| HTTP request/response/error behavior | `docs/API.md` |
| Go/module/data-flow boundary | `docs/ARCHITECTURE.md`; add/supersede an ADR for a durable decision |
| Service version/default/support | `docs/COMPATIBILITY.md` and ADR-008 |
| UI information architecture/workflow | `docs/UX.md` |
| UI tokens, shell, responsive/accessibility behavior | `docs/UI_DESIGN_SYSTEM.md` |
| Configuration key/default/validation | `docs/CONFIGURATION.md` |
| Installer/runtime/network/domain behavior | `docs/INSTALLER_SPEC.md`, `docs/DEPLOYMENT.md`, or `docs/DOMAIN_AND_NETWORK.md` as applicable |
| PostgreSQL existing/fresh lifecycle, extensions, preload/restart, PgBouncer | `docs/POSTGRESQL_PROVISIONING.md` and ADR-009 |
| Backup/restore/runbook behavior | `docs/BACKUP_RECOVERY.md` or `docs/OPERATIONS.md` |
| Security/secret boundary | `docs/SECURITY.md`, `docs/DATA_AND_SECRETS.md`, and an ADR when architectural |
| Legacy parity/provenance discovery | `docs/SOURCE_SYSTEMS.md` or `docs/SOURCE_BASELINE.md` |
| Test/release gate | `docs/TESTING.md` and `docs/ACCEPTANCE_CHECKLIST.md` |
| Milestone order/scope | `docs/ROADMAP.md` or `docs/MIGRATION.md` |

Do not edit a document merely to create noise. If behavior and contracts did not change, record test evidence only in traceability. If implementation reveals that a target document is wrong, correct the canonical document and requirement/ADR in the same change before claiming completion.

## Change protocol

Before coding:

1. Identify the PRD requirement and acceptance criteria.
2. Identify affected invariants and ADRs.
3. Inspect the corresponding source-system implementation and tests.
4. State migration/rollback impact, including data and secret compatibility.

Before declaring complete:

1. Run the required checks in `docs/TESTING.md`.
2. Confirm no secrets or runtime artifacts are in the diff.
3. Update `docs/TRACEABILITY.md` with implementation and test evidence.
4. State what was not tested and why.

If documentation conflicts, security invariants and accepted ADRs take precedence, followed by the PRD, architecture, deployment documents, and roadmap. Resolve the contradiction in the same change; do not silently choose one.
