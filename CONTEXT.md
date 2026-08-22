# Redgres domain context

Redgres is a self-hosted control plane for one PostgreSQL cluster and one Redis instance. It is not the application data plane and is not a public multi-tenant database service.

## Canonical language

- **Owner**: the authenticated Redgres operator. Version 1 has one owner, but authorization still uses internal capabilities.
- **Project database**: a manageable PostgreSQL database owned by a restricted project role.
- **Project role**: the least-privilege PostgreSQL login associated with project database access.
- **Redis ACL user**: a Redis identity constrained by explicit commands and one project key prefix.
- **Direct PostgreSQL**: native PostgreSQL endpoint on port 5432.
- **Pooled PostgreSQL**: PgBouncer endpoint on port 6432.
- **Vault**: the existing `database_console_vault` PostgreSQL database containing Fernet-encrypted project passwords.
- **Control state**: Redgres SQLite data for owner hashes, sessions, CSRF hashes, lockouts, audit events, and operation state. It never stores project credentials.
- **Protected resource**: database, role, or Redis user that ordinary Redgres workflows may never mutate.
- **Legacy PostgreSQL console**: `D:\code\github\database-app`, used as a read-only behavioral reference.
- **Legacy Redis console / Redact**: `D:\code\github\redis-ui`, used as a read-only Go/React foundation reference.

The complete glossary is [docs/GLOSSARY.md](docs/GLOSSARY.md). Product decisions are in [docs/decisions](docs/decisions); requirements are in [docs/PRD.md](docs/PRD.md).

## Bounded contexts

- `auth`: owner identity, password verification, sessions, CSRF, lockout, reauthentication.
- `audit`: append-only, secret-safe operator action history.
- `postgresadmin`: PostgreSQL inventory, project lifecycle, rows, URLs, rotation, duplication, protected policy, and vault access.
- `redisadmin`: Redis health and ACL user lifecycle through explicit safe commands.
- `platform`: aggregate health, release/runtime status, operation coordination, and expert-tool links.
- `database`: Redgres SQLite control-state persistence only.
- `httpapi`: versioned transport, middleware, error mapping, and embedded frontend delivery.

Do not use “user” without qualifying owner, PostgreSQL role, or Redis ACL user. Do not call RedisInsight the Redis admin console; `redis-admin` means ACL administration and `redis-insight` means data exploration.
