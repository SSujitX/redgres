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
│   │   ├── compatibility.go
│   │   └── redaction.go
│   ├── platform/
│   │   └── status.go
│   ├── httpapi/
│   │   ├── server.go
│   │   ├── middleware.go
│   │   ├── errors.go
│   │   ├── auth_routes.go
│   │   ├── postgres_routes.go
│   │   ├── redis_routes.go
│   │   ├── audit_routes.go
│   │   └── system_routes.go
│   └── web/
│       └── embed.go
├── migrations/
│   ├── embed.go
│   ├── 001_initial.sql
│   └── 002_operations.sql
├── web/
│   ├── package.json
│   ├── package-lock.json
│   ├── vite.config.ts
│   └── src/
│       ├── api/
│       ├── components/
│       ├── features/
│       │   ├── auth/
│       │   ├── overview/
│       │   ├── postgres/
│       │   ├── redis-users/
│       │   ├── audit/
│       │   └── system/
│       ├── styles/
│       └── test/
├── integration/
│   ├── postgres_test.go
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
│   │   └── secrets.sh
│   ├── systemd/
│   ├── compose/
│   └── cloudflare/
├── docs/
├── .github/
│   ├── workflows/ci.yml
│   ├── ISSUE_TEMPLATE/
│   └── PULL_REQUEST_TEMPLATE.md
├── .env.example
├── AGENTS.md
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
- Deployment scripts never source application `.env` files as shell code; they parse known keys safely.
- Tests use fixtures and ephemeral services, never live production endpoints by default.

## Naming rules

- Go module: decide the final public organization first, then use `github.com/<owner>/redgres`; never publish the placeholder.
- Packages are lower-case singular concepts. Avoid `utils`, `common`, or `helpers` inside application code.
- Configuration and API use `postgres`, not `postgresql`, except user-facing prose where either is clear.
- Product is `Redgres`; binary/service/user/directories are lowercase `redgres`.
- Legacy `Redact` names appear only in migration compatibility code/docs.
