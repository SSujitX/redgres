# AGENTS.md — mandatory Redgres context

This file governs all work in this repository. Read it completely before planning or editing. Then read [docs/INDEX.md](docs/INDEX.md) and the documents it marks as required for your task.

Repository-local engineering skills are vendored under `.agents/skills`. Read [.agents/README.md](.agents/README.md) before invoking their tracker-based workflows. Their one-time repository setup is not complete until `setup-matt-pocock-skills` creates the required `docs/agents/` configuration.

## Current truth

Redgres is in specification/pre-implementation status. Documentation describes the target and migration gates; it does not prove that a feature is implemented, deployed, or tested.

The two source systems are local sibling repositories:

| Source | Local path | Role |
|---|---|---|
| PostgreSQL console | `D:\code\github\database-app` | Behavioral reference for PostgreSQL operations and the existing credential vault |
| Redis console | `D:\code\github\redis-ui` | Preferred Go/React foundation for Redgres auth, sessions, audit, API shape, and Redis ACL management |

Local paths are development references only. Production Redgres must not import, execute, or depend on files from those directories. The repositories may change independently; record the reviewed commit IDs in [docs/SOURCE_BASELINE.md](docs/SOURCE_BASELINE.md) before implementation begins.

## Required reading by task

- Any task: `README.md`, this file, `docs/INDEX.md`, `docs/PROJECT_CHARTER.md`.
- Product or UI: `docs/PRD.md`, `docs/DOMAIN_AND_NETWORK.md`, `docs/API.md`.
- Backend: `docs/ARCHITECTURE.md`, `docs/SOURCE_SYSTEMS.md`, `docs/DATA_AND_SECRETS.md`, `docs/SECURITY.md`.
- Deployment/operations: `docs/DEPLOYMENT.md`, `docs/INSTALLER_SPEC.md`, `docs/BACKUP_RECOVERY.md`, `docs/OPERATIONS.md`.
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

## Engineering rules

- Prefer the Go/React source structure from `redis-ui`; port PostgreSQL behavior behind a `postgresadmin` package. Do not mechanically translate Python line by line.
- Use direct, parameterized SQL through `pgx`; do not add an ORM unless an ADR replaces the current decision.
- Version Redgres endpoints under `/api/v1`. Preserve legacy behavior through explicit adapters or a documented migration, not accidental compatibility.
- All state-changing use cases must produce a secret-safe audit event with actor, action, target, outcome, request ID, client IP, and redacted metadata.
- Validate identifiers at both transport and domain boundaries; quote PostgreSQL identifiers with library primitives. Values always use parameters.
- Use typed errors mapped to stable API error codes. Do not return raw database/server errors to browsers.
- New behavior requires unit tests. Database/Redis behavior requires integration tests against PostgreSQL 17 and Redis 8.
- Never commit runtime SQLite files, `.env`, certificates, token files, binaries, backups, or generated credentials.
- Update documentation and the traceability matrix in the same change as behavior.

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
