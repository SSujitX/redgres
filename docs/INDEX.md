# Redgres documentation index

This directory is the engineering source of truth. “Target” statements describe the intended Redgres system; “current” statements describe verified behavior in the two source repositories. Do not confuse plans with implementation evidence.

## Read first

For day-to-day Cursor commands, start at [../CURSOR_CODING.md](../CURSOR_CODING.md). Agents then route themselves through the engineering documents below.

1. [PROJECT_CHARTER.md](PROJECT_CHARTER.md) — mission, boundaries, principles, and decisions.
2. [PRD.md](PRD.md) — functional/non-functional requirements and acceptance criteria.
3. [SOURCE_SYSTEMS.md](SOURCE_SYSTEMS.md) — current Python/PostgreSQL and Go/Redis systems.
4. [ARCHITECTURE.md](ARCHITECTURE.md) — target components and flows.
5. [COMPATIBILITY.md](COMPATIBILITY.md) — supported service versions, defaults, selection, and test matrix.
6. [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md) — visual direction, responsive shell, search, login, and UI review gate.
7. [SECURITY.md](SECURITY.md) — threat model and mandatory controls.
8. [MIGRATION.md](MIGRATION.md) — sequence, gates, coexistence, and retirement.

## Product and contracts

- [PRD.md](PRD.md)
- [API.md](API.md)
- [DATA_AND_SECRETS.md](DATA_AND_SECRETS.md)
- [DOMAIN_AND_NETWORK.md](DOMAIN_AND_NETWORK.md)
- [COMPATIBILITY.md](COMPATIBILITY.md)
- [TRACEABILITY.md](TRACEABILITY.md)
- [ACCEPTANCE_CHECKLIST.md](ACCEPTANCE_CHECKLIST.md)
- [GLOSSARY.md](GLOSSARY.md)
- [REFERENCES.md](REFERENCES.md)

## Engineering and operations

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [REPOSITORY_STRUCTURE.md](REPOSITORY_STRUCTURE.md)
- [DEPLOYMENT.md](DEPLOYMENT.md)
- [INSTALLER_SPEC.md](INSTALLER_SPEC.md)
- [BACKUP_RECOVERY.md](BACKUP_RECOVERY.md)
- [OPERATIONS.md](OPERATIONS.md)
- [TESTING.md](TESTING.md)
- [MIGRATION.md](MIGRATION.md)
- [ROADMAP.md](ROADMAP.md)
- [CONFIGURATION.md](CONFIGURATION.md)
- [POSTGRESQL_PROVISIONING.md](POSTGRESQL_PROVISIONING.md)
- [CURSOR_WORKFLOW.md](CURSOR_WORKFLOW.md)
- [UX.md](UX.md)
- [UI_DESIGN_SYSTEM.md](UI_DESIGN_SYSTEM.md)
- [OPEN_SOURCE.md](OPEN_SOURCE.md)
- [RISK_REGISTER.md](RISK_REGISTER.md)
- [SOURCE_BASELINE.md](SOURCE_BASELINE.md)

## Architecture decisions

- [ADR-001: Go modular monolith](decisions/ADR-001-go-modular-monolith.md)
- [ADR-002: Hybrid host and container deployment](decisions/ADR-002-hybrid-deployment.md)
- [ADR-003: Network and domain separation](decisions/ADR-003-network-boundaries.md)
- [ADR-004: Preserve the Fernet vault](decisions/ADR-004-fernet-vault-compatibility.md)
- [ADR-005: SQLite for control-plane state](decisions/ADR-005-sqlite-control-plane.md)
- [ADR-006: Redis explicit command allow-list](decisions/ADR-006-redis-command-allowlist.md)
- [ADR-007: Staged migration with legacy fallback](decisions/ADR-007-staged-migration.md)
- [ADR-008: Tested service-version matrix and operator selection](decisions/ADR-008-service-version-policy.md)
- [ADR-009: PostgreSQL adoption, extension enablement, and PgBouncer lifecycle](decisions/ADR-009-postgres-adoption-and-extensions.md)

## Status labels

- **Verified current**: inspected in a named source repository and pinned baseline.
- **Target**: approved design, not necessarily implemented.
- **Provisional**: must be checked against the live VPS/provider state.
- **Gate**: objective evidence required before progression.
