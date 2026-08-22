# Roadmap

Roadmap items are not implementation claims. Each milestone exits only through the gates in the PRD and migration plan.

## M0 — specification and provenance

- Complete authoritative docs, ADRs, traceability skeleton.
- Initialize Git and choose public owner/module after availability/legal check.
- Pin/import Redis source provenance and PostgreSQL source commit.
- Inventory live infrastructure.

## M1 — secure Redgres foundation

- Go/React project bootstrap from reviewed Redis foundation.
- Owner auth/session/audit/config/SQLite migration.
- Distinctive responsive shell, login, sidebar/icon rail/mobile drawer, topbar search/owner menu, status, and versioned API.
- Release, CI, SBOM, secret scan, exact dependency/toolchain pinning, and reviewed automated update proposals.

## M2 — Redis parity

- Explicit command allow-list and versioned presets.
- One-time/no-store credential handling.
- ACL management and the Redis 8.2/8.8 TLS integration matrix defined in [COMPATIBILITY.md](COMPATIBILITY.md).
- Migration from legacy Redact without importing runtime SQLite blindly.

## M3 — PostgreSQL read-only parity

- pgx adapter, inventory/details/security/tables/rows.
- Direct/PgBouncer health and URL metadata.
- Protected target policy.
- PostgreSQL 17/18 catalog and capability compatibility matrix.

## M4 — vault and PostgreSQL provisioning

- Fernet compatibility gates.
- Create/reveal/rotate with operation state and audit.
- Duplicate database parity.

## M5 — guarded destructive operations

- Row delete, truncate, drop with feature flags, reauth, backup policy, fault tests.

## M6 — deployment and production cutover

- Idempotent fresh/existing installer.
- Operator service-version selection/detection with exact package/image pinning.
- Independent PostgreSQL/PgBouncer existing/fresh modes plus preserve/apply extension plans from [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md).
- Backup/restore automation.
- Staging/shadow period, controlled cutover, observation, legacy retirement.

## Post-v1 candidates

- Multiple operator accounts and scoped RBAC/MFA integration.
- Multiple PostgreSQL/Redis targets with explicit trust boundaries.
- WebAuthn/passkeys.
- PostgreSQL PITR/WAL backup integration.
- External secret-manager integration.
- OIDC mapped to internal capabilities.
- Signed update channel and automated provenance verification.

Do not add multi-server/HA/RBAC complexity before the single-server migration is correct and recoverable.
