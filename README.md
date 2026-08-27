# Redgres

**One secure control plane for PostgreSQL and Redis.**

Redgres is a self-hosted Go/React administration console for one PostgreSQL cluster and one Redis instance. It is the migration successor to the local read-only reference systems `../database-app` and `../redis-ui`; production never depends on those repositories.

## Status

The repository contains substantial authenticated PostgreSQL/Redis administration, audit, UI, integration harnesses, and partial deployment/recovery code. It is **not production accepted**. The evidence-backed current matrix and limitations are in [docs/TRACEABILITY.md](docs/TRACEABILITY.md); planned behavior is not implementation evidence.

Do not retire either legacy console until every gate in [docs/MIGRATION.md](docs/MIGRATION.md) and [docs/ACCEPTANCE_CHECKLIST.md](docs/ACCEPTANCE_CHECKLIST.md) passes on authorized staging/production infrastructure.

## Develop

Required versions are pinned in `go.mod`, `web/.nvmrc`, and lockfiles.

```powershell
npm --prefix web ci
npm --prefix web run build
go test ./...
go vet ./...
```

Create a development owner once:

```powershell
go run ./cmd/redgres create-owner --username admin
```

Run the embedded API/UI development stack:

```powershell
npm --prefix web run dev:full
```

Open <http://127.0.0.1:8989>. Stop with `Ctrl+C`. A frontend rebuild briefly clears embedded assets; refresh after Vite finishes.

Frontend-only HMR uses two terminals:

```powershell
$env:REDGRES_BASE_URL = "http://127.0.0.1:5173"
go run ./cmd/redgres serve
```

```powershell
npm --prefix web run dev
```

Open <http://127.0.0.1:5173>. Vite proxies `/api` to the local Go process.

Development without PostgreSQL/Redis configuration is supported for shell work; dependent pages report unavailable. Configure secrets only through the documented credential-file settings in [docs/CONFIGURATION.md](docs/CONFIGURATION.md). Never put passwords, tokens, or private keys in `.env`, command lines, logs, commits, or browser storage.

## Product boundary

Redgres provides:

- PostgreSQL project database/role lifecycle, bounded inspection, credentials, duplication, and guarded destructive workflows;
- Redis ACL user lifecycle with explicit command allow-lists and key-prefix isolation;
- owner authentication, audit, health, search, and protected expert-tool links;
- deterministic installer, backup, verification, update, and rollback workflows as they become accepted.

Redgres is not a public DBaaS, SQL workbench, Redis key browser, Kubernetes operator, fleet manager, or replacement for pgAdmin/RedisInsight.

## Documentation

Start with [docs/INDEX.md](docs/INDEX.md). Key entry points:

- [AGENTS.md](AGENTS.md) — compact agent safety, routing, and completion contract;
- [docs/PRD.md](docs/PRD.md) — requirements and acceptance criteria;
- [docs/TRACEABILITY.md](docs/TRACEABILITY.md) — current implementation/test evidence;
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — component and data-flow contracts;
- [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) — exact supported service matrix;
- [docs/SECURITY.md](docs/SECURITY.md) — threat model and controls;
- [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) — target runtime/topology;
- [docs/CURSOR_WORKFLOW.md](docs/CURSOR_WORKFLOW.md) — local agent orchestration.

Cursor users open `Redgres.code-workspace` and run `/start-redgres`. The workflow may create reviewed local commits, but it never pushes or changes production/DNS/Cloudflare without separate explicit authorization.

## License and name

Apache-2.0 is the recommended project license. Vendored agent skills retain their upstream notices in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). The Redgres name is not represented as legally cleared.
