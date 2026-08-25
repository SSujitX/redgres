# Repository structure contract

The target structure keeps the application, deployment automation, documentation, and tests in one repository while preserving domain boundaries.

```text
Redgres/
├── cmd/
│   └── redgres/
│       └── main.go
├── internal/
│   ├── auth/
│   │   ├── owner.go
│   │   ├── password.go
│   │   ├── session.go
│   │   ├── csrf.go
│   │   └── security.go
│   ├── audit/
│   │   └── audit.go
│   ├── config/
│   │   ├── config.go
│   │   └── dotenv.go
│   ├── database/
│   │   ├── database.go
│   │   └── migrations.go
│   ├── operations/
│   │   ├── service.go
│   │   └── locks.go
│   ├── postgresadmin/
│   │   ├── adapter.go
│   │   ├── databases.go
│   │   ├── roles.go
│   │   ├── catalog.go
│   │   ├── rows.go
│   │   ├── duplicate.go
│   │   ├── rotation.go
│   │   ├── vault.go
│   │   ├── urls.go
│   │   ├── policy.go
│   │   └── errors.go
│   ├── redisadmin/
│   │   ├── adapter.go
│   │   ├── users.go
│   │   ├── presets.go
│   │   ├── credentials.go
│   │   ├── validation.go
│   │   └── errors.go
│   ├── secrets/
│   │   ├── fernet.go
│   │   ├── kdf.go
│   │   └── testdata/            # Python cryptography 49.0.0 Fernet/KDF fixtures
│   ├── platform/
│   │   ├── status.go
│   │   └── search.go
│   ├── httpapi/
│   │   ├── server.go
│   │   ├── middleware.go
│   │   ├── errors.go
│   │   ├── auth_routes.go
│   │   ├── postgres_routes.go
│   │   ├── redis_routes.go
│   │   ├── audit_routes.go
│   │   ├── search_routes.go
│   │   └── system_routes.go
│   └── web/
│       ├── embed.go
│       └── dist/
│           ├── .gitkeep
│           └── app/                 # Vite output; gitignored
├── migrations/
│   ├── embed.go
│   ├── 001_initial.sql
│   └── 002_operations.sql
├── web/
│   ├── package.json
│   ├── package-lock.json
│   ├── .nvmrc
│   ├── tsconfig.json
│   ├── tsconfig.node.json
│   ├── index.html
│   ├── vite.config.ts
│   └── src/
│       ├── api/
│       ├── text/
│       ├── components/
│       │   ├── shell/
│       │   ├── search/
│       │   └── ui/
│       ├── features/
│       │   ├── auth/
│       │   ├── overview/
│       │   ├── postgres/
│       │   ├── redis-users/
│       │   ├── audit/
│       │   └── system/
│       ├── styles/
│       │   ├── tokens.css
│       │   └── globals.css
│       └── test/
├── integration/
│   ├── postgres_test.go
│   ├── postgres_extensions_test.go
│   ├── redis_test.go
│   ├── vault_compatibility_test.go
│   └── fixtures/
├── deploy/
│   ├── install.sh
│   ├── verify.sh
│   ├── backup.sh
│   ├── rollback.sh
│   ├── update.sh
│   ├── lib/
│   │   ├── common.sh
│   │   ├── checks.sh
│   │   ├── logging.sh
│   │   ├── secrets.sh
│   │   ├── postgres.sh
│   │   ├── postgres_extensions.sh
│   │   └── pgbouncer.sh
│   ├── manifests/
│   │   └── postgres-capabilities.json  # release-owned exact package/extension mapping
│   ├── schemas/
│   │   └── postgres-extension-plan.schema.json
│   ├── tests/
│   │   └── run.sh
│   ├── systemd/
│   ├── compose/
│   └── cloudflare/
├── docs/
├── .agents/
│   └── skills/                    # repository-local workflows discovered by Cursor/agents
├── .cursor/
│   ├── commands/                  # reusable start/resume/status/fix entry points
│   ├── agents/                    # planner, bounded implementer, and independent reviewers
│   └── rules/                     # persistent and path-routed project instructions
├── .github/
│   ├── workflows/ci.yml          # authoritative Wave 0 check list
│   ├── ISSUE_TEMPLATE/
│   └── PULL_REQUEST_TEMPLATE.md
├── .env.example
├── AGENTS.md
├── CURSOR_CODING.md               # human day-to-day command cheat sheet
├── CONTRIBUTING.md
├── SECURITY.md
├── LICENSE
├── Makefile
├── go.mod
└── README.md
```

## Boundary rules

- `internal/httpapi` may call domain services; it does not execute SQL/Redis commands.
- `postgresadmin` is the only package allowed to perform PostgreSQL administration or vault queries.
- `redisadmin` is the only package allowed to perform Redis commands.
- `database` owns only Redgres SQLite state; it is not a generic PostgreSQL package.
- `secrets` implements compatibility/redaction primitives; it does not decide product policy.
- UI API types are generated from or checked against the versioned API contract where practical.
- UI shell, search, and primitives are shared components; feature folders consume semantic tokens and do not create independent navigation, palettes, or breakpoints.
- Deployment scripts never source application `.env` files as shell code; they parse known keys safely.
- `deploy/manifests/postgres-capabilities.json` is release-owned and maps canonical capability IDs to exact PostgreSQL-major/architecture artifacts, SQL names, preload/restart metadata and verification probes. Operator plans reference IDs only and cannot inject packages, repositories, SQL or libraries.
- Tests use fixtures and ephemeral services, never live production endpoints by default.

## Naming rules

- Go module: `github.com/SSujitX/redgres` (case-sensitive). The path follows the configured `origin` remote; do not lowercase it.
- Packages are lower-case singular concepts. Avoid `utils`, `common`, or `helpers` inside application code.
- Configuration and API use `postgres`, not `postgresql`, except user-facing prose where either is clear.
- Product is `Redgres`; binary/service/user/directories are lowercase `redgres`.
- Legacy `Redact` names appear only in migration compatibility code/docs.
