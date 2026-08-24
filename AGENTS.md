# AGENTS.md — mandatory Redgres context

This file governs all work in this repository. Read it completely before planning or editing. Then read [docs/INDEX.md](docs/INDEX.md) and the documents it marks as required for your task.

Repository-local engineering skills are vendored under `.agents/skills`. Read [.agents/README.md](.agents/README.md) before invoking their tracker-based workflows. Their one-time repository setup is not complete until `setup-matt-pocock-skills` creates the required `docs/agents/` configuration.

## Current truth

Redgres has a compiling Wave 0 foundation, owner auth, a browser login/shell, read-only PostgreSQL inventory (API + Databases page), a table-list API plus inspector list, a bounded row-browse API, inspector row paging/search, paginated audit history, Overview component status (Redis Ping on `GET /api/v1/status` plus metrics on `GET /api/v1/redis/status`), authenticated bounded global search (PostgreSQL names + non-protected Redis ACL usernames + client navigation), Redis ACL list/inspect (`GET /api/v1/redis/users` and username detail + ACL users page), and create-only isolated ACL users (`POST /api/v1/redis/users`, always `on` + `cache-read-write`, one-time credential ticket). Redis Ping, INFO/DBSIZE/Ping-RTT metrics, ACL LIST parse, and one `ACLSetUser` create exist when a URL file is configured; the adapter is not a full ACL admin (no enable/disable, rotate, delete, or other presets). No vault decrypt or installer is implemented. Target documentation is not evidence that those features exist.

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
