# Redgres

**One secure control plane for PostgreSQL and Redis.**

Redgres is the planned open-source successor to two existing private administration consoles:

- `D:\code\github\database-app` — PostgreSQL provisioning and data-management console written in Python/FastAPI.
- `D:\code\github\redis-ui` — Redis ACL administration console (currently branded Redact) written in Go with an embedded React application.

This repository is currently the authoritative product and engineering specification. It does **not** yet contain a production Redgres implementation. The source applications remain operationally independent until the migration gates in [docs/MIGRATION.md](docs/MIGRATION.md) are passed.

## Product boundary

Redgres will provide one authenticated browser console for:

- PostgreSQL database and project-role provisioning, connection URLs, inventory, safe inspection, duplication, credential rotation, and guarded destructive operations.
- Redis ACL user provisioning, prefix isolation, permission presets, enable/disable, credential rotation, and guarded deletion.
- Platform health, audit history, operational links, and backup/verification status.

Redgres is not intended to replace pgAdmin or RedisInsight. Those tools remain optional, separately protected expert consoles. Version 1 is also not a database-as-a-service control plane, SQL workbench, Redis key browser, Kubernetes operator, or multi-server fleet manager.

## Recommended architecture

- Backend: Go, Chi, `pgx/v5` + `pgxpool`, `go-redis/v9`, SQLite via `modernc.org/sqlite`.
- Frontend: React 19, TypeScript, Vite, Tailwind CSS, TanStack Query, Radix primitives; embedded into the Go binary.
- Runtime: Ubuntu 24.04 LTS; Redgres as a systemd service; PostgreSQL 17 and PgBouncer host-native; Redis 8 in Docker Compose; `cloudflared` as a systemd service.
- Browser ingress: Cloudflare Tunnel + Cloudflare Access to loopback-only HTTP services.
- Database ingress: direct DNS records with end-to-end TLS, authentication, `pg_hba.conf`/Redis ACLs, and source-restricted firewall rules where possible.

There is deliberately no Kubernetes requirement and no Node.js runtime in production.

## Documentation map

Start with [docs/INDEX.md](docs/INDEX.md). The most important documents are:

- [AGENTS.md](AGENTS.md) — mandatory context and rules for any implementation agent.
- [.agents/README.md](.agents/README.md) — pinned repository-local engineering skills and setup requirements.
- [docs/CURSOR_WORKFLOW.md](docs/CURSOR_WORKFLOW.md) — automatic Cursor context routing, subagents, worktrees, and kickoff prompts.
- [docs/PRD.md](docs/PRD.md) — product requirements and acceptance criteria.
- [docs/SOURCE_SYSTEMS.md](docs/SOURCE_SYSTEMS.md) — how both current repositories work and what must be preserved.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — target components, boundaries, and request/data flows.
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

Status: **Specification / pre-implementation**.

No source code from either existing application has been copied into this repository. Do not decommission either application based only on these documents.
