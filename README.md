# Redgres

**One secure control plane for PostgreSQL and Redis.**

Redgres is the planned open-source successor to two existing private administration consoles:

- `D:\code\github\database-app` — PostgreSQL provisioning and data-management console written in Python/FastAPI.
- `D:\code\github\redis-ui` — Redis ACL administration console (currently branded Redact) written in Go with an embedded React application.

This repository is currently the authoritative product and engineering specification. It does **not** yet contain a production Redgres implementation. The source applications remain operationally independent until the migration gates in [docs/MIGRATION.md](docs/MIGRATION.md) are passed.

## Start coding with Cursor

1. Open `Redgres.code-workspace` in Cursor—not an individual sibling folder.
2. Open Agent mode.
3. Run `/start-redgres`.

That is the normal entry point. The command inspects Git/roadmap/traceability, selects the next dependency-ready slice, invokes the planner, uses isolated implementation subagents when parallel work is safe, runs reviewers/tests, and synchronizes canonical documentation. Project rules in `.cursor/rules` apply persistently; specialized skills and subagents are loaded when relevant.

For start, resume, status, and bug-fix copy/paste commands, use [CURSOR_CODING.md](CURSOR_CODING.md).

Do not paste all documentation into chat. Agents receive the durable project context through the always-applied rule, `AGENTS.md`, routed docs, committed code, and explicit subagent context packets. See [docs/CURSOR_WORKFLOW.md](docs/CURSOR_WORKFLOW.md) for limits and recovery behavior.

## Product boundary

Redgres will provide one authenticated browser console for:

- PostgreSQL database and project-role provisioning, connection URLs, inventory, safe inspection, duplication, credential rotation, and guarded destructive operations.
- Redis ACL user provisioning, prefix isolation, permission presets, enable/disable, credential rotation, and guarded deletion.
- Platform health, audit history, operational links, and backup/verification status.

Redgres is not intended to replace pgAdmin or RedisInsight. Those tools remain optional, separately protected expert consoles. Version 1 is also not a database-as-a-service control plane, SQL workbench, Redis key browser, Kubernetes operator, or multi-server fleet manager.

## Recommended architecture

- Backend: latest stable compatible Go toolchain, Chi, `pgx/v5` + `pgxpool`, `go-redis/v9`, and SQLite via `modernc.org/sqlite`, all exactly pinned at the implementation/release baseline.
- Frontend: latest stable compatible React, TypeScript, Vite, Tailwind CSS, TanStack Query, and Radix primitives, locked in the npm lockfile and embedded into the Go binary.
- Runtime: Ubuntu 24.04 LTS; Redgres as a systemd service; PostgreSQL 17/18 and PgBouncer independently adopted or installed host-native; a supported Redis 8.2/8.8 selection in Docker Compose; `cloudflared` as a systemd service. Exact packages/images are release-pinned.
- Browser ingress: Cloudflare Tunnel + Cloudflare Access to loopback-only HTTP services.
- Database ingress: direct DNS records with end-to-end TLS, authentication, `pg_hba.conf`/Redis ACLs, and source-restricted firewall rules where possible.

There is deliberately no Kubernetes requirement and no Node.js runtime in production.

“Latest” means the newest stable, security-supported, mutually compatible release that passes Redgres tests—not a floating package/container tag, beta, RC, or automatic unreviewed major upgrade.

PostgreSQL installation is not all-or-nothing: Redgres can preserve/adopt an existing cluster or install a fresh supported major. Optional capabilities such as pgvector, PostGIS, TimescaleDB, pgAudit and contrib extensions are explicitly selected, pinned and enabled only in named databases; PgBouncer remains a separate service. See [docs/POSTGRESQL_PROVISIONING.md](docs/POSTGRESQL_PROVISIONING.md).

## Documentation map

Start with [docs/INDEX.md](docs/INDEX.md). The most important documents are:

- [AGENTS.md](AGENTS.md) — mandatory context and rules for any implementation agent.
- [.agents/README.md](.agents/README.md) — pinned repository-local engineering skills and setup requirements.
- [docs/CURSOR_WORKFLOW.md](docs/CURSOR_WORKFLOW.md) — automatic Cursor context routing, subagents, worktrees, and kickoff prompts.
- [docs/PRD.md](docs/PRD.md) — product requirements and acceptance criteria.
- [docs/SOURCE_SYSTEMS.md](docs/SOURCE_SYSTEMS.md) — how both current repositories work and what must be preserved.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — target components, boundaries, and request/data flows.
- [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) — authoritative service-version choices, defaults, detection, and test matrix.
- [docs/UI_DESIGN_SYSTEM.md](docs/UI_DESIGN_SYSTEM.md) — distinctive responsive shell, sidebar, topbar search, login, visual tokens, and review requirements.
- [docs/SECURITY.md](docs/SECURITY.md) — threat model and non-negotiable controls.
- [docs/MIGRATION.md](docs/MIGRATION.md) — staged migration and cutover gates.
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — target server, filesystem, networking, and installer behavior.

## Proposed repository layout

```text
redgres/
├── cmd/redgres/                 # Future executable entry point
├── internal/
│   ├── auth/                    # Owner auth, sessions, CSRF, reauthentication
│   ├── audit/                   # Secret-safe audit events
│   ├── config/                  # Fail-closed configuration loading
│   ├── database/                # Redgres SQLite state and migrations
│   ├── postgresadmin/           # PostgreSQL adapter and use cases
│   ├── redisadmin/              # Redis ACL adapter and use cases
│   ├── secrets/                 # Fernet compatibility and future key handling
│   ├── httpapi/                 # Versioned HTTP API and middleware
│   └── web/                     # Embedded frontend assets
├── migrations/                  # SQLite schema migrations
├── web/                         # React application
├── integration/                 # PostgreSQL, Redis, TLS, and vault tests
├── deploy/                      # Installer modules, units, Compose, verification
├── docs/                        # Product, engineering, and operations source of truth
└── .github/                     # CI and contribution templates
```

The layout is a contract for implementation, not evidence that these packages already exist.

## Naming and licensing

- Product/UI: **Redgres** / **Redgres Console**
- Binary: `redgres`
- Linux service account: `redgres`
- systemd unit: `redgres.service`
- Default public console hostname: `console.onelifeltd.xyz`
- Recommended open-source license: Apache License 2.0

Vendored repository-local agent skills retain their upstream MIT notice in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

The name had no obvious exact software-project collision in a quick web search, but project creation must still include formal GitHub organization/repository, package/module, domain, and trademark availability checks. Do not represent the name as legally cleared.

## Current status

Status: **Wave 0 foundation, owner auth, login/shell, PostgreSQL inventory, and table-list API**. Redis, vault decrypt, mutations, row browse, and installer behavior are not implemented. The unauthenticated login route does not call `/api/v1/healthz`.

No source code from either existing application has been copied into this repository. Do not decommission either application based only on these documents.
