# Redgres agent contract

Keep this always-loaded file compact; routed documents own detail.

## Start

Before planning or editing:

1. Read `docs/PROJECT_CHARTER.md`, then inspect Git status/diff and preserve user-owned work.
2. Read the applicable `docs/TRACEABILITY.md` row and current slice. Freeze the PRD IDs, acceptance criteria, non-goals, invariants/ADRs, parity needs, and migration/rollback impact. Target prose is not implementation evidence.
3. Load only the relevant route below, locating its heading/endpoint first. Use `docs/INDEX.md` only when no route fits; `README.md` is optional human guidance.

Open `Redgres.code-workspace`. Use one writing command per checkout: `/start-redgres` for new work, `/resume-redgres` for an unfinished slice, `/status-redgres` for reports, or `/fix-redgres <issue>` for one reproduced issue.

## Boundary and language

Redgres is a self-hosted, single-owner control plane for one PostgreSQL cluster and one Redis instance—not a public DBaaS or replacement for pgAdmin/RedisInsight. `docs/TRACEABILITY.md` owns current implementation status; every group is Partial until its listed evidence is accepted.

Qualify **user** as owner, PostgreSQL role, or Redis ACL user. Prefer **project database/role**, **Redis ACL user**, **direct** (5432), **pooled** (6432), **vault** (`database_console_vault`), **control state** (SQLite; no project credentials), and **protected resource**. `redis-admin` manages ACLs; `redis-insight` explores data. Put new terms in `docs/GLOSSARY.md`; do not recreate `CONTEXT.md`.

Read-only parity sources are `D:\\code\\github\\database-app` (PostgreSQL) and `D:\\code\\github\\redis-ui` (Redis). Never edit or depend on them; pin reviewed commits in `docs/SOURCE_BASELINE.md`.

## Routes and owners

| Change | Read / update canonical owner |
|---|---|
| Product/acceptance | `docs/PRD.md`; `docs/TRACEABILITY.md` |
| HTTP/API | `docs/API.md`; `docs/ARCHITECTURE.md`; security route when sensitive |
| Go/data flow | `docs/ARCHITECTURE.md`; `docs/COMPATIBILITY.md`; `docs/SOURCE_SYSTEMS.md` only for parity; ADR for durable decisions |
| Auth/secrets/destructive actions | `docs/SECURITY.md`; `docs/DATA_AND_SECRETS.md`; governing ADR |
| React/UI | `.agents/skills/redgres-ui-design/SKILL.md`; `docs/UX.md`; `docs/UI_DESIGN_SYSTEM.md`; affected API |
| Installer/runtime/network | `docs/INSTALLER_SPEC.md`; `docs/DEPLOYMENT.md`; `docs/CONFIGURATION.md`; `docs/DOMAIN_AND_NETWORK.md`; `docs/COMPATIBILITY.md`; `docs/CLOUDFLARED.md` for tunnel units |
| PostgreSQL lifecycle/extensions | `docs/POSTGRESQL_PROVISIONING.md`; ADR-009 |
| Backup/recovery | `docs/BACKUP_RECOVERY.md`; `docs/OPERATIONS.md`; ADR-011/ADR-005 |
| Migration/cutover | `docs/MIGRATION.md`; `docs/ROADMAP.md`; `docs/TESTING.md`; affected ADRs |
| Tests/release evidence | `docs/TESTING.md`; `docs/ACCEPTANCE_CHECKLIST.md`; `docs/TRACEABILITY.md` |
| Public GitHub Pages | `site/` only; never publish `docs/` |

## Invariants

1. Browser/admin origins bind loopback behind Cloudflare Tunnel + Access; origin ports stay private.
2. Tunnel excludes ordinary PostgreSQL/Redis clients; raw endpoints use DNS-only records and end-to-end TLS.
3. Passwords, credential URLs, session/CSRF/tunnel/DNS tokens, and private keys never enter logs, audits, caches, browser storage, or later responses; server storage is limited to documented mode-0600 secret files.
4. Credential responses are `Cache-Control: no-store`; UI clears them on dismiss, navigation, and selection change.
5. Preserve Fernet-vault and `SESSION_SECRET` compatibility until copied-record tests prove migration safety.
6. PostgreSQL destructive actions default off and protect system/state/admin databases, roles, and the configured deny-list.
7. Redis custom permissions use an explicit tested allow-list; arbitrary commands remain unavailable.
8. Rollback changes only application binaries/config—not data, migrations, rotations, Redis state, or backups.
9. Installer runs are idempotent; only explicit fresh-install mode with validated preconditions may overwrite, reinitialize, or remove a PostgreSQL cluster.
10. Backup acceptance requires checksums and an isolated restore; same-server copies are not disaster recovery.
11. `docs/COMPATIBILITY.md` owns support: exact immutable artifacts only, with no floating tags, implicit majors, or config-based widening.
12. PostgreSQL adoption, packages/extensions, preload/restart, and PgBouncer are separate lifecycles. Existing mode preserves by default; no blanket extension enabling, `template1` changes, or unapproved restart.

## Engineering

- Work within one accepted PRD slice. Verify external/API/version facts from code or exact primary sources. Use supported pins; new dependencies need compatibility/security evidence.
- Keep the Go/React structure. PostgreSQL lives behind `postgresadmin`, uses parameterized `pgx` without an ORM, and validates/quotes identifiers. APIs live under `/api/v1`.
- State changes emit redacted, secret-safe actor/action/target/outcome/request-ID/client-IP audit events. Typed errors map to stable API errors; dependency errors stay server-side.
- Production paths are explicit, typed, bounded, cancellable where applicable, fail-closed, and tested for observable success/failure.
- PostgreSQL/Redis claims require applicable compatibility-matrix jobs reporting detected full versions and immutable artifacts.
- Never commit runtime SQLite/WAL, `.env`, certificates, tokens, binaries, backups, or credentials.
- Update the canonical owner and one current `docs/TRACEABILITY.md` block with implementation; create no duplicate status ledger. UI work also needs shared tokens/shell, responsive/accessibility evidence, and independent UI review.
- Local edits/commits are allowed by the active workflow. Pushing or changing production, DNS, Cloudflare, secrets, or legacy services requires separate explicit authorization.

## Completion

Before claiming a slice complete:

1. Run its focused and complete `docs/TESTING.md` checks.
2. Inspect the diff for secrets/runtime artifacts and synchronize canonical docs plus one traceability block.
3. Record exactly what ran, what did not, and why.
4. Obtain required independent security/UI review and verify the corrected immutable commit.

Precedence: security invariants, accepted ADRs, PRD, architecture/deployment docs, roadmap, then targets. Resolve contradictions in the same change.

## Local preferences and facts

- Installer commits contain only installer, deploy, and related auth work; exclude unrelated UI/theme changes. Keep `.github/workflows/ci.yml`, leave full CI off push until ready, and use a separate installer workflow gate.
- Finish application code before Ubuntu-host verification. GitHub Actions service containers are development evidence, not production acceptance.
- Atlanta VPS `45.76.250.202` (`root`) is the authorized Redgres test host. SSH, installer uninstall/reinstall, and Cloudflare test-token use there need no further approval. Keep its password/API token only in the operator home env file; never commit them.
- Root `install.sh` is application-only (`curl | bash`); full-stack PostgreSQL/Redis/PgBouncer uses `git clone` then `sudo ./deploy/install.sh`.
- Windows live-installer checks use `C:\\Program Files\\Git\\bin\\bash.exe`; WSL lacks `/bin/bash`.
- The live Ubuntu gate covers 24.04 (`noble`) and 26.04 (`resolute`) through PGDG named packages; this is not 26.04-only exact apt pinning and does not widen `docs/COMPATIBILITY.md` section 6.
