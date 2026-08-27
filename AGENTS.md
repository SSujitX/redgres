# AGENTS.md — Redgres operating contract

This file is always loaded. Keep it compact; detailed truth belongs in routed canonical documents.

## Bootstrap

Before planning or editing:

1. Read this file and `CONTEXT.md`. Read `docs/PROJECT_CHARTER.md` for product scope/acceptance decisions; use `docs/INDEX.md` only when the routing table does not cover the task.
2. Inspect Git status/diff and preserve unrelated or user-owned changes.
3. Read the applicable row and latest slice in `docs/TRACEABILITY.md`; target prose is never implementation evidence.
4. Identify the governing PRD IDs, acceptance criteria, invariants, and ADRs.
5. Load only the task-relevant documents below. For large documents, locate the relevant heading or endpoint first; read the whole file only for a genuinely cross-cutting change.

`README.md` is human setup/run guidance, not mandatory agent context. Historical slice ledgers are Git history referenced by `docs/TRACEABILITY.md`, not always-loaded prose.

Before any tracker-based vendored skill, read `.agents/README.md`; its setup is incomplete until the required `docs/agents/` configuration exists.

## Current boundary

Redgres is a self-hosted control plane for one PostgreSQL cluster and one Redis instance. Version 1 has one owner and is neither a public DBaaS nor a replacement for pgAdmin/RedisInsight.

The current implementation matrix is `docs/TRACEABILITY.md`. The codebase has substantial authenticated PostgreSQL/Redis administration and UI behavior, but every requirement group remains Partial; installer, recovery, complete compatibility evidence, staging, Cloudflare/DNS/TLS/firewall, canary observation, and production sign-off are not accepted. Documentation describing a target does not make it implemented.

Read-only behavioral references:

| System | Path |
|---|---|
| PostgreSQL console | `D:\\code\\github\\database-app` |
| Redis console | `D:\\code\\github\\redis-ui` |

Never edit those repositories or make Redgres depend on them. Pin reviewed source commits in `docs/SOURCE_BASELINE.md`.

## Context routing

| Change | Read |
|---|---|
| Product behavior/acceptance | relevant `docs/PRD.md` requirement; `docs/TRACEABILITY.md` |
| HTTP endpoint | relevant `docs/API.md` endpoint; `docs/ARCHITECTURE.md`; security docs when sensitive |
| Go/backend/data flow | relevant `docs/ARCHITECTURE.md`; `docs/COMPATIBILITY.md`; `docs/SOURCE_SYSTEMS.md` only for parity |
| Secrets/auth/destructive actions | `docs/SECURITY.md`; `docs/DATA_AND_SECRETS.md`; governing ADR |
| React/UI | `.agents/skills/redgres-ui-design/SKILL.md`; relevant `docs/UX.md`, `docs/UI_DESIGN_SYSTEM.md`, and API endpoint |
| Deployment/installer | relevant `docs/INSTALLER_SPEC.md`, `docs/DEPLOYMENT.md`, `docs/CONFIGURATION.md`, `docs/COMPATIBILITY.md` |
| PostgreSQL lifecycle/extensions | `docs/POSTGRESQL_PROVISIONING.md`; ADR-009 |
| Backup/recovery | `docs/BACKUP_RECOVERY.md`; `docs/OPERATIONS.md`; ADR-011 and ADR-005 as applicable |
| Migration/cutover | `docs/MIGRATION.md`; `docs/TESTING.md`; affected ADRs only |
| Test/release evidence | relevant `docs/TESTING.md`; `docs/ACCEPTANCE_CHECKLIST.md`; `docs/TRACEABILITY.md` |

`docs/INDEX.md` is the full catalog. Do not preload the catalog.

## Non-negotiable invariants

1. Browser/admin origins bind loopback and are reached through Cloudflare Tunnel + Access; never expose origin ports.
2. Tunnel does not proxy ordinary PostgreSQL/Redis clients. Raw endpoints use DNS-only records and end-to-end TLS.
3. Never log, audit, cache, persist in browser storage, or return later: passwords, credential URLs, session/CSRF/tunnel/DNS tokens, or private keys.
4. Credential responses use `Cache-Control: no-store`; UI clears them on dismiss, navigation, and selection change.
5. Preserve the legacy Fernet vault and `SESSION_SECRET` compatibility until copied-record tests prove migration safety.
6. PostgreSQL destructive actions default off and protect system/state/admin databases, roles, and the configured deny-list.
7. Redis custom permissions are an explicit tested allow-list; arbitrary Redis commands are forbidden.
8. Rollback switches application binaries/config only; it never reverses data, migrations, rotations, Redis state, or backups.
9. Installer behavior is idempotent and cannot overwrite/reinitialize/remove an existing PostgreSQL cluster without explicit fresh-install mode and validated preconditions.
10. Backup acceptance requires checksums and an isolated restore test; same-server copies are not disaster recovery.
11. Support is the release-owned matrix in `docs/COMPATIBILITY.md`: exact immutable artifacts, no floating tags, implicit majors, or config-based widening.
12. PostgreSQL adoption, packages/extensions, preload/restart, and PgBouncer are separate lifecycles. Existing mode preserves by default; no blanket extension enablement, `template1` changes, or unapproved restart.

## Engineering contract

- Stay within one accepted PRD slice and its explicit non-goals.
- Verify APIs, flags, versions, service behavior, and external facts from code or exact primary sources; never invent them.
- Use exact supported dependency pins. New dependencies require compatibility/security evidence.
- Prefer the existing Go/React structure, with PostgreSQL behavior behind `postgresadmin`. PostgreSQL uses direct parameterized `pgx` without an ORM; identifiers are validated and safely quoted. HTTP endpoints are versioned under `/api/v1`.
- State changes emit secret-safe actor/action/target/outcome/request-ID/client-IP audit events with redacted metadata. Typed errors map to stable API errors; raw dependency errors never reach browsers.
- Production code is explicit, typed, bounded, cancellable where applicable, fail-closed, and covered by observable success/failure tests.
- PostgreSQL/Redis claims require the applicable `docs/COMPATIBILITY.md` matrix jobs with detected full versions and immutable artifacts.
- Preserve dirty work. Never commit runtime SQLite/WAL files, `.env`, certificates, tokens, binaries, backups, or credentials.
- Update the canonical owner and `docs/TRACEABILITY.md` in the same implementation change. Avoid duplicate status ledgers.
- UI changes use shared tokens/shell, responsive/accessibility evidence, and independent UI review.
- Local implementation/commits are allowed by the active workflow. Never push or change production, DNS, Cloudflare, secrets, or legacy services without separate explicit authorization.

## Documentation ownership

| Change | Canonical owner |
|---|---|
| Product/acceptance | `docs/PRD.md` |
| HTTP contract | `docs/API.md` |
| Module/data flow | `docs/ARCHITECTURE.md` plus ADR for durable decisions |
| Versions/support | `docs/COMPATIBILITY.md`, ADR-008 |
| UI workflow/system | `docs/UX.md`, `docs/UI_DESIGN_SYSTEM.md` |
| Configuration | `docs/CONFIGURATION.md` |
| Installer/runtime/network | `docs/INSTALLER_SPEC.md`, `docs/DEPLOYMENT.md`, `docs/DOMAIN_AND_NETWORK.md` |
| PostgreSQL lifecycle | `docs/POSTGRESQL_PROVISIONING.md`, ADR-009 |
| Backup/runbook | `docs/BACKUP_RECOVERY.md`, `docs/OPERATIONS.md` |
| Security/secrets | `docs/SECURITY.md`, `docs/DATA_AND_SECRETS.md`, ADR when architectural |
| Source parity | `docs/SOURCE_SYSTEMS.md`, `docs/SOURCE_BASELINE.md` |
| Tests/release gates | `docs/TESTING.md`, `docs/ACCEPTANCE_CHECKLIST.md` |
| Milestones/cutover | `docs/ROADMAP.md`, `docs/MIGRATION.md` |

## Completion protocol

Before coding, freeze the requirement, acceptance criteria, affected invariants/ADRs, source parity, and migration/rollback impact. Before claiming a slice complete:

1. Run the required focused and complete checks from `docs/TESTING.md`.
2. Confirm the diff contains no secret or runtime artifact.
3. Synchronize canonical docs and one current block in `docs/TRACEABILITY.md`.
4. Record exactly what ran, what did not, and why.
5. Obtain required independent security/UI review and verify the corrected immutable commit.

Security invariants and accepted ADRs outrank PRD, architecture/deployment docs, roadmap, and targets in that order. Resolve contradictions in the same change.
